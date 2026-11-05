# Internet Speed Monitor

## Current design (v3, 2026-11-04)

Two parts, no server and no open tab:

- **Collector** (`collector/`): runs invisibly from login on each computer. It checks
  connectivity every minute and runs a speed test about every 4 hours, then appends to
  `data/<computer>/`, which Dropbox syncs.
- **Viewer** (`speed_monitor.html`): double-click to see every computer's logs and graphs. Keep
  it with `vendor/` and `viewer_data/`. It reloads its data every 5 minutes. Its Ping and Full
  Test buttons run immediately in that browser and are stored only there; it runs no scheduled
  tests. Export and Import cover those manual tests only. `archive/speed_viewer.html` is an
  alternate layout with downtime bars and an outage table.

Tested in Chrome and Firefox on Mac. Safari, Windows and Linux are not yet tested. Previews inside
other apps cannot run the page; use a real browser window.

### Install on a computer

Run once from `collector/bin/`, using the file for that system:

    ./speedmon-collector-mac-arm64 install --machine "Mac M4 Laptop"
    speedmon-collector-win.exe install --machine "Dell"
    ./speedmon-collector-linux-amd64 install --machine "Debix"

The file is copied to the local app folder (`speedmon` under the user config folder),
with its log `collector.log`. `uninstall` removes the login item and keeps the logs.
After changing code, run `collector/build_all.sh`, then `install` again on each computer.

`collector_settings.json` applies to all computers: `check_seconds`, and `speed_hours`
(minimum 1). A full test on a gigabit line uses about 1.3 GB, so an hourly schedule is
close to 1 TB a month per computer.

### What is measured

- **Connectivity:** three connectivity endpoints (Google, Cloudflare, Apple), any correct
  answer means online. If none answer, it retries after 10 seconds and pings the router.
- **Speed:** M-Lab NDT7, with Cloudflare only if M-Lab fails; the service is logged per row.
  Test times are randomized by 15% because M-Lab refuses strictly periodic clients.
- **Response time** is the fastest endpoint's HTTP response time, not ICMP ping.
- **Network label:** owner of the public IPv4 address, from Team Cymru DNS, plus Wi-Fi or wired.

### Outage rules

States: `online`, `internet` (router fine or unknown), `local` (router silent), `no_network`,
`captive` (sign-in page). Only `internet` and `local` count as downtime, and an outage needs
two checks in a row. A gap over about 3 checks is logged as asleep or off, and failures in the
2 minutes after a gap are ignored while the network reconnects.

### Keeping history meaningful

`annotations.csv` records changes in equipment, service, computers and method:
`id,start,end,scope,kind,text,source,confidence`, with dates as YYYY-MM-DD. Add a line when
something changes. They appear as markers on the graph and rows in the log.

Each speed row keeps the tool that measured it, and the viewer filters by tool. Compare trends
within one tool only. In October 2026 the old Python tool, Cloudflare and M-Lab read about 175,
335 and 500 on the same days.

### Planned, when the Linux box is ready

- Run the collector on the box's host OS, wired to the router, as the long-term reference.
- Per-computer `speed_hours`: the box about every 4 hours, laptops rare or manual.
- Temporary watch mode (`watch 24h`): checks every 15 seconds, self-expiring, auto-annotated.
  Speed tests only after an outage, on sustained high response time, or on a network change,
  with a daily cap, and each logs why it ran.
- Then: Place field, status line, monthly summary, time-of-day heatmap, before/after Vero.

### Imported history

`import` cleans a copy into `data/<computer>/legacy_*.csv`; `speedtest_logs/` is never changed.
Missing values stay blank: network is `Not recorded`, the old Python tool's fixed `cable` label is
blanked, out-of-range ping is blanked, and a failed test with no site answering becomes `drop`.

### SQLite database

