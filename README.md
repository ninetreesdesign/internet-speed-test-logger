# Speed Monitor

A single-file browser dashboard that measures and logs internet speeds on a schedule.
Runs on any machine with Python and a modern browser. Logs to localStorage and exports
quarterly CSV files. Built to monitor ISP consistency across multiple machines sharing
a Dropbox folder.

---

## Features

- **Dual scheduler** — ping check every N min + full speed test every M min (defaults 15/30)
- Full test only fires after a successful ping (no wasted tests on a dead connection)
- **3-pass averaged full test** — ping, download, upload each measured 3× and averaged
- **Retry mode** — ping fail triggers 1-min retries ×5, logs OFFLINE / RECOVERED events
- **Log panel** — scrollable, newest first; click any row for a detail popup
- **Chart panel** — full-width speed chart (DN + UP Mb/s); draggable split with log panel
- **Ping overlay** — optional ping ms trace on second Y-axis; toggle with Show/Hide Ping
- **Time-window selector** — 1h / 12h / 1d / 7d / 30d / All
- **Stats strip** — avg download, upload, ping, uptime %, test count, ping count
- **Network change detection** — logs NETWORK-CHANGED event if client IP shifts
- Export CSV / Import CSV / drag-and-drop import onto log area
- Help modal (? button), storage usage warning near localStorage limit

---

## Quick Start

The Cloudflare speed test API requires an HTTP origin — `file://` URLs will not work.
A local Python server takes care of this.

### Mac / Linux

```bash
cd ~/Dropbox/Internet\ Speed\ Test\ Log
python3 -m http.server 8765
```

Then open: `http://localhost:8765/speed_monitor.html`

To find the IP for access from another device on the same network:

```bash
ipconfig getifaddr en0        # Mac
hostname -I | awk '{print $1}' # Linux
```

Then open `http://<ip>:8765/speed_monitor.html` on any device. Python's http.server
binds to all interfaces by default.

### Windows

```
cd "C:\Users\<you>\Dropbox\Internet Speed Test Log"
python -m http.server 8765
```

*(Windows often uses `python` rather than `python3`.)*

Hard-refresh after any code edit: `Cmd+Shift+R` (Mac) / `Ctrl+Shift+R` (Win/Linux)

### First use on a new machine

1. Start the server and open the URL above
2. Enter a machine name in the top-left field — labels every log row from this device
3. Set Ping interval and Full Test interval (click field, type, press Enter)
4. Dashboard auto-starts — ping fires immediately, full test follows at its interval

Data saves in browser localStorage. Export CSV quarterly to keep a permanent copy.

---

## Controls

| Control | Action |
|---------|--------|
| Machine name | Label for this device in all log rows; defaults to os-browser if blank |
| Ping N min | Ping-check interval; Enter or click away to apply |
| Full N min | Full test interval |
| Ping | Manual ping check now |
| Full Test | Manual full speed test now |
| Pause / Resume | Suspend or resume all scheduling |
| Stop | Halt without closing the page |
| Export | Download CSV log |
| Import | Load a CSV to merge history |
| Log area drop zone | Drag a CSV onto the log to import |

---

## Log Row Symbols

| Symbol | Meaning |
|--------|---------|
| `●` green | Full speed test — ok |
| `✕` red | Speed test or ping — failed |
| `·` dim blue | Ping check — ok (no DN/UP speeds) |
| `#` gray italic | State event (STARTED, PAUSED, OFFLINE, RECOVERED…) |

---

## CSV Format

```
timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result
```

| Field | Values / notes |
|-------|---------------|
| `timestamp` | `M/D/YY H:MM AM/PM` |
| `download_mbps` | blank for ping-only rows |
| `upload_mbps` | blank for ping-only rows |
| `ping_ms` | blank on failure |
| `machine` | user-set label, or `os-browser` if blank |
| `browser` | `mac-chrome`, `win-edge`, `linux-firefox`, etc. |
| `iface` | `wifi` / `eth` / `cable` / `cell` |
| `result` | blank = ok; failure note if not |

**Examples:**
```
4/19/26 9:14 AM,224.10,22.30,54,Mac-M4-Laptop,mac-chrome,cable,
4/19/26 9:29 AM,,,48,Mac-M4-Laptop,mac-chrome,cable,
4/19/26 9:30 AM,,,,Mac-M4-Laptop,mac-chrome,wifi,ping-fail abort
# 4/19/26 9:00 AM STARTED machine=Mac-M4-Laptop ping=15 test=30 mac cable
```

