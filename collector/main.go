// speedmon-collector: invisible background logger for internet performance and outages.
// v1.0.0  2026-11-04  First release; replaces speedmon_bg.py and browser scheduling.
//
// One small binary per OS, no runtime dependencies. Each computer appends its own CSVs
// under <root>/data/<machine>/ (Dropbox syncs them) and writes a viewer file to
// <root>/viewer_data/ that speed_monitor.html loads with no server.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	ndt7 "github.com/m-lab/ndt7-client-go"
	"github.com/m-lab/ndt7-client-go/spec"
)

// --- Constants
const VERSION_STR = "1.0.0"
const SINGLETON_ADDR = "127.0.0.1:48765" // held, never accepted on; stops a second collector
const WITNESS_TIMEOUT = 5 * time.Second
const CONFIRM_DELAY = 10 * time.Second
const SPEED_TIMEOUT = 90 * time.Second
const VIEWER_REFRESH = 10 * time.Minute
const NETWORK_REFRESH = 6 * time.Hour
const DB_REFRESH = time.Hour
const WAKE_SETTLE_SECS = 120.0 // no failures or speed tests count while Wi-Fi reconnects after sleep
const MIN_GAP_SECS = 200.0     // longer than a speed test, so a test never reads as sleep
const RECENT_SECS = 8 * 86400.0
const MIN_OUTAGE_CHECKS = 2
const MAX_DOWN_MBPS = 20000.0
const MAX_UP_MBPS = 20000.0
const CLOUDFLARE_CAP_BYTES = int64(256 * 1024 * 1024)

// Check states.
const ST_ONLINE = "online"
const ST_INTERNET = "internet" // router reachable or unknown, internet not: ISP-side outage
const ST_LOCAL = "local"       // joined a network but the router does not answer
const ST_NO_NET = "no_network" // not joined to any network: not an outage
const ST_CAPTIVE = "captive"   // hotel-style sign-in page intercepts traffic: not an outage

type witness struct {
	url, body string
	code      int
}

// Purpose-built connectivity endpoints; a sign-in page cannot fake these answers.
var WITNESSES = []witness{
	{"https://www.gstatic.com/generate_204", "", 204},
	{"https://cp.cloudflare.com/generate_204", "", 204},
	{"http://captive.apple.com/hotspot-detect.html", "Success", 200},
}

// --- Types
type config struct{ Machine, Root string }
type settings struct {
	CheckSeconds int     `json:"check_seconds"`
	SpeedHours   float64 `json:"speed_hours"`
}
type netInfo struct{ label, iface string }
type check struct {
	epoch   float64
	state   string
	rtt     string
	network string
}
type speedRow struct {
	epoch                            float64
	status, provider, down, up, ping string
	server, network, iface, note     string
}

// --- Global state
var no_keepalive = &http.Client{
	Transport:     &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
var bulk_client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableCompression: true}}

