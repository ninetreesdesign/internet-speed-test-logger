# Internet Speed Monitor: Codex handoff

## Purpose and local workflow

Work in `~/Dropbox/Internet Speed Test Log`. This Dropbox-shared project records sustained Internet bandwidth, latency, and connectivity history across machines. The browser dashboard provides manual Full Test/Ping Test, scheduled monitoring, charts, localStorage persistence, and CSV export/import. Current defaults are 15-minute ping and 30-minute full tests. Preserve the existing local browser launch workflow and storage origin; inspect how it is actually launched before changing serving arrangements.

The optional macOS Python collector runs unattended through a LaunchAgent, with an hourly schedule and quarterly per-machine CSV files under `speedtest_logs`. Browser exports use `speedtest_<machine>_<year>-Q<n>.csv`. Historical browser, background, and LiveCode companion logs must remain mergeable.

Required files:

- `speed_monitor.html`: operational dashboard filename; keep it unchanged.
- `ndt7-download-worker.js` and `ndt7-upload-worker.js`: local workers beside the HTML.
- `speedmon_bg.py`: background collector using the official Go NDT7 client.
- `speedmon_bg.plist`: macOS LaunchAgent configuration.
- `README.md`: workflow, dependencies, and version history.

## Reference performance and architecture

Established direct-connection references are approximately **950–1030 Mbps download, 35 Mbps upload, and 10–25 ms latency**. These are comparison ranges, not hard acceptance thresholds. VPN examples are routing/server-selection context only; do not use them as the direct-service baseline. HTTP latency and TCP MinRTT are different measurements.

Use M-Lab NDT7 for sustained throughput: the browser client plus **exact upstream M-Lab workers**, with explicit worker paths and compatible client/worker versions. Verify provenance and the actual callback contract; do not reconstruct workers. One completed NDT7 run is one full test. For **both download and upload**, use the final client `MeanClientMbps`, already in Mbps, not a peak/progress sample or an ad hoc raw-rate conversion. Retain selected server metadata. Cloudflare remains the lightweight HTTP latency target.

The background collector uses official `ndt7-client` final-summary JSON: `Download.Throughput.Value`, `Upload.Throughput.Value`, `Download.Latency.Value`, and `ServerFQDN`. Inspect actual client output and units before trusting the parser; progress/event output is not the final summary.

## Connectivity and graph meaning

Independent witnesses are `ninetrees.com`, `cambrianworks.com`, and `apple.com`. Any reachable witness establishes connectivity. If all three fail, wait **10 seconds**, then repeat. Only all three failing in both rounds is a **confirmed dropout**. Retain per-site results and confirmation-round diagnostics separately from bandwidth history.

- **Valid measurement:** plot measured throughput/latency and include eligible values in averages.
- **Confirmed dropout:** explicit outage marker/zero-throughput representation, distinguishable from a measured zero; preserve dropout classification through CSV roundtrips.
- **Tool/measurement failure:** visible failure or graph gap, never invented zero bandwidth or an inferred outage. A failed NDT7 run with reachable witnesses is a tool failure.

Availability must use connectivity evidence and confirmed dropouts, not speed-test success rate. Exclude unknown connectivity and state events from its denominator; describe it as observation-based unless elapsed outage duration is actually calculated. Exclude missing ping from averages rather than coercing it to zero. Keep ping on its own millisecond axis (current intended scale: 300 ms), independent of the Mbps axis.

## Data contract and compatibility

Preserve historical data, import formats, machine identity, server diagnostics, and these existing localStorage keys:
`speedmon_logs_v2`, `speedmon_url_logs_v1`, `speedmon_prefs_v2`, `speedmon_chart_h`.

Current v2.3 speed CSV schema, in order:

```csv
timestamp,epoch,type,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result,server
```

`epoch` is Unix seconds (fractional seconds supported; importer also accepts milliseconds); retain human-readable `timestamp`. Record types must distinguish full tests, ping-only records, and events. Inspect and reconcile the existing `evt` versus `event` mapping without losing historical events.

Measurement `result` vocabulary:

- Empty: successful measurement.
- `dropout: confirmed`: confirmed connectivity outage.
- `fail: <reason>`: measurement/tool failure.
- Event rows: event text, interpreted by record type rather than mistaken for failed measurements.

Keep support for v1.8's eight-column schema below, earlier date/time and legacy schemas, and existing LiveCode logs:

