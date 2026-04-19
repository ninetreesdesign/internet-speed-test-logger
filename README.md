# Internet Speed Test Log

## What's in this folder

| File | Description |
|------|-------------|
| `speed_monitor.html` | HTML dashboard (v1.7) — the monitoring app |
| `skins/` | CSS skin files loaded at runtime |
| `speedtest_logs/` | CSV log files, one per machine per quarter |
| `speedtest_Mac M4 Laptop_<date>.csv` | Exported log snapshots |
| `CLAUDE_CODE_KICKOFF.md` | Session kickoff doc for Claude Code development |

---

## Quick start

Requires a local HTTP server — the Cloudflare speed test API does not work from `file://` URLs.

**Mac — Terminal:**
```bash
cd ~/Dropbox/Internet\ Speed\ Test\ Log
python3 -m http.server 8765
```

**Windows — CMD:**
```
cd "C:\Users\Owner\Dropbox\Internet Speed Test Log"
python -m http.server 8765
```

Then open: `http://localhost:8765/speed_monitor.html`

Hard-refresh after any edit: `Cmd+Shift+R` (Mac) / `Ctrl+Shift+R` (Win)

---

## First use on a new machine

1. Start the server (commands above)
2. Open `http://localhost:8765/speed_monitor.html`
3. Enter a machine name in the top-left field
4. Set Ping interval (default 5 min) and Full test interval (default 15 min)
5. Dashboard auto-starts — ping fires immediately, full test follows on schedule

Data saves in browser localStorage. Export CSV periodically for a permanent copy.

---

## speed_monitor.html (v1.7)

### Features

- **Dual scheduler:** ping check every N min + full speed test every M min (defaults 5/15)
- Full test only fires after a successful ping
- **3-pass averaged full test** — ping, download, upload each measured 3× and averaged
- **Retry mode:** ping fail triggers 1-min retries ×5, then reverts to normal; logs OFFLINE / RECOVERED events
- **Log panel** (top) — scrollable, newest entry first; click any row for a detail popup
- **Chart panel** (bottom) — single full-width speed chart (download + upload Mb/s); drag resizer to adjust split
- **Ping overlay** — optional ping ms trace on second Y-axis; toggle with Show/Hide Ping button
- **Time-window selector:** 12h / 1d / 7d / 30d / All
- **Stats strip** above chart: avg download, upload, ping, uptime %, test count, ping count
- **Network change detection** — logs NETWORK-CHANGED event if client IP changes
- Export CSV / Import CSV / drag-and-drop import onto log area
- Help modal (? button)
- Storage usage warning when localStorage approaches 80% full

### Log row symbols

| Symbol | Meaning |
|--------|---------|
| `●` green | Full speed test — ok |
| `✕` red | Full speed test or ping — failed |
| `·` dim blue | Ping check — ok (no speeds) |
| `#` gray italic | State event (STARTED, PAUSED, OFFLINE, RECOVERED…) |

### Controls

| Control | Action |
|---------|--------|
| Machine name field | Label for this device in all log rows |
| Ping N min | Ping check interval; press Enter or click away to apply |
| Full Test N min | Full test interval; same |
| Ping Test | Run a manual ping check now |
| Full Test | Run a manual full speed test now |
| Pause / Resume | Suspend or resume all scheduling |
| Stop | Halt without closing the page |
| Export | Download CSV log file |
| Import | Load an existing CSV to merge history |
| Drop zone | Drag a CSV onto the log area to import |

---

## CSV log format

```
date,time,machine,os,conn_type,iface_name,status,ping_ms,download_mbps,upload_mbps,note
```

**Examples:**
```
4/11/26,9:19 PM,Mac-M4-Laptop,mac,cable,,ok,54,224.10,22.30,speed-3pass
4/11/26,9:14 PM,Mac-M4-Laptop,mac,cable,,ok,51,,,,ping
4/19/26,8:00 AM,CW-DS-Dell,win,wifi,RedDwarf,ok,12,188.20,18.40,speed-3pass
# 4/11/26 9:14 PM STARTED machine=Mac-M4-Laptop ping=5 test=15 mac cable
```

**Field values:**
- `note`: ping / speed-3pass / ping-fail / speed-fail (max 28 chars)
- `conn_type`: wifi / eth / cable / cell
- `os`: mac / win / linux / ios / android

**Parser rule:** skip any line where first character is `#`

**Log file naming:** `speedtest_<machine>_<year>-Q<quarter>.csv` — one per machine per quarter

---

## Companion app

A LiveCode desktop stack (`internet_status.livecode` v2.4) runs on Windows and logs to the
same CSV format. Logs from both tools can be merged via Import.

---

## Outstanding work items

| # | Item | Notes |
|---|------|-------|
| 1 | **Toolbar — single non-wrapping row** | Currently 2-row layout; restructure with CSS grid/table-layout |
| 2 | **Chart X-axis date legibility** | Show date on tick labels; 3h or 6h intervals only |
| 3 | **Chart 500pt / pre-fill / live scroll** | Null pre-fill so data anchors right edge; point selector 100/200/500/All |
| 4 | **Ping overlay** | Already partially wired (`btnPingToggle`, `setPingVisible()`); low priority |

---

## Changes log

### v1.7 (current — April 2026)
- Single full-width chart panel replaces side-by-side dual chart layout
- Draggable resizer between log panel and chart panel (height persisted in localStorage)
- Ping overlay as optional second dataset on speed chart (right Y-axis, toggleable)
- Time-window selector (12h / 1d / 7d / 30d / All) replaces point-count selector
- Chart X-axis: true linear time axis with 6-hour tick grid, right edge = now
- Log columns reordered: DN / UP / PING (was PING / DN / UP)
- Diamond point markers reduced to 1.5px radius
- Ping-only rows styled distinctly: dim opacity, smaller font, blue dot icon
- Legend key strip added below column headers
- Platform display field added to toolbar (OS + browser)
- Network change detection (NETWORK-CHANGED state event)
- Storage usage warning when localStorage nears capacity
- Separate Ping Test button (manual ping without full test)

### v1.6 (March–April 2026)
- CSV schema updated: split timestamp into separate date + time columns
- Added os field to CSV schema
- Import handles both old (timestamp,machine,iface_type…) and new schema
- Retry mode on ping fail: 1-min retries ×5, OFFLINE / RECOVERED state events
- Full test deferred until after successful ping (no wasted test on dead connection)
- Detail popup on log row click
- Help modal (? button)
- localStorage persistence with export/import CSV

### v1.x (early 2026)
- Basic speed test via Cloudflare speed.cloudflare.com (ping, download, upload)
- Single-interval scheduler
- Line chart (last N readings) — side-by-side download/upload and ping panels
- CSV export, drag-and-drop import
- Machine name field, localStorage log storage

---

## Development notes

| | |
|---|---|
| **Developed with** | Claude (claude.ai chat + Claude Code) |
| **Speed test source** | Cloudflare speed.cloudflare.com |
| **Chart library** | Chart.js 4.4.0 (cdnjs) |
| **Skin** | `skins/skin-indigo.css` |
| **Last updated** | 2026-04-19 |