// --- Main
func main() {
	if len(os.Args) < 2 {
		fatal("usage: speedmon-collector install|uninstall|run|once|import|build|db|query|status [--machine NAME] [--root DIR]")
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	machine_flag := fs.String("machine", "", "label for this computer")
	root_flag := fs.String("root", "", "project folder holding data/")
	fs.Parse(os.Args[2:])
	switch cmd {
	case "install":
		install(*machine_flag, *root_flag)
	case "uninstall":
		uninstall()
	case "run":
		runLoop(loadConfig())
	case "once":
		cfg := loadConfig()
		info := networkInfo()
		c := measureCheck(info)
		fmt.Printf("check: %s rtt=%sms network=%q iface=%s\n", c.state, c.rtt, info.label, info.iface)
		appendCheck(cfg, c)
		if c.state == ST_ONLINE {
			r := measureSpeed(info)
			fmt.Printf("speed: %s %s down=%s up=%s ping=%s %s %s\n", r.status, r.provider, r.down, r.up, r.ping, r.server, r.note)
			appendSpeed(cfg, r)
		}
		mustLog(buildViewerFiles(cfg))
	case "import":
		cfg := loadConfig()
		mustLog(importLegacy(cfg.Root, cfg.Machine))
		mustLog(buildViewerFiles(cfg))
	case "build":
		mustLog(buildViewerFiles(loadConfig()))
	case "db":
		path, err := buildDatabase(loadConfig().Root, databasePath())
		mustLog(err)
		fmt.Println("built", path)
	case "query":
		if fs.NArg() != 1 {
			fatal("usage: speedmon-collector query \"SELECT ...\"")
		}
		mustLog(runQuery(databasePath(), fs.Arg(0)))
	case "status":
		cfg := loadConfig()
		fmt.Printf("machine=%q root=%q settings=%+v\n", cfg.Machine, cfg.Root, loadSettings(cfg.Root))
	default:
		fatal("unknown command " + cmd)
	}
}

// --- Scheduler
func runLoop(cfg config) {
	lock, err := net.Listen("tcp", SINGLETON_ADDR)
	if err != nil {
		log.Fatal("another collector is already running")
	}
	defer lock.Close()
	log.Printf("collector %s machine=%q root=%q", VERSION_STR, cfg.Machine, cfg.Root)
	next_speed := nextSpeedTime(lastSpeedEpoch(cfg), loadSettings(cfg.Root).SpeedHours)
	last_viewer, last_db := time.Time{}, time.Time{}
	last_tick := time.Time{}
	var info netInfo
	local_ip, last_lookup := "", time.Time{}
	for {
		now := time.Now().Round(0) // wall clock: monotonic time can stop during sleep
		st := loadSettings(cfg.Root)
		settling := !last_tick.IsZero() && now.Sub(last_tick).Seconds() > gapLimit(st.CheckSeconds)
		ip := localIP()
		if ip != local_ip || settling || time.Since(last_lookup) > NETWORK_REFRESH || info.label == "" {
			info = networkInfo()
			local_ip, last_lookup = ip, time.Now()
		}
		c := measureCheck(info)
		appendCheck(cfg, c)
		if settling { // give the network time to return before an expensive test
			if t := now.Add(WAKE_SETTLE_SECS * time.Second); next_speed.Before(t) {
				next_speed = t
			}
		}
		if c.state == ST_ONLINE && !time.Now().Before(next_speed) {
			appendSpeed(cfg, measureSpeed(info))
			next_speed = nextSpeedTime(float64(time.Now().Unix()), st.SpeedHours)
		}
		if time.Since(last_viewer) > VIEWER_REFRESH {
			if err := buildViewerFiles(cfg); err != nil {
				log.Print("viewer build: ", err)
			}
			last_viewer = time.Now()
		}
		if time.Since(last_db) > DB_REFRESH {
			if _, err := buildDatabase(cfg.Root, databasePath()); err != nil {
				log.Print("database build: ", err)
			}
			last_db = time.Now()
		}
		last_tick = time.Now().Round(0)
		time.Sleep(time.Duration(st.CheckSeconds) * time.Second)
	}
}

// Randomized spacing: M-Lab refuses clients that test on an exact periodic schedule.
func nextSpeedTime(last_epoch, hours float64) time.Time {
	if last_epoch == 0 {
		return time.Now()
	}
	jitter := 0.85 + 0.3*mrand.Float64()
	return time.Unix(int64(last_epoch+hours*3600*jitter), 0)
}

func gapLimit(check_seconds int) float64 {
	if g := 3 * float64(check_seconds); g > MIN_GAP_SECS {
		return g
	}
	return MIN_GAP_SECS
}

// --- Measurement: connectivity
func measureCheck(info netInfo) check {
	c := check{epoch: nowEpoch(), network: info.label}
	if localIP() == "" {
		c.state = ST_NO_NET
		return c
	}
	ok, reached, rtt := probeWitnesses()
	if !ok && !reached { // confirm before believing a failure
		time.Sleep(CONFIRM_DELAY)
		ok, reached, rtt = probeWitnesses()
	}
	switch {
	case ok:
		c.state, c.rtt = ST_ONLINE, fmt.Sprintf("%.0f", rtt)
	case reached:
		c.state = ST_CAPTIVE
	case gatewayAnswers() == "fail":
		c.state = ST_LOCAL
	default:
		c.state = ST_INTERNET
	}
	return c
}

// ok: a witness gave its exact answer. reached: something answered, but wrongly.
func probeWitnesses() (ok, reached bool, best_ms float64) {
	type result struct {
		ok, reached bool
		ms          float64
	}
	results := make(chan result, len(WITNESSES))
	for _, w := range WITNESSES {
		go func(w witness) {
			ctx, cancel := context.WithTimeout(context.Background(), WITNESS_TIMEOUT)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", w.url, nil)
			req.Header.Set("User-Agent", "speedmon-collector/"+VERSION_STR)
			t0 := time.Now()
			res, err := no_keepalive.Do(req)
			if err != nil {
				results <- result{}
				return
			}
			defer res.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			ms := float64(time.Since(t0).Microseconds()) / 1000
			good := res.StatusCode == w.code && strings.Contains(string(body), w.body)
			results <- result{good, true, ms}
		}(w)
	}
	for range WITNESSES {
		r := <-results
		reached = reached || r.reached
		if r.ok && (!ok || r.ms < best_ms) {
			ok, best_ms = true, r.ms
		}
	}
	return ok, reached && !ok, best_ms
}

// "ok", "fail", or "unknown" when no gateway can be determined.
func gatewayAnswers() string {
	gw := defaultGateway()
	if gw == "" {
		return "unknown"
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = hiddenCommand("ping", "-n", "2", "-w", "1500", gw)
	} else {
		cmd = exec.Command("ping", "-c", "2", gw)
	}
	kill_timer := time.AfterFunc(6*time.Second, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	defer kill_timer.Stop()
	if err := cmd.Run(); err != nil {
		return "fail"
	}
	return "ok"
}

func defaultGateway() string {
	var out []byte
	var re *regexp.Regexp
	switch runtime.GOOS {
	case "darwin":
		out, _ = exec.Command("route", "-n", "get", "default").Output()
		re = regexp.MustCompile(`gateway:\s*(\S+)`)
	case "windows":
		out, _ = hiddenCommand("route", "print", "0.0.0.0").Output()
		re = regexp.MustCompile(`(?m)^\s*0\.0\.0\.0\s+0\.0\.0\.0\s+(\S+)`)
	default:
		out, _ = exec.Command("ip", "route", "show", "default").Output()
		re = regexp.MustCompile(`default via (\S+)`)
	}
	if m := re.FindSubmatch(out); m != nil {
		return string(m[1])
	}
	return ""
}

// Local address chosen for internet traffic. A UDP "dial" sends no packets.
func localIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// --- Measurement: which network
func networkInfo() netInfo {
	return netInfo{label: providerName(), iface: ifaceType(localIP())}
}

// Provider name via Cloudflare's trace (public IP) and Team Cymru's DNS lookup (owner).
func providerName() string {
	ctx, cancel := context.WithTimeout(context.Background(), WITNESS_TIMEOUT)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://1.1.1.1/cdn-cgi/trace", nil)
	res, err := no_keepalive.Do(req)
	if err != nil {
		return "unknown"
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	m := regexp.MustCompile(`(?m)^ip=(\d+)\.(\d+)\.(\d+)\.(\d+)$`).FindSubmatch(body)
	if m == nil {
		return "unknown" // IPv6 addresses are not looked up
	}
	var r net.Resolver
	host := fmt.Sprintf("%s.%s.%s.%s.origin.asn.cymru.com", m[4], m[3], m[2], m[1])
	txt, err := r.LookupTXT(ctx, host)
	if err != nil || len(txt) == 0 {
		return "unknown"
	}
	asn := strings.Fields(strings.SplitN(txt[0], "|", 2)[0])
	if len(asn) == 0 {
		return "unknown"
	}
	txt, err = r.LookupTXT(ctx, "AS"+asn[0]+".asn.cymru.com")
	if err != nil || len(txt) == 0 {
		return "AS" + asn[0]
	}
	return parseAsnName(txt[0], asn[0])
}

// "7922 | US | arin | 1997-02-14 | COMCAST-7922, US" gives "COMCAST-7922".
func parseAsnName(txt, asn string) string {
	parts := strings.Split(txt, "|")
	if len(parts) < 5 {
		return "AS" + asn
	}
	return strings.TrimSpace(strings.SplitN(parts[4], ",", 2)[0])
}

func ifaceType(ip string) string {
	ifaces, _ := net.Interfaces()
	name := ""
	for _, ifc := range ifaces {
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.String() == ip {
				name = ifc.Name
			}
		}
	}
	if name == "" {
		return "unknown"
	}
	low := strings.ToLower(name)
	switch runtime.GOOS {
	case "darwin":
		out, _ := exec.Command("networksetup", "-listallhardwareports").Output()
		if m := regexp.MustCompile(`Hardware Port: Wi-Fi\nDevice: (\S+)`).FindSubmatch(out); m != nil && string(m[1]) == name {
			return "wifi"
		}
	case "windows":
		if strings.Contains(low, "wi-fi") || strings.Contains(low, "wireless") {
			return "wifi"
		}
	default:
		if strings.HasPrefix(low, "wl") {
			return "wifi"
		}
	}
	return "wired"
}

// --- Measurement: speed. M-Lab first, Cloudflare only if M-Lab fails.
func measureSpeed(info netInfo) speedRow {
	r := speedRow{epoch: nowEpoch(), network: info.label, iface: info.iface, status: "ok", provider: "M-Lab"}
	down, up, ping, server, err := runMlab()
	if err == nil && !plausible(down, up) {
		err = fmt.Errorf("implausible result down=%.1f up=%.1f", down, up)
	}
	if err != nil {
		r.note = "M-Lab: " + oneLine(err.Error())
		r.provider = "Cloudflare"
		down, up, ping, err = runCloudflare()
		server = "speed.cloudflare.com"
		if err == nil && !plausible(down, up) {
			err = fmt.Errorf("implausible result down=%.1f up=%.1f", down, up)
		}
		if err != nil {
			r.status = "fail"
			r.note += "; Cloudflare: " + oneLine(err.Error())
			return r
		}
	}
	r.server = server
	r.down, r.up = fmt.Sprintf("%.2f", down), fmt.Sprintf("%.2f", up)
	if ping > 0 {
		r.ping = fmt.Sprintf("%.1f", ping)
	}
	return r
}

func plausible(down, up float64) bool {
	return down > 0 && down < MAX_DOWN_MBPS && up > 0 && up < MAX_UP_MBPS
}

func appMbps(m *spec.AppInfo) float64 {
	if m == nil || m.ElapsedTime <= 0 {
		return 0
	}
	return float64(m.NumBytes) * 8 / float64(m.ElapsedTime) // bits per microsecond = Mb/s
}

func runMlab() (down, up, ping float64, server string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), SPEED_TIMEOUT)
	defer cancel()
	c := ndt7.NewClient("speedmon-collector", VERSION_STR)
	ch, err := c.StartDownload(ctx)
	if err != nil {
		return
	}
	for range ch {
	}
	if res := c.Results()[spec.TestDownload]; res != nil {
		down = appMbps(res.Client.AppInfo)
		if res.Server.TCPInfo != nil {
			ping = float64(res.Server.TCPInfo.MinRTT) / 1000
		}
	}
	ch, err = c.StartUpload(ctx)
	if err != nil {
		return
	}
	for range ch {
	}
	if res := c.Results()[spec.TestUpload]; res != nil {
		up = appMbps(res.Client.AppInfo)
	}
	return down, up, ping, c.FQDN, ctx.Err()
}