```csv
timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result
```

Intended shared URL diagnostics schema:

```csv
timestamp,url,state,ms,conn_ok,round
```

`state`: `up`, `timeout`, or `down`; `conn_ok`: normalized `true`/`false`, blank if unknown; `round`: `initial` or `confirm`. Accept historical five-column URL logs without fabricating a round. Parse booleans explicitly: the string `"false"` is not truthy connectivity. Quote commas, quotes, and newlines correctly. Preserve timestamps, sort chronologically by normalized epoch, and deduplicate repeated imports without collapsing distinct machines, record types, or meaningful observations.

## Known failure and unverified correction

v2.3 produced **996531233 / 8334302 Mbps**, polluted 24-hour averages, and repeatedly logged FAIL/no reliable completion. Prior workers were admitted to be reconstructed rather than exact upstream copies. Raw-rate/unit confusion is suspected; inspect the actual cause.

v2.3a claims to use final client `MeanClientMbps` in both directions, reject implausible measurements before storage, and quarantine corrupt legacy rows. Treat those as intended corrections, not validated behavior. Apply finite, nonnegative plausibility gates: **download <=2000 Mbps, upload <=500 Mbps, ping <=2000 ms**. Missing values remain missing; an incomplete full test must not become a valid zero-speed result. Enforce gates on acquisition, load, and import. Preserve invalid legacy records as diagnosable failures/quarantine, excluded from charts and averages; do not silently erase history.

A read-only handoff inspection found local HTML labeled `2.3a`, README still describing `2.3` and server-side upload, and browser URL export still five columns while Python declares six including `round`. These are inspection leads, not a completed review. No runtime, syntax, or live measurement tests were run while preparing this handoff. Prior conversation claims of passing checks do not establish the current files' correctness.

## Claude review priorities / inspection checklist

1. **Measurement correctness:** exact upstream workers, load paths, callback/event contract, units, final-not-peak selection, both-direction `MeanClientMbps`, missing/invalid-value handling, legacy quarantine, background final-summary parser.
2. **History integrity:** dropout CSV roundtrip, `evt`/`event` mapping, dedupe, timestamp sorting, URL CSV quoting, `conn_ok` normalization, URL round field, and old-schema append/import compatibility.
3. **Lifecycle:** scheduler/reentrancy, manual-test rearm, absolute scheduling without drift, pause/stop behavior including URL timers and in-flight completion, and `runPingCheck` cleanup through `try/finally` so exceptions cannot strand its busy flag.
4. **Classification/statistics:** distinguish timeout from other failure and from confirmed dropout; availability denominator, missing ping exclusion, separate ping scale, and graph semantics.
5. **Background operation:** browser/background NDT7 overlap can depress both results; inspect coordination and scheduling. Check background log rotation, actual LaunchAgent paths, `ThrottleInterval=300`, subprocess timeouts, and README synchronization.

The event mapping, URL round schema, background log rotation, browser/background overlap, scheduler/manual rearm, and `runPingCheck` exception cleanup remain explicit caveats requiring inspection. Do not assume an earlier version-history bullet proves implementation.

## Codex task

**Review first.** Inspect all six actual project files and relevant local instructions/history before changing anything. Briefly report concrete P1/P2 findings with file locations and impact, distinguishing confirmed defects from open questions. Then implement focused fixes directly in `~/Dropbox/Internet Speed Test Log`; avoid broad rewrites, renamed operational HTML, changed storage keys, or unnecessary data migrations.

Run JavaScript syntax checks on extracted HTML scripts and both workers, Python compilation, and plist validation. Exercise CSV import/export with valid full/ping/event/dropout/failure rows, historical formats, repeated imports, out-of-order timestamps, quoted URLs, boolean strings, missing ping, and bogus/nonfinite/out-of-range values. Verify quarantine, statistics, scheduler/reentrancy, and pause/stop behavior with targeted tests.

Validate **one live NDT7 run** against the plausible direct reference range, recording download/upload/latency, server, connection/VPN context, and any limitation; avoid simultaneous background bandwidth testing. Passing syntax alone is insufficient. Update README and version only after validation, accurately distinguishing verified behavior from unresolved issues. Leave a concise change/test summary. **Never claim a check passed unless you actually ran it; explicitly report blocked or unrun checks.**