**Parser rule:** skip any line where the first character is `#`

**Log file naming:** `speedtest_<machine>_<year>-Q<quarter>.csv` — one per machine per quarter

The importer handles three historical schema versions automatically (v1.x timestamp-based,
v1.6 split date/time, current).

---

## Developer Guide

### Project layout

```
speed_monitor.html      main app — edit in place, self-contained
themes/theme-indigo.css color palette as CSS :root vars
speedtest_logs/         CSV exports, one file per machine per quarter
README.md               this file
CLAUDE_CODE_KICKOFF.md  deprecated — content merged here
```

### How to test changes

Server is likely already running at `http://localhost:8765`. If not, start it (see Quick Start).
Hard-refresh after each edit.

### Theme system

All colors are CSS custom properties defined in `themes/theme-indigo.css`.
The active theme is selected by `const THEME = 'indigo'` at the top of the JS block.
To add a theme: copy `themes/theme-indigo.css` → `themes/theme-<name>.css`, edit the
`:root` vars, set `THEME = '<name>'`.

**CSS variable reference (indigo theme):**

```
--bg        #1e1560   page background (darkest)
--bg2       #1a1450   input / log background
--bg3       #150f40   toolbar / chart background
--bdr       #0a0828   subtle border
--bdr2      #3a3280   active border / button hover
--div       #3a2e90   divider lines
--gold      #c8a020   primary action color
--gold-h    #e8d060   gold hover
--btntxt    #100c00   text on gold buttons
--txt       #c8c4e8   primary text
--dim       #6860a8   secondary / label text
--lbl       #a09cd0   column headers / values
--ok        #80d8a0   good speed data (green)
--fail      #e09090   failure (red)
--dot       #60e090   status dot idle
--stxt      #8880c8   status bar text
--ping-ok   #7878b8   ping-only log rows
--ping-fail #e09090   ping fail rows
```

### Design conventions

- Always use CSS vars — never hardcode colors in CSS or JS
  *(JS chart colors are constants `COL_*` at the top of the script, defined to match theme vars)*
- Font: PT Mono / Consolas / Courier New monospace throughout
- Background layers: `--bg` (darkest) → `--bg2` → `--bg3`
- All user-visible strings: plain ASCII only
- Chart.js 4.4.0 from cdnjs — no other external dependencies
- Numeric separators (`1_000_000`) used in constants — ES2021, requires Chrome 75+,
  Firefox 70+, Safari 13+, Edge 75+

### Key functions

| Function | What it does |
|----------|-------------|
| `init()` | Load prefs, restore logs, start charts and scheduler |
| `runPingCheck()` | Quick 3-probe ping; triggers full test if one is due |
| `runFullTest()` | 3-pass averaged ping → download → upload via Cloudflare |
| `addEntry(e)` | Push entry to `logs[]`, save to localStorage, prepend log row |
| `addStateEvent(ev)` | Append a `# timestamp EVENT` row |
| `startSchedules()` | Arm both ping and test timers |
| `clearAllTimers()` | Cancel all timers (pause / stop) |
| `initCharts()` | Create Chart.js instance with all dataset and axis config |
| `updateCharts()` | Map current log window to chart data and call `chart.update()` |
| `getChartRecords()` | Filter `_t==='data'` entries to the active time window |
| `getPingOnlyRecords()` | Filter `_t==='ping'` entries to the active time window |
| `exportCSV()` | Build and download a CSV file |
| `importCSV(txt)` | Parse CSV, detect schema version, merge into logs[] |
| `applyChartHeight(h)` | Set chart panel height; called by resizer drag handler |

### Scheduler design

Two independent timers run in parallel:

- **Ping timer** — fires every `getPingInterval()` minutes; runs `runPingCheck()`
- **Test due time** (`testDueAt`) — absolute epoch set by `scheduleTest()`; checked each
  time ping succeeds. If the test is due (or within one ping interval), ping skips its
  standalone log entry and runs the full test instead.

On ping failure: retry mode fires every `RETRY_INTERVAL_MINS` up to `MAX_RETRIES` times,
then reverts to normal ping schedule. Logs OFFLINE on first fail, RECOVERED on success.