func runCloudflare() (down, up, ping float64, err error) {
	if _, _, ms := probeWitnesses(); ms > 0 {
		ping = ms
	}
	if down, err = cloudflareDirection(false); err != nil {
		return
	}
	up, err = cloudflareDirection(true)
	return
}

// Repeated transfers for 8 seconds, capped so a test cannot use unbounded data.
func cloudflareDirection(upload bool) (float64, error) {
	const size = 8 * 1024 * 1024
	payload := make([]byte, size)
	if upload {
		rand.Read(payload)
	}
	start := time.Now()
	total := int64(0)
	for time.Since(start) < 8*time.Second && total < CLOUDFLARE_CAP_BYTES {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		method, url := "GET", "https://speed.cloudflare.com/__down?bytes="+strconv.Itoa(size)
		var body io.Reader
		if upload {
			method, url, body = "POST", "https://speed.cloudflare.com/__up", bytes.NewReader(payload)
		}
		req, _ := http.NewRequestWithContext(ctx, method, url, body)
		res, err := bulk_client.Do(req)
		if err != nil {
			cancel()
			return 0, err
		}
		n, err := io.Copy(io.Discard, res.Body)
		res.Body.Close()
		cancel()
		if err != nil {
			return 0, err
		}
		if res.StatusCode != 200 {
			return 0, fmt.Errorf("HTTP %d", res.StatusCode)
		}
		if upload {
			n = size
		}
		total += n
	}
	return float64(total) * 8 / time.Since(start).Seconds() / 1e6, nil
}