The collector rebuilds a local database hourly from all computers' CSVs and `annotations.csv`,
as `speedmon.db` in its app folder. It stays off Dropbox and can be deleted at any time. The
CSVs remain the source of truth. `collector/schema.sql` holds the tables and the views
`speed_ok`, `speed_monthly` and `speed_by_hour`. The HTML page still reads `viewer_data/`.

    speedmon-collector db
    speedmon-collector query "SELECT * FROM speed_monthly"

Compare within one tool; `tool.comparable_group` marks which results belong together.

### Data files, for the viewer or LiveCode

`data/<computer>/`:

- `checks_YYYY-MM.csv`: `epoch,state,rtt_ms,network`
- `speed_YYYY-MM.csv`, `legacy_speed.csv`: `epoch,status,provider,down_mbps,up_mbps,ping_ms,server,network,iface,note`
- `legacy_checks.csv`: imported site checks, same columns as checks

`viewer_data/machines.js` lists computers. `viewer_data/<computer>.js` is JSON after the first
`=` on its line (drop the final `;`), with `speed`, `hours`
`[hour,network,ok,bad,other,median_ms]`, `recent` (8 days of checks), and `events`
`[start,end,kind,network,ongoing]`. It is rebuilt from the CSVs every 10 minutes.

`speedmon-collector import` reloads earlier history from `speedtest_logs/`, including browser
CSV exports saved there. Imported history has no sleep information, so it counts toward
availability but does not list outages.

Everything below is history: it describes the earlier browser and Python versions, which are retired.

## Earlier workflow (v2.3b, 2026-09-26)

Double-click `speed_monitor.html`; no server is needed. Keep it with `vendor/`.
The workers run from `vendor/ndt7-workers-embedded.js` as Blob workers; after changing
a worker file, run `python3 vendor/build_embedded_workers.py`. Keep the same browser
profile and file path to retain that page's localStorage. Export/import CSV to move history
between profiles, browsers, or machines; localStorage itself is not Dropbox-synced.

Pinned local dependencies: M-Lab `@m-lab/ndt7` 0.1.5 and Chart.js 4.4.0. The client
and both workers are exact upstream assets; provenance, hashes, and licenses are
in `vendor/README.md`. Internet access is still required for measurements.

### v2.3b repairs

- Both browser directions use the final client `MeanClientMbps`, not peak or raw
  byte rates. Finite plausibility limits are 2000 Mbps down, 500 Mbps up, and
  2000 ms ping; invalid history/imports become excluded failures with diagnostic
  original values retained. Missing ping is not zero.
- CSV preserves event types, confirmed dropouts, precise timestamps, multiline
  quoted fields, and distinct observations. Repeated imports deduplicate.
- Pause/Stop cancel active browser probes/workers and prevent stale completion
  from rearming timers. Recurring slots use absolute deadlines, skip missed
  slots, and honor a full-test interval shorter than the ping interval.
- Witness confirmation always uses ninetrees.com, cambrianworks.com, apple.com:
  any reachable site means online; all failing twice, 10 seconds apart, means
  dropout. Editable URL slots remain independent diagnostics.
- Availability is a percentage of known connectivity observations, not elapsed
  uptime. Known-reachable tool failures count as reachable; unknowns are excluded.
  Graphs distinguish failures/gaps from confirmed outages, preserve measured values,
  and keep ping on a separate scale.
- Fixed popup HTML escaping, toolbar overflow, narrow-screen formatting, and
  Windows-incompatible Python date formatting. Pinned local chart assets allow
  history viewing without a CDN connection.
- Background summaries validate units/ranges and tolerate absent latency. Existing
  CSV files with older headers remain untouched; new-schema rows go to a `_v23.csv`
  companion when needed. The background diagnostic log rotates at 2 MB with three
  backups. LaunchAgent bootstrap errors go to `speedmon_launchd.log`.

### Shared CSV contract

Speed: `timestamp,epoch,type,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result,server`

