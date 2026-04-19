# Speed Monitor — Claude Code Session Kickoff
# Paste this into a Claude Code session to continue development

## Project location
~/Dropbox/Internet Speed Test Log/
(folder was renamed from "Speed Test Log" on 2026-04-11 — old name in any legacy refs is stale)

## Key files
- speed_monitor.html          — main app (v1.7+), edit in place
- skins/skin-indigo.css       — color palette (CSS :root vars), default skin
- _start_monitor.command      — Mac launcher (double-click)
- _start_monitor.bat          — Windows launcher (double-click)
- speedtest_logs/             — CSV log files per machine per quarter
- speed_monitor_handoff.txt   — full developer handoff notes

## How to test changes
Server is likely already running at http://localhost:8765
If not: cd ~/Dropbox/Internet\ Speed\ Test\ Log && python3 -m http.server 8765
Then open: http://localhost:8765/speed_monitor.html
Hard-refresh after each edit: Cmd+Shift+R (Mac) or Ctrl+Shift+R (Win)

## Current state (v1.7+, as of 2026-04-19 session)

### Working / recently changed:
  - Dual schedule: ping check (default 5 min) + full speed test (default 15 min)
  - Full test fires only after successful ping
  - 3-pass averaged full test: ping, then DOWNLOAD, then UPLOAD (in that order)
  - Retry mode on ping fail: 1-min retries x5, log OFFLINE/RECOVERED
  - Full-width chart panel at bottom, default 210px height
  - Single combined chart: Download + Upload (Mb/s), optional Ping overlay (right Y axis)
  - Diamond points (rectRot), 1.5px radius, 3px hover (reduced from 2.5/5 this session)
  - Stats strip above chart: avg down/up/ping/uptime/tests/pings
  - Click any log row for detail popup
  - Help modal (? button in toolbar)
  - Export CSV (toolbar button)
  - Import CSV: toolbar Import button (file picker) OR drag-and-drop onto log area
  - CSV schema v1.6: date,time,machine,os,conn_type,iface_name,
                     status,ping_ms,download_mbps,upload_mbps,note

### Column display order (log + header): DN | UP | PING [ms]
### Test execution order: ping gate → download → upload (matches typical speed test tools)

### Skin system:
  - :root color vars extracted to skins/skin-indigo.css
  - const SKIN = 'indigo' at top of JS selects active skin
  - To add a skin: copy skins/skin-indigo.css → skins/skin-<name>.css, edit vars, set SKIN='<name>'

### Draggable chart/log resizer:
  - 5px #resizer bar between log and chart panel
  - Turns gold on hover/drag
  - Min 120px / max 600px chart height, default 210px
  - Position saved to localStorage key 'speedmon_chart_h'

### Responsive / max-width:
  - max-width: 880px, centered (margin: 0 auto)
  - html background fills sides beyond cap
  - min-width: 360px (phone-safe, no forced horizontal scroll)
  - @media(max-width:500px): smaller fonts + narrower columns for phones

### Local network phone access:
  - ipconfig getifaddr en0  →  http://<mac-ip>:8765/speed_monitor.html
  - python http.server binds 0.0.0.0 — all interfaces active simultaneously
  - For remote access: Tailscale (recommended) or ngrok

## Outstanding work items (priority order)

### 1. TOOLBAR LAYOUT — highest priority
Problem: toolbar has two rows; wraps on narrow windows.
Fix needed: restructure as single non-wrapping row using CSS grid or table-layout.
Layout target (left to right):
  [Machine name] [net display] [platform] | [Ping N min] [Full Test N min] | [Export] [Import] [?] || [Ping Test] [Full Test] [Pause] [Stop]
Right-side action buttons = fixed-width cell. Everything left stretches, never wraps.

### 2. CHART — date/time axis legibility
Problem: hard to see what day/time a data point is from on the chart.
Wanted:
  - Show date on X axis tick labels (not just time)
  - Tick marks at 3hr or 6hr intervals only (not every point)
  - Helps identify exactly when speed degradation events occurred