// --- Log files
func machineDir(cfg config) string { return filepath.Join(cfg.Root, "data", safeName(cfg.Machine)) }

func safeName(s string) string {
	out := regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(s, "_")
	if out = strings.Trim(out, "._"); out == "" {
		return "machine"
	}
	return out
}

var CHECK_HEADER = []string{"epoch", "state", "rtt_ms", "network"}
var SPEED_HEADER = []string{"epoch", "status", "provider", "down_mbps", "up_mbps", "ping_ms", "server", "network", "iface", "note"}

func appendCheck(cfg config, c check) {
	logErr(appendRow(cfg, "checks", CHECK_HEADER, []string{fmtEpoch(c.epoch), c.state, c.rtt, c.network}))
}

func appendSpeed(cfg config, r speedRow) {
	logErr(appendRow(cfg, "speed", SPEED_HEADER, speedFields(r)))
}

func speedFields(r speedRow) []string {
	return []string{fmtEpoch(r.epoch), r.status, r.provider, r.down, r.up, r.ping, r.server, r.network, r.iface, r.note}
}

func appendRow(cfg config, kind string, header, row []string) error {
	dir := machineDir(cfg)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, kind+"_"+time.Now().Format("2006-01")+".csv")
	_, stat_err := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if os.IsNotExist(stat_err) {
		w.Write(header)
	}
	w.Write(row)
	w.Flush()
	return w.Error()
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Rows of every matching file; short or damaged rows are skipped.
func readRows(pattern string, fields int) [][]string {
	files, _ := filepath.Glob(pattern)
	sort.Strings(files)
	var rows [][]string
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		r := csv.NewReader(bufio.NewReader(f))
		r.FieldsPerRecord = -1
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			if len(row) >= fields && row[0] != "epoch" {
				rows = append(rows, row)
			}
		}
		f.Close()
	}
	return rows
}

func lastSpeedEpoch(cfg config) float64 {
	rows := readRows(filepath.Join(machineDir(cfg), "speed_*.csv"), 2)
	if len(rows) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(rows[len(rows)-1][0], 64)
	return v
}