`epoch` is Unix seconds, including fractions; imports also accept milliseconds.
Types: `full`, `ping`, `event` (legacy `evt` accepted). Results: empty for success,
`dropout: confirmed`, or `fail: <reason>`; event rows contain event text.
`[conn_ok=true]` on a failure preserves positive connectivity evidence across CSV.
Legacy v1.x/v1.6/v1.8 logs remain importable. URL logs are separate and are not
imported into speed history.

URL: `timestamp,url,state,ms,conn_ok,round`. States: `up`, `timeout`, `down`;
connectivity: `true`, `false`, or blank/unknown; round: `initial`, `confirm`, or
blank for historical rows. One unsuccessful initial round is not a dropout.

### Validation and remaining limits

- Passed 9 JavaScript regression tests and 5 Python tests covering CSV/history,
  invalid values, measurements, connectivity, cancellation, and scheduling.
- Passed extracted HTML JavaScript, client/worker syntax, Python compilation, and
  plist checks. Workers match the pinned upstream files byte-for-byte.
- Browser UI checked at desktop and 390 px widths in the available Chromium
  in-app browser; live Stop returned controls to idle without a new test record.
- One live run completed: **205.2 Mbps down, 11.8 Mbps up, 53.1 ms**, M-Lab
  San Francisco Bay Area (2026-09-25 22:41 PDT). This is below the established
  direct reference of about 950-1030/35 Mbps and 10-25 ms. VPN/path conditions were
  not established; reference-range accuracy is **not yet confirmed**.
- Safari native testing was blocked by unavailable computer-control permission.
  Firefox and Windows runtime checks were not performed; portability fixes have
  automated/source coverage only. The Go NDT7 client is not installed here, so
  the background final-summary parser was tested with fixtures, not a live CLI run.
- Web Locks prevent overlapping tests in same-origin tabs where supported.
  Browser versus background, separate browsers, and separate machines still need
  nonoverlapping schedules; there is no cross-process bandwidth lock.
- The plist remains specific to this Mac. Check Python/project paths on another
  Mac; it is not a Windows service installer. `ThrottleInterval` remains 300.
  Changes to a loaded LaunchAgent require unloading/reloading it; that was not done.

Run regression checks from this folder:

```sh
node --test tests/monitor.test.cjs
python3 -m unittest discover -s tests -p 'test_*.py'
plutil -lint speedmon_bg.plist  # macOS
```

The following is historical context; the v2.3 server-side upload preference is
superseded by the v2.3b final-client measurement contract above.

## Version History

### v2.3 (2026-09-25)

- Bundled the two required NDT7 Web Workers locally and passed their paths explicitly
- Browser bandwidth now uses the final completed NDT7 measurements, not the peak running value
- Upload prefers the final server-side TCPInfo measurement; download uses final client MeanClientMbps
- Background collector now parses the official Go client's final summary JSON
- Added explicit epoch/type/server CSV fields and shared result vocabulary
- Corrected CSV import status, deduplication and chronological sorting
- Corrected URL CSV quoting/timestamps and added confirmation-round field
- Added ping reentrancy guard, scheduler re-arm/watchdog, URL pause/stop handling
- Added separate 300 ms ping chart scale and dropout/failure distinction
- Added plist ThrottleInterval and absolute background schedule


### v2.0--v2.2b (September 2026)

-   Reliability redesign prompted by comparison with independent Ookla,
    Cloudflare and M-Lab tests
-   Confirmed that the previous 25 MB maximum browser transfer could
    substantially under-report a gigabit-class connection
-   Added explicit dropout classification and independent connectivity
    probes
-   Added M-Lab server/location diagnostic retention where available
-   Improved NDT7 callback/error handling, including VPN-path cases

### v1.8 (2026-04-19)

-   New CSV schema:
    `timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result`
    (stats-first column order; status+note merged into `result`;
    `os`+browser merged into `browser`)
-   Machine name defaults to `os-browser` (e.g. `mac-chrome`) if field
    left blank