### Browser compatibility notes

- **Network Information API** (`navigator.connection`) — not supported in Firefox or Safari.
  Code falls back to `'cable'` as the connection type on those browsers.
- **`navigator.platform`** — deprecated spec, still works universally.
- **OS version detection** — macOS version is frozen at 10.15.7 in Chrome/Edge (fingerprint
  protection). Not stored; machine label captures device identity instead.
- **`file://` URLs** — will not work. Cloudflare's speed test API requires a proper HTTP origin.

---

## Outstanding Work

| # | Item | Notes |
|---|------|-------|
| 1 | **Toolbar — single non-wrapping row** | Currently 2-row; restructure with CSS grid |
| 2 | **Chart 500pt point selector** | Count-based (100/200/500/All) with null pre-fill so data anchors right edge |
| 3 | **Import schema helpers** | Split `importCSV()` format detection into `parseRowV1x()`, `parseRowV16()`, `parseRowCurrent()` |

---

## Version History

### v1.8 (2026-04-19)
- New CSV schema: `timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result`
  (stats-first column order; status+note merged into `result`; `os`+browser merged into `browser`)
- Machine name defaults to `os-browser` (e.g. `mac-chrome`) if field left blank
- `skins/` renamed to `themes/`; `skin-indigo.css` → `theme-indigo.css`
- Full code constant cleanup: `VERSION`, `THEME`, `HOUR_MS`, `CHART_Y_MAX`, `CHART_WINDOW_ALL`,
  `LOG_MAX_ENTRIES`, `FULL_TEST_PASSES`, `PING_PROBE_COUNT`, `PING_QUICK_COUNT`,
  `DOWNLOAD_SIZES`, `UPLOAD_SIZES`, six chart color constants `COL_*`
- Dead code removed: `getIfaceName()` (always blank), `netStr()` (duplicate of `compactNet()`),
  `PING_COLOR` (replaced by `COL_PING`)
- Default intervals changed: ping 15 min, full test 30 min
- Scheduler detail popup: shows `Platform` (e.g. `mac-chrome`), removed always-blank SSID row
- Git repo initialized

### v1.7 (2026-04-11 to 2026-04-19)
- Single full-width chart panel replaces side-by-side layout; draggable log/chart resizer
- Ping overlay as optional second dataset on speed chart (right Y-axis, toggleable)
- Time-window selector: 1h / 12h / 1d / 7d / 30d / All
- 1h window button added; window buttons moved to left of stats strip
- Chart X-axis: true linear time axis, 6h tick grid, right edge = now
- Dynamic tick steps by window (15 min → 3h → 12h → 3d depending on range)
- Log columns reordered: DN / UP / PING
- Diamond point markers at 1.5px radius
- Ping-only rows styled distinctly; ping-only records feed into ping overlay dataset
- Legend key strip below column headers
- Platform display field in toolbar (OS + browser)
- Network change detection (NETWORK-CHANGED state event)
- Storage usage warning near localStorage capacity
- Separate Ping Test button

### v1.6 (March–April 2026)
- CSV schema: split timestamp into date + time columns; added `os` field
- Import handles both old (`timestamp,machine,iface_type…`) and new schema
- Retry mode on ping fail: 1-min retries ×5, OFFLINE / RECOVERED state events
- Full test deferred until after successful ping
- Row detail popup; help modal; localStorage persistence

### v1.x (early 2026)
- Basic speed test via Cloudflare (ping, download, upload)
- Single-interval scheduler
- Line chart (last N readings), CSV export and drag-and-drop import

---

## Project Context

**Owner:** David Smith, senior EE, Ukiah CA

**Machines:**
- Mac M4 Laptop — primary dev machine
- Mac Mini ~macOS 10.15
- Win 11 Dell (CW-DS-Dell-2025)

**Infrastructure:** Dropbox folder shared across all machines. Each machine runs its own
browser session; CSV logs named `speedtest_<machine>_<year>-Q<quarter>.csv`.

**Companion app:** LiveCode stack `internet_status.livecode` v2.4 — runs on Windows,
logs to the same CSV format. Logs from both tools can be merged via Import.

**Speed test source:** Cloudflare `speed.cloudflare.com` — always routes to nearest PoP,
no auth required, consistent target for relative comparisons.

**Developed with:** Claude (claude.ai chat + Claude Code CLI/desktop app)