// --- Viewer data: derived from the CSVs every time, so it can always be rebuilt.
type hourKey struct {
	hour    int64
	network string
}
type hourStat struct {
	ok, bad, other int
	rtts           []float64
}

// event: [start, end, kind, network, ongoing(0/1)]
func deriveFromChecks(checks []check, limit float64) (events [][]any, hours map[hourKey]*hourStat) {
	hours = map[hourKey]*hourStat{}
	events = [][]any{} // JSON [] rather than null
	settle_until := 0.0
	var run *check
	var run_last float64
	run_count := 0
	closeRun := func(end float64, ongoing int) {
		if run != nil && run_count >= MIN_OUTAGE_CHECKS {
			events = append(events, []any{run.epoch, end, run.state, run.network, ongoing})
		}
		run, run_count = nil, 0
	}
	for i := range checks {
		c := &checks[i]
		if i == 0 {
			settle_until = c.epoch + WAKE_SETTLE_SECS
		} else if prev := checks[i-1].epoch; c.epoch-prev > limit {
			closeRun(run_last, 0)
			events = append(events, []any{prev, c.epoch, "unmonitored", "", 0})
			settle_until = c.epoch + WAKE_SETTLE_SECS
		}
		key := hourKey{int64(c.epoch) / 3600 * 3600, c.network}
		h := hours[key]
		if h == nil {
			h = &hourStat{}
			hours[key] = h
		}
		if c.state == ST_ONLINE {
			h.ok++
			if v, err := strconv.ParseFloat(c.rtt, 64); err == nil {
				h.rtts = append(h.rtts, v)
			}
			closeRun(c.epoch, 0)
			continue
		}
		if c.epoch < settle_until { // reconnecting after sleep; neither up nor down
			continue
		}
		if c.state == ST_INTERNET || c.state == ST_LOCAL {
			h.bad++
		} else {
			h.other++
		}
		if run != nil && run.state != c.state {
			closeRun(c.epoch, 0)
		}
		if run == nil {
			run = c
		}
		run_last = c.epoch
		run_count++
	}
	closeRun(run_last, 1)
	return events, hours
}

func parseChecks(rows [][]string) []check {
	out := make([]check, 0, len(rows))
	for _, r := range rows {
		if v, err := strconv.ParseFloat(r[0], 64); err == nil {
			out = append(out, check{v, r[1], r[2], r[3]})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].epoch < out[b].epoch })
	return out
}

func buildViewerFiles(cfg config) error {
	dir := machineDir(cfg)
	st := loadSettings(cfg.Root)
	checks := parseChecks(readRows(filepath.Join(dir, "checks_*.csv"), 4))
	events, hours := deriveFromChecks(checks, gapLimit(st.CheckSeconds))
	// Imported hourly history has no sleep information, so it only adds to availability.
	for _, c := range parseChecks(readRows(filepath.Join(dir, "legacy_checks.csv"), 4)) {
		key := hourKey{int64(c.epoch) / 3600 * 3600, c.network}
		if hours[key] == nil {
			hours[key] = &hourStat{}
		}
		if c.state == ST_ONLINE {
			hours[key].ok++
		} else {
			hours[key].bad++
		}
	}
	hour_rows := make([][]any, 0, len(hours))
	for k, h := range hours {
		hour_rows = append(hour_rows, []any{k.hour, k.network, h.ok, h.bad, h.other, median(h.rtts)})
	}
	sort.Slice(hour_rows, func(a, b int) bool { return hour_rows[a][0].(int64) < hour_rows[b][0].(int64) })
	speed := [][]string{}
	speed = append(speed, readRows(filepath.Join(dir, "legacy_speed.csv"), 10)...)
	speed = append(speed, readRows(filepath.Join(dir, "speed_*.csv"), 10)...)
	recent := [][]any{}
	cutoff := nowEpoch() - RECENT_SECS
	for _, c := range checks {
		if c.epoch >= cutoff {
			recent = append(recent, []any{c.epoch, c.state, c.rtt, c.network})
		}
	}
	payload, err := json.Marshal(map[string]any{
		"machine": cfg.Machine, "updated": nowEpoch(), "check_seconds": st.CheckSeconds, "speed_hours": st.SpeedHours,
		"speed": speed, "hours": hour_rows, "recent": recent, "events": events,
	})
	if err != nil {
		return err
	}
	name := safeName(cfg.Machine)
	out := filepath.Join(cfg.Root, "viewer_data")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	js := fmt.Sprintf("(window.SPEEDMON_DATA=window.SPEEDMON_DATA||{})[%q]=%s;\n", name, payload)
	if err := writeAtomic(filepath.Join(out, name+".js"), []byte(js)); err != nil {
		return err
	}
	if err := writeAnnotations(cfg.Root); err != nil {
		return err
	}
	return writeMachineIndex(cfg.Root)
}