-   `skins/` renamed to `themes/`; `skin-indigo.css` →
    `theme-indigo.css`
-   Full code constant cleanup: `VERSION`, `THEME`, `HOUR_MS`,
    `CHART_Y_MAX`, `CHART_WINDOW_ALL`, `LOG_MAX_ENTRIES`,
    `FULL_TEST_PASSES`, `PING_PROBE_COUNT`, `PING_QUICK_COUNT`,
    `DOWNLOAD_SIZES`, `UPLOAD_SIZES`, six chart color constants `COL_*`
-   Dead code removed: `getIfaceName()` (always blank), `netStr()`
    (duplicate of `compactNet()`), `PING_COLOR` (replaced by `COL_PING`)
-   Default intervals changed: ping 15 min, full test 30 min
-   Scheduler detail popup: shows `Platform` (e.g. `mac-chrome`),
    removed always-blank SSID row
-   Git repo initialized

### v1.7 (2026-04-11 to 2026-04-19)

-   Single full-width chart panel replaces side-by-side layout;
    draggable log/chart resizer
-   Ping overlay as optional second dataset on speed chart (right
    Y-axis, toggleable)
-   Time-window selector: 1h / 12h / 1d / 7d / 30d / All
-   1h window button added; window buttons moved to left of stats strip
-   Chart X-axis: true linear time axis, 6h tick grid, right edge = now
-   Dynamic tick steps by window (15 min → 3h → 12h → 3d depending on
    range)
-   Log columns reordered: DN / UP / PING
-   Diamond point markers at 1.5px radius
-   Ping-only rows styled distinctly; ping-only records feed into ping
    overlay dataset
-   Legend key strip below column headers
-   Platform display field in toolbar (OS + browser)
-   Network change detection (NETWORK-CHANGED state event)
-   Storage usage warning near localStorage capacity
-   Separate Ping Test button

### v1.6 (March--April 2026)

-   CSV schema: split timestamp into date + time columns; added `os`
    field
-   Import handles both old (`timestamp,machine,iface_type…`) and new
    schema
-   Retry mode on ping fail: 1-min retries ×5, OFFLINE / RECOVERED state
    events
-   Full test deferred until after successful ping
-   Row detail popup; help modal; localStorage persistence

### v1.x (early 2026)

-   Basic speed test via Cloudflare (ping, download, upload)
-   Single-interval scheduler
-   Line chart (last N readings), CSV export and drag-and-drop import

------------------------------------------------------------------------

## Design Re-evaluation --- September 2026

The original design treated browser throughput measurement as a
relatively simple timed transfer problem. That produced plausible
results at moderate speeds, but the design had no explicit accuracy
requirement against an established reference and no minimum
measurement-duration requirement.

Testing against independent services exposed the issue. Ookla,
Cloudflare, and M-Lab could all measure approximately gigabit-class
direct service while the monitor's short fixed-size transfers often
reported only a fraction of that rate. At 1 Gb/s, a 25 MB transfer takes
only about 0.2 seconds at line rate, making startup and path effects a
large fraction of the measurement.

The resulting design principle is:

> **Bandwidth measurements should be validated against established
> independent implementations across the expected operating range, and
> measurement failure must be distinguishable from network failure.**

The current architecture therefore separates the three questions instead
of asking one test to answer all of them:

-   **Can the Internet be reached?** --- independent witness sites.
-   **How responsive is the path?** --- lightweight HTTP latency probe.
-   **What sustained bandwidth is available?** --- M-Lab NDT7.

VPN operation further demonstrated why path metadata matters: a VPN can
cause different test services to select very different server locations.
A bandwidth result should therefore be interpreted together with its
route/server context when available.

------------------------------------------------------------------------

## Project Context

**Owner:** David Smith, senior EE, Ukiah CA

**Machines:** - Mac M4 Laptop --- primary dev machine - Mac Mini \~macOS
10.15 - Win 11 Dell (CW-DS-Dell-2025)

