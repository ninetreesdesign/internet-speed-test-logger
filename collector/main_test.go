package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func seq(start float64, step float64, states ...string) []check {
	out := []check{}
	for i, s := range states {
		out = append(out, check{epoch: start + float64(i)*step, state: s, network: "N"})
	}
	return out
}

func TestOutageNeedsTwoAwakeChecks(t *testing.T) {
	c := seq(0, 60, ST_ONLINE, ST_ONLINE, ST_ONLINE, ST_INTERNET, ST_ONLINE, ST_INTERNET, ST_INTERNET, ST_ONLINE)
	ev, _ := deriveFromChecks(c, 200)
	if len(ev) != 1 || ev[0][2] != ST_INTERNET || ev[0][0] != 300.0 || ev[0][1] != 420.0 {
		t.Fatalf("events %v", ev)
	}
}

func TestSleepIsNotOutage(t *testing.T) {
	c := append(seq(0, 60, ST_ONLINE, ST_ONLINE, ST_ONLINE), seq(10000, 60, ST_NO_NET, ST_INTERNET, ST_ONLINE)...)
	ev, h := deriveFromChecks(c, 200)
	if len(ev) != 1 || ev[0][2] != "unmonitored" {
		t.Fatalf("events %v", ev)
	}
	for _, s := range h {
		if s.bad != 0 {
			t.Fatalf("wake failures counted %+v", s)
		}
	}
}

func TestStartupSettleAndOngoing(t *testing.T) {
	c := seq(0, 60, ST_INTERNET, ST_ONLINE, ST_ONLINE, ST_LOCAL, ST_LOCAL)
	ev, _ := deriveFromChecks(c, 200)
	if len(ev) != 1 || ev[0][2] != ST_LOCAL || ev[0][4] != 1 {
		t.Fatalf("events %v", ev)
	}
}

func TestNoNetworkAndCaptiveAreSeparate(t *testing.T) {
	c := seq(0, 60, ST_ONLINE, ST_ONLINE, ST_ONLINE, ST_NO_NET, ST_NO_NET, ST_CAPTIVE, ST_CAPTIVE, ST_ONLINE)
	ev, h := deriveFromChecks(c, 200)
	if len(ev) != 2 || ev[0][2] != ST_NO_NET || ev[1][2] != ST_CAPTIVE {
		t.Fatalf("events %v", ev)
	}
	for _, s := range h {
		if s.bad != 0 {
			t.Fatalf("not outages %+v", s)
		}
	}
}

func TestLegacyParsing(t *testing.T) {
	if _, ok := parseLegacyTime("# 3/28/26 9:58 PM STARTED"); ok {
		t.Fatal("comment accepted")
	}
	if _, ok := parseLegacyTime("10/1/26 12:19 AM"); !ok {
		t.Fatal("time rejected")
	}
	if s, _, _ := classifyLegacy("", "", "dropout: confirmed"); s != "drop" {
		t.Fatal(s)
	}
	if s, _, _ := classifyLegacy("996531233", "1", ""); s != "fail" {
		t.Fatal(s)
	}
	if s, d, _ := classifyLegacy("100", "10", ""); s != "ok" || d != "100" {
		t.Fatal(s)
	}
}

func TestParseAsnName(t *testing.T) {
	if n := parseAsnName("7922 | US | arin | 1997-02-14 | COMCAST-7922, US", "7922"); n != "COMCAST-7922" {
		t.Fatal(n)
	}
}

func TestBuildAndDamagedRow(t *testing.T) {
	root := t.TempDir()
	cfg := config{"Test Box", root}
	dir := filepath.Join(root, "data", "Test_Box")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "checks_2026-11.csv"), []byte("epoch,state,rtt_ms,network\n100,online,12,N\n160,onl\n"), 0o644)
	if err := buildViewerFiles(cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "viewer_data", "machines.js"))
	if err != nil || string(b) != "window.SPEEDMON_MACHINES=[\"Test_Box\"];\n" {
		t.Fatalf("%s %v", b, err)
	}
}

func TestDatabaseBuildAndViews(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data", "Box")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "speed_2026-11.csv"), []byte("epoch,status,provider,down_mbps,up_mbps,ping_ms,server,network,iface,note\n1793857650,ok,M-Lab,100,10,5,s,N,wifi,\n1793857999,ok,M-Lab,300,30,5,s,N,wifi,\n1793858999,fail,M-Lab,,,,,N,wifi,x\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "checks_2026-11.csv"), []byte("epoch,state,rtt_ms,network\n1793857650,online,12,N\n"), 0o644)
	os.WriteFile(filepath.Join(root, "annotations.csv"), []byte("id,start,end,scope,kind,text,source,confidence\nA1,2026-09-03,,x,y,z,s,c\n"), 0o644)
	path := filepath.Join(root, "out", "t.db")
	if _, err := buildDatabase(root, path); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", path)
	defer db.Close()
	var tests int
	var med float64
	if err := db.QueryRow(`SELECT tests, down_median FROM speed_monthly`).Scan(&tests, &med); err != nil {
		t.Fatal(err)
	}
	if tests != 2 || med != 100 {
		t.Fatalf("tests=%d median=%v", tests, med)
	}
	var total int
	db.QueryRow(`SELECT (SELECT COUNT(*) FROM speed_test)+(SELECT COUNT(*) FROM connectivity_check)+(SELECT COUNT(*) FROM annotation)`).Scan(&total)
	if total != 5 {
		t.Fatalf("total=%d", total)
	}
}