### 3. CHART — 500 points, pre-fill, live scroll
  - Default to 500 points displayed
  - Pre-fill with null slots so real data anchors to right edge
  - As new points arrive beyond 500, chart shifts left
  - Point selector: 100 / 200 / 500 / All

### 4. PING OVERLAY (optional / low priority)
  - Ping ms as secondary dataset on speed chart, right-hand Y axis (0-300ms)
  - Toggle on/off via button in chart title area
  - Already partially wired (hidden dataset [0], btnPingToggle, setPingVisible())

## CSS / design conventions
- Color palette in skins/skin-indigo.css — always use CSS vars, never hardcode colors
- Font: PT Mono / Consolas / Courier New monospace throughout
- Background layers: --bg (darkest) > --bg2 > --bg3
- Accent: --gold for primary actions, --ok green for good data, --fail red for errors
- All strings plain ASCII (no extended chars)
- Chart.js 4.4.0 from cdnjs — no other external dependencies

## CSS variable quick ref (skin-indigo)
  --bg/#1e1560  --bg2/#1a1450  --bg3/#150f40
  --bdr/#0a0828  --bdr2/#3a3280  --div/#3a2e90
  --gold/#c8a020  --gold-h/#e8d060  --btntxt/#100c00
  --txt/#c8c4e8  --dim/#6860a8  --lbl/#a09cd0
  --ok/#80d8a0  --fail/#e09090  --dot/#60e090  --stxt/#8880c8
  --ping-ok/#7878b8  --ping-fail/#e09090

## CSV schema reference
Header: date,time,machine,os,conn_type,iface_name,status,ping_ms,download_mbps,upload_mbps,note
Full test: 4/11/26,9:19 PM,Mac-M4-Laptop,mac,cable,,ok,54,224.10,22.30,speed-3pass
Ping row:  4/11/26,9:14 PM,Mac-M4-Laptop,mac,cable,,ok,51,,,,ping
State evt: # 4/11/26 9:14 PM STARTED machine=Mac-M4-Laptop ping=5 test=15 mac cable
Parser:    skip lines where char[0]==='#'
_t field:  'data'=full test, 'ping'=ping check, 'evt'=state event
Charts use only _t==='data' && status==='ok' rows

## Key functions
initCharts()        — creates Chart.js instance, sets point style/colors
updateCharts()      — maps logs to chart data, calls chart.update()
getChartRecords()   — filters and slices logs for chart display
runPingCheck()      — fast 3-probe ping, triggers full test if due
runFullTest()       — 3-pass averaged: ping → download → upload
addEntry(e)         — push to logs[], save to localStorage, prepend log row
addStateEvent(ev)   — log # comment line
startSchedules()    — starts both ping and test timers
cancelAllTimers()   — stops all timers
exportCSV()         — download CSV
importCSV(txt)      — parse and merge, handles old and new schema
applyChartHeight(h) — sets chart panel height, used by resizer

## Developer context
- David Smith, senior EE, Ukiah CA
- Mac M4 primary (main dev machine), Mac Mini ~10.15, Win 11 Dell
- Dropbox as shared file backbone across machines
- Companion LiveCode stack (internet_status.livecode v2.4) logs same CSV format
- Developed in Claude (claude.ai chat + Claude Code CLI/desktop app)
- Full handoff notes: speed_monitor_handoff.txt in same folder

## Active ISP investigation (as of 2026-04-19)
DN speed dropped from ~150-200 Mbps to ~50 Mbps a few days ago.
WiFi: NCC-1701 (2.4GHz) and NCC-1705 (5GHz) — both showing similar ~50 Mbps.
Similar speed on both bands suggests bottleneck is upstream of radio selection.
Modem was reset recently, no change. Router power cycle in progress (2026-04-19).
Next steps if router cycle doesn't help:
  1. Ethernet direct to modem — if fast, router is culprit
  2. If still slow on ethernet — call ISP, request provisioning profile re-push
CSV log has exact timestamp of the drop — needs chart date axis (work item #2) to see easily.