**Infrastructure:** Dropbox folder shared across all machines. Each
machine runs its own browser session; CSV logs named
`speedtest_<machine>_<year>-Q<quarter>.csv`.

**Companion app:** LiveCode stack `internet_status.livecode` v2.4 ---
runs on Windows, logs to the same CSV format. Logs from both tools can
be merged via Import.

**Current bandwidth source:** M-Lab NDT7. Cloudflare remains the
lightweight HTTP latency target. The three configured witness sites are
used only for connectivity classification, not bandwidth or comparative
website-performance testing.

**Validation references:** periodic manual comparison with established
independent services such as Ookla Speedtest, Cloudflare Speed Test, and
M-Lab is useful. Their results need not be identical because server
selection, routing, VPN path, protocol, duration, and aggregation
differ.

**Developed with:** Claude (claude.ai chat + Claude Code CLI/desktop
app)
## Background Collector

`speedmon_bg.py` is the unattended companion. It uses M-Lab's official Go
`ndt7-client` and parses the client's final JSON summary:

- download = `Download.Throughput.Value`
- upload = `Upload.Throughput.Value`
- latency = `Download.Latency.Value` (MinRTT, ms)
- server = `ServerFQDN`

Install the client:

`go install -v github.com/m-lab/ndt7-client-go/cmd/ndt7-client@latest`

Validate before enabling `launchd`:

`python3 speedmon_bg.py --once`

Install the LaunchAgent:

`cp speedmon_bg.plist ~/Library/LaunchAgents/com.ninetreesdesign.speedmon-bg.plist`

Load:

`launchctl load ~/Library/LaunchAgents/com.ninetreesdesign.speedmon-bg.plist`

Unload:

`launchctl unload ~/Library/LaunchAgents/com.ninetreesdesign.speedmon-bg.plist`

The collector log is `speedmon_bg.log`. The plist includes a 300-second
`ThrottleInterval`. The collector uses an absolute hourly schedule to avoid cumulative
drift.

Do not intentionally run browser and background NDT7 tests at the same time: overlapping
sustained tests compete for the same connection and can depress both measurements.





## v2.3 Required Files

- `speed_monitor.html`
- `ndt7-download-worker.js`
- `ndt7-upload-worker.js`
- `speedmon_bg.py`
- `speedmon_bg.plist`
- `README.md`
- `vendor/` (pinned client, chart library, licenses)

The two worker files must remain next to `speed_monitor.html`.

### Shared browser latency (2026-10-02)
Ping-only and full browser tests now use the same ten Cloudflare HTTP HEAD probes, before bandwidth testing. Displayed latency is the median of valid samples; zero, nonfinite, timed-out, and readings over 2000 ms are excluded. With an even number of valid samples, the middle pair is averaged. No valid samples means a failed check, not zero latency. Historical records remain unchanged; background collector latency remains M-Lab MinRTT.

### M-Lab rate limiting (2026-10-02)
The local NDT7 client has a small HTTP-error handling patch so discovery HTTP 429 is reported accurately. A 429 suspends browser full-test requests for 30 minutes; lightweight pings continue. Reloading resets this in-memory cooldown. This is a recovery delay, not a guaranteed provider quota or reset time. Use a longer full-test interval (30 minutes recommended) rather than continuous 2-minute bandwidth tests. Ping values display as whole milliseconds without changing stored precision.

### Failure clarity (2026-10-05)
Full bandwidth failures display TEST FAIL with their reason on hover/click; confirmed connectivity outages remain distinct. M-Lab rate-limit cooldown persists across page reloads and remains visible in the status bar.

### Discovery URL guard (2026-10-05)
Empty discovery results and missing secure WebSocket URLs fail before workers start, preventing requests to /undefined. The patched client script has a versioned URL to avoid stale browser caching. Provider rate-limit recovery has no confirmed reset time.