// annotations.csv is the hand-kept history of equipment, service and method changes.
func writeAnnotations(root string) error {
	rows := readCSVMaps(filepath.Join(root, "annotations.csv"))
	if rows == nil {
		rows = []map[string]string{}
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return writeIfChanged(filepath.Join(root, "viewer_data", "annotations.js"), []byte("window.SPEEDMON_ANNOTATIONS="+string(b)+";\n"))
}

func writeIfChanged(path string, b []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b) {
		return nil // no write means no needless Dropbox sync or conflict
	}
	return writeAtomic(path, b)
}

// The page cannot list a folder from file://, so it reads this index. Its content depends
// only on the folder names, so copies from different computers converge.
func writeMachineIndex(root string) error {
	entries, err := os.ReadDir(filepath.Join(root, "data"))
	if err != nil {
		return err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	b, _ := json.Marshal(names)
	js := []byte("window.SPEEDMON_MACHINES=" + string(b) + ";\n")
	return writeIfChanged(filepath.Join(root, "viewer_data", "machines.js"), js)
}

func median(v []float64) any {
	if len(v) == 0 {
		return nil
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// --- Import of earlier browser, Python and LiveCode history (repeatable; deduped by minute)
var TS_RE = regexp.MustCompile(`^\d+/\d+/\d+ \d+:\d+ [AP]M$`)

func parseLegacyTime(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if !TS_RE.MatchString(s) {
		return 0, false
	}
	t, err := time.ParseInLocation("1/2/06 3:04 PM", s, time.Local)
	if err != nil {
		return 0, false
	}
	return float64(t.Unix()), true
}

func classifyLegacy(down, up, result string) (status, d, u string) {
	dv, e1 := strconv.ParseFloat(down, 64)
	uv, e2 := strconv.ParseFloat(up, 64)
	low := strings.ToLower(result)
	if strings.HasPrefix(low, "dropout") {
		return "drop", "", ""
	}
	if e1 == nil && e2 == nil && plausible(dv, uv) && !strings.HasPrefix(low, "fail") {
		return "ok", down, up
	}
	return "fail", "", ""
}

const NOT_RECORDED = "Not recorded"
const MAX_PING_MS = 2000.0

// Blank rather than guess: missing values stay missing.
func cleanPing(s string) string {
	if v, err := strconv.ParseFloat(s, 64); err != nil || v < 0 || v > MAX_PING_MS {
		return ""
	}
	return s
}

// The old Python tool wrote "cable" on every row whatever the real link was.
func cleanIface(row map[string]string) string {
	if strings.Contains(row["browser"], "python") {
		return ""
	}
	return row["iface"]
}

// Per minute: whether any old site check answered, and whether checks ran at all.
func legacySiteChecks(root string) (up, seen map[int64]bool) {
	up, seen = map[int64]bool{}, map[int64]bool{}
	files, _ := filepath.Glob(filepath.Join(root, "speedtest_logs", "urlcheck_*.csv"))
	for _, path := range files {
		for _, row := range readCSVMaps(path) {
			if epoch, ok := parseLegacyTime(row["timestamp"]); ok {
				minute := int64(epoch) / 60
				seen[minute] = true
				up[minute] = up[minute] || row["state"] == "up"
			}
		}
	}
	return up, seen
}

// Which old tool wrote a row. Tools read differently, so this must survive the import.
func legacyTool(browser string) string {
	if strings.Contains(browser, "python") {
		return "Old Python tool"
	}
	return "Old browser page"
}

func importLegacy(root, default_machine string) error {
	speed := map[string]map[int64]speedRow{}
	add := func(machine string, r speedRow) {
		if machine == "" || machine == "macOS" { // old pages used the OS name as a placeholder
			machine = default_machine
		}
		if speed[machine] == nil {
			speed[machine] = map[int64]speedRow{}
		}
		key := int64(r.epoch) / 60
		if old, seen := speed[machine][key]; !seen || old.status != "ok" {
			speed[machine][key] = r
		}
	}
	sites_up, seen_sites := legacySiteChecks(root)
	files, _ := filepath.Glob(filepath.Join(root, "speedtest_logs", "speedtest_*.csv"))
	for _, path := range files {
		for _, row := range readCSVMaps(path) {
			epoch, ok := parseLegacyTime(row["timestamp"])
			if v, err := strconv.ParseFloat(row["epoch"], 64); err == nil && v > 0 {
				epoch, ok = v, true
			}
			if !ok || (row["type"] != "" && row["type"] != "full") {
				continue
			}
			machine := row["machine"]
			if row["download_mbps"] == "" && row["upload_mbps"] == "" && row["result"] == "" {
				continue // an old ping-only row, not a failed speed test
			}
			status, d, u := classifyLegacy(row["download_mbps"], row["upload_mbps"], row["result"])
			if status == "fail" && !sites_up[int64(epoch)/60] && seen_sites[int64(epoch)/60] {
				status = "drop" // the test failed and no site answered at that time either
			}
			add(machine, speedRow{epoch: epoch, status: status, provider: legacyTool(row["browser"]), down: d, up: u,
				ping: cleanPing(row["ping_ms"]), server: row["server"], network: NOT_RECORDED, iface: cleanIface(row), note: oneLine(row["result"])})
		}
	}
	importLiveCode(add)
	for machine, rows := range speed {
		if err := writeLegacySpeed(root, machine, rows); err != nil {
			return err
		}
		fmt.Printf("imported %d speed rows for %s\n", len(rows), machine)
		if err := buildViewerFiles(config{machine, root}); err != nil {
			return err
		}
	}
	return importLegacyChecks(root, default_machine)
}

// The earlier LiveCode app kept its own history outside Dropbox on this computer.
func importLiveCode(add func(string, speedRow)) {
	base, err := os.UserConfigDir()
	if err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(base, "NineTrees", "SpeedMonitor", "history.json"))
	if err != nil {
		return
	}
	var store struct {
		Records []struct {
			Epoch                 float64
			Type, Machine, Status string
			Provider, Server      string
			Note                  string
			Down, Up, Ping        *float64
		}
	}
	if json.Unmarshal(b, &store) != nil {
		return
	}
	for _, r := range store.Records {
		if r.Type == "ping" || r.Epoch == 0 || r.Provider == "" && r.Down == nil {
			continue
		}
		if r.Provider == "" {
			r.Provider = "Unknown tool"
		}
		if r.Provider == "Cloudflare HTTP" { // the same test under an older name
			r.Provider = "Cloudflare"
		}
		if r.Provider == "Legacy / unspecified" {
			r.Provider = "Unknown tool"
		}
		row := speedRow{epoch: r.Epoch, status: "fail", provider: r.Provider, server: r.Server, network: NOT_RECORDED, note: oneLine(r.Note)}
		if r.Down != nil && r.Up != nil && plausible(*r.Down, *r.Up) {
			row.status, row.down, row.up = "ok", fmt.Sprintf("%.2f", *r.Down), fmt.Sprintf("%.2f", *r.Up)
		}
		if r.Ping != nil {
			row.ping = cleanPing(fmt.Sprintf("%.1f", *r.Ping))
		}
		add(r.Machine, row)
	}
}

func writeLegacySpeed(root, machine string, rows map[int64]speedRow) error {
	dir := filepath.Join(root, "data", safeName(machine))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	list := make([]speedRow, 0, len(rows))
	for _, r := range rows {
		list = append(list, r)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].epoch < list[b].epoch })
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write(SPEED_HEADER)
	for _, r := range list {
		w.Write(speedFields(r))
	}
	w.Flush()
	return writeAtomic(filepath.Join(dir, "legacy_speed.csv"), buf.Bytes())
}

