# Internet Speed Monitor

## Current design (v3, 2026-11-04)

Two parts, no server and no open tab:

- **Collector** (`collector/`): runs invisibly from login on each computer. It checks
  connectivity every minute and runs a speed test about every 4 hours, then appends to
  `data/<computer>/`, which Dropbox syncs.
- **Viewer** (`speed_monitor.html`): double-click to see every computer's logs and graphs. Keep
  it with `vendor/` and `viewer_data/`. It reloads its data every 5 minutes. Its Ping and Full
  Test buttons run immediately in that browser and are stored only there; it runs no scheduled
  tests. Export and Import cover those manual tests only.

Tested in Chrome and Firefox on Mac. Safari, Windows and Linux are not yet tested. Previews inside
other apps cannot run the page; use a real browser window.

### Install on a computer

Run once from `collector/bin/`, using the file for that system:

    ./speedmon-collector-mac-arm64 install --machine "Laptop"
    speedmon-collector-win.exe install --machine "Dell"
    ./speedmon-collector-linux-amd64 install --machine "Linux reference"

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
within one tool only. Different test tools can produce different results on the same connection.

### Planned, when the Linux box is ready

Objective, method and steps are in [docs/linux_box_plan.md](docs/linux_box_plan.md): the collector on
the box's host OS as the wired reference, per-computer speed intervals, and a self-expiring watch mode.

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

## Development and verification

Build platform binaries with `collector/build_all.sh` (requires Go). Run browser regressions
with `node --test tests/monitor.test.cjs` and collector tests with `cd collector && go test ./...`.
Vendored dependencies and licenses are documented in `vendor/README.md`. Regenerate embedded
workers after editing the upstream worker copies with `python3 vendor/build_embedded_workers.py`.

Copy `annotations.example.csv` to `annotations.csv` and add your equipment and service changes locally.
Logs, generated viewer files, binaries and personal annotations are excluded from commits.

## Design notes

- CSVs are the durable record; SQLite and viewer files can be rebuilt from them.
- Distinguish a failed measurement from a connectivity outage. Missing measurements stay blank.
- Keep tool identity with every speed result and compare trends within one method.
- Keep the page's dormant scheduler for compatibility; background collection is the default.
- Avoid overlapping manual and scheduled bandwidth tests on the same connection.
- A later desktop viewer can consume the same data. Linux setup and future watch mode are
  described in [the Linux plan](docs/linux_box_plan.md).