// Old site checks: online if any site answered at that time.
func importLegacyChecks(root, machine string) error {
	files, _ := filepath.Glob(filepath.Join(root, "speedtest_logs", "urlcheck_*.csv"))
	up := map[int64]bool{}
	for _, path := range files {
		for _, row := range readCSVMaps(path) {
			if epoch, ok := parseLegacyTime(row["timestamp"]); ok {
				up[int64(epoch)] = up[int64(epoch)] || row["state"] == "up"
			}
		}
	}
	if len(up) == 0 {
		return nil
	}
	times := make([]int64, 0, len(up))
	for t := range up {
		times = append(times, t)
	}
	sort.Slice(times, func(a, b int) bool { return times[a] < times[b] })
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write(CHECK_HEADER)
	for _, t := range times {
		state := ST_INTERNET
		if up[t] {
			state = ST_ONLINE
		}
		w.Write([]string{strconv.FormatInt(t, 10), state, "", NOT_RECORDED})
	}
	w.Flush()
	fmt.Printf("imported %d site-check times for %s\n", len(times), machine)
	return writeAtomic(filepath.Join(root, "data", safeName(machine), "legacy_checks.csv"), buf.Bytes())
}

func readCSVMaps(path string) []map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var header []string
	var out []map[string]string
	for {
		row, err := r.Read()
		if err == io.EOF {
			return out
		}
		if err != nil || len(row) == 0 || strings.HasPrefix(row[0], "#") {
			continue
		}
		if header == nil {
			header = row
			continue
		}
		m := map[string]string{}
		for i, h := range header {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		out = append(out, m)
	}
}

// --- Small helpers
func nowEpoch() float64         { return float64(time.Now().Unix()) }
func fmtEpoch(v float64) string { return strconv.FormatInt(int64(v), 10) }
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
func fatal(msg string) { fmt.Fprintln(os.Stderr, msg); os.Exit(1) }
func logErr(err error) {
	if err != nil {
		log.Print(err)
	}
}
func mustLog(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

// --- Settings shared across computers through Dropbox
func loadSettings(root string) settings {
	st := settings{CheckSeconds: 60, SpeedHours: 4}
	if b, err := os.ReadFile(filepath.Join(root, "collector_settings.json")); err == nil {
		json.Unmarshal(b, &st)
	}
	if st.CheckSeconds < 15 {
		st.CheckSeconds = 15
	}
	if st.SpeedHours < 1 { // full tests use real data and M-Lab limits frequent clients
		st.SpeedHours = 1
	}
	return st
}

// --- Install: runs at login, invisibly, from a local copy Dropbox cannot replace mid-run
func appDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		fatal(err.Error())
	}
	return filepath.Join(base, "speedmon")
}

func loadConfig() config {
	b, err := os.ReadFile(filepath.Join(appDir(), "config.json"))
	var cfg config
	if err != nil || json.Unmarshal(b, &cfg) != nil || cfg.Root == "" {
		fatal("not installed yet: run  speedmon-collector install --machine \"Name\"")
	}
	return cfg
}

func install(machine, root string) {
	exe, err := os.Executable()
	mustLog(err)
	exe, _ = filepath.EvalSymlinks(exe)
	if root == "" { // <root>/collector/bin/<binary>
		root = filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	}
	if _, err := os.Stat(filepath.Join(root, "speed_monitor.html")); err != nil {
		fatal("project folder not found at " + root + "; pass --root")
	}
	if machine == "" {
		machine, _ = os.Hostname()
		machine = strings.TrimSuffix(machine, ".local")
	}
	dir := appDir()
	mustLog(os.MkdirAll(dir, 0o755))
	b, _ := json.MarshalIndent(config{machine, root}, "", "  ")
	mustLog(writeAtomic(filepath.Join(dir, "config.json"), b))
	stopService()
	src := exe
	name := "speedmon-collector"
	if runtime.GOOS == "windows" { // the windowless build has no console to flash
		name += ".exe"
		src = strings.TrimSuffix(exe, ".exe") + "-bg.exe"
	}
	dest := filepath.Join(dir, name)
	data, err := os.ReadFile(src)
	mustLog(err)
	mustLog(writeAtomic(dest, data))
	mustLog(os.Chmod(dest, 0o755))
	mustLog(startService(dest, dir))
	fmt.Printf("installed: machine=%q root=%q\nOpen speed_monitor.html in that folder to see the graphs.\n", machine, root)
}

func uninstall() {
	stopService()
	switch runtime.GOOS {
	case "darwin":
		os.Remove(launchPlistPath())
	case "windows":
		hiddenCommand("reg", "delete", WIN_RUN_KEY, "/v", SERVICE_NAME, "/f").Run()
	default:
		os.Remove(systemdUnitPath())
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	fmt.Println("uninstalled; logs in Dropbox were left in place")
}

const SERVICE_NAME = "SpeedMonitorCollector"
const LAUNCH_LABEL = "com.ninetreesdesign.speedmon-collector"
const WIN_RUN_KEY = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func launchPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", LAUNCH_LABEL+".plist")
}
func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "speedmon-collector.service")
}

func stopService() {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), LAUNCH_LABEL)).Run()
	case "windows":
		hiddenCommand("taskkill", "/IM", "speedmon-collector.exe", "/F").Run()
	default:
		exec.Command("systemctl", "--user", "disable", "--now", "speedmon-collector").Run()
	}
}

func startService(bin, dir string) error {
	log_path := filepath.Join(dir, "collector.log")
	switch runtime.GOOS {
	case "darwin":
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>run</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>60</integer>
<key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, LAUNCH_LABEL, xmlEscape(bin), xmlEscape(log_path), xmlEscape(log_path))
		if err := writeAtomic(launchPlistPath(), []byte(plist)); err != nil {
			return err
		}
		out, err := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), launchPlistPath()).CombinedOutput()
		if err != nil {
			return errors.New("launchctl: " + string(out))
		}
		return exec.Command("launchctl", "kickstart", fmt.Sprintf("gui/%d/%s", os.Getuid(), LAUNCH_LABEL)).Run()
	case "windows":
		value := `"` + bin + `" run`
		if out, err := hiddenCommand("reg", "add", WIN_RUN_KEY, "/v", SERVICE_NAME, "/t", "REG_SZ", "/d", value, "/f").CombinedOutput(); err != nil {
			return errors.New("reg: " + string(out))
		}
		return exec.Command(bin, "run").Start()
	default:
		unit := fmt.Sprintf("[Unit]\nDescription=Speed Monitor collector\n[Service]\nExecStart=%q run\nRestart=always\nRestartSec=60\nNice=10\n[Install]\nWantedBy=default.target\n", bin)
		if err := os.MkdirAll(filepath.Dir(systemdUnitPath()), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(systemdUnitPath(), []byte(unit)); err != nil {
			return err
		}
		exec.Command("systemctl", "--user", "daemon-reload").Run()
		return exec.Command("systemctl", "--user", "enable", "--now", "speedmon-collector").Run()
	}
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// /// EOF
