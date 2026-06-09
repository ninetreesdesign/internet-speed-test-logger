#!/usr/bin/env python3
"""
speedmon_bg.py — background speed + URL monitor for Speed Monitor v1.9
Writes to the same CSV schema as the browser app so rows import cleanly.

Usage:
    python3 speedmon_bg.py            # runs continuously at INTERVAL_MINS
    python3 speedmon_bg.py --once     # single run then exit (for cron/launchd)

Auto-start at login (macOS):
    See speedmon_bg.plist in this folder — copy to ~/Library/LaunchAgents/
    then: launchctl load ~/Library/LaunchAgents/speedmon_bg.plist
"""

import urllib.request, urllib.error, ssl, time, csv, os, socket, sys, math
from datetime import datetime, timezone
from pathlib import Path

# ── config ────────────────────────────────────────────────────────────────────
MACHINE       = 'Mac M4 Laptop'       # must match your browser machine label
INTERVAL_MINS = 60                    # how often to run a full test
PASSES        = 3                     # ping/download/upload passes to average
TIMEOUT_SECS  = 30                    # per-request timeout
CF            = 'https://speed.cloudflare.com'

URL_TARGETS   = [                     # URLs to check each run
    'ninetrees.com',
    'cambrianworks.com',
    'apple.com',
]

LOG_DIR = Path(__file__).parent / 'speedtest_logs'

# ── helpers ───────────────────────────────────────────────────────────────────
def fmt_ts(dt=None):
    """Format timestamp matching browser app: M/D/YY H:MM AM/PM"""
    d = dt or datetime.now()
    return d.strftime('%-m/%-d/%y %-I:%M %p')

def quarter(dt=None):
    d = dt or datetime.now()
    return f"{d.year}-Q{math.ceil(d.month/3)}"

def speed_csv_path():
    LOG_DIR.mkdir(exist_ok=True)
    name = MACHINE.replace(' ', '_')
    return LOG_DIR / f"speedtest_{name}_{quarter()}.csv"

def url_csv_path():
    LOG_DIR.mkdir(exist_ok=True)
    name = MACHINE.replace(' ', '_')
    return LOG_DIR / f"urlcheck_{name}_{quarter()}.csv"

def ctx():
    """SSL context — verify certs"""
    return ssl.create_default_context()

def timed_get(url, body=None, method='GET'):
    """Return (elapsed_ms, bytes_received) or raise on error."""
    req = urllib.request.Request(url, data=body, method=method)
    req.add_header('User-Agent', 'speedmon-bg/1.9')
    t0 = time.perf_counter()
    with urllib.request.urlopen(req, timeout=TIMEOUT_SECS, context=ctx()) as r:
        data = r.read()
    elapsed = (time.perf_counter() - t0) * 1000
    return elapsed, len(data)

# ── speed test ────────────────────────────────────────────────────────────────
DOWNLOAD_SIZES = [500_000, 2_000_000, 5_000_000, 10_000_000, 25_000_000]
UPLOAD_SIZES   = [500_000, 2_000_000, 5_000_000]

def measure_ping():
    times = []
    for _ in range(5):
        try:
            ms, _ = timed_get(f'{CF}/__down?bytes=0', method='HEAD')
            times.append(ms)
        except Exception:
            pass
    if not times:
        raise RuntimeError('ping failed — no response')
    times.sort()
    return round(times[len(times)//2], 1)   # median

def measure_download():
    total_bytes = total_ms = 0
    for size in DOWNLOAD_SIZES:
        ms, n = timed_get(f'{CF}/__down?bytes={size}')
        total_ms += ms; total_bytes += n
    return round(total_bytes * 8 / (total_ms / 1000) / 1e6, 2)  # Mb/s

def measure_upload():
    total_bytes = total_ms = 0
    for size in UPLOAD_SIZES:
        body = bytes(size)
        ms, _ = timed_get(f'{CF}/__up', body=body, method='POST')
        total_ms += ms; total_bytes += size
    return round(total_bytes * 8 / (total_ms / 1000) / 1e6, 2)

def run_full_test():
    """3-pass averaged ping, download, upload. Returns (ping, down, up) or raises."""
    pings, downs, ups = [], [], []
    for p in range(PASSES):
        print(f'  pass {p+1}/{PASSES}: ping...', end=' ', flush=True)
        pings.append(measure_ping())
        print(f'{pings[-1]}ms  down...', end=' ', flush=True)
        downs.append(measure_download())
        print(f'{downs[-1]} Mb/s  up...', end=' ', flush=True)
        ups.append(measure_upload())
        print(f'{ups[-1]} Mb/s')
    avg = lambda lst: round(sum(lst)/len(lst), 2)
    return avg(pings), avg(downs), avg(ups)

# ── URL checks ────────────────────────────────────────────────────────────────
def check_url(raw):
    """Returns (state, ms) — state: 'up'|'timeout'|'down'"""
    url = raw if raw.startswith('http') else f'https://{raw}'
    t0 = time.perf_counter()
    try:
        req = urllib.request.Request(url, method='HEAD')
        req.add_header('User-Agent', 'speedmon-bg/1.9')
        with urllib.request.urlopen(req, timeout=10, context=ctx()) as r:
            r.read()
        return 'up', round((time.perf_counter()-t0)*1000)
    except socket.timeout:
        return 'timeout', round((time.perf_counter()-t0)*1000)
    except Exception:
        return 'down', round((time.perf_counter()-t0)*1000)

# ── CSV writers ───────────────────────────────────────────────────────────────
SPEED_HEADER = 'timestamp,download_mbps,upload_mbps,ping_ms,machine,browser,iface,result'
URL_HEADER   = 'timestamp,url,state,ms,conn_ok'

def append_speed_row(ts, ping='', down='', up='', result=''):
    path = speed_csv_path()
    write_header = not path.exists() or path.stat().st_size == 0
    with open(path, 'a', newline='') as f:
        if write_header:
            f.write(SPEED_HEADER + '\n')
        f.write(f'{ts},{down},{up},{ping},{MACHINE},mac-python,cable,{result}\n')

def append_url_rows(ts, results, conn_ok):
    path = url_csv_path()
    write_header = not path.exists() or path.stat().st_size == 0
    with open(path, 'a', newline='') as f:
        if write_header:
            f.write(URL_HEADER + '\n')
        for url, state, ms in results:
            f.write(f'{ts},{url},{state},{ms},{conn_ok}\n')

# ── main loop ─────────────────────────────────────────────────────────────────
def run_once():
    ts = fmt_ts()
    print(f'\n[{ts}] Starting full test...')
    conn_ok = True
    try:
        ping, down, up = run_full_test()
        print(f'  Result: ping={ping}ms  down={down} Mb/s  up={up} Mb/s')
        append_speed_row(ts, ping=ping, down=down, up=up)
    except Exception as e:
        print(f'  FAILED: {e}')
        append_speed_row(ts, result=f'fail: {str(e)[:40]}')
        conn_ok = False

    print(f'  Checking URLs...')
    url_results = []
    for raw in URL_TARGETS:
        state, ms = check_url(raw)
        label = 'remote?' if (conn_ok and state != 'up') else ''
        print(f'    {raw}: {state} {ms}ms {label}')
        url_results.append((raw, state, ms))
    append_url_rows(ts, url_results, conn_ok)

def main():
    once = '--once' in sys.argv
    print(f'speedmon_bg.py  machine={MACHINE}  interval={INTERVAL_MINS}min  passes={PASSES}')
    print(f'Speed log : {speed_csv_path()}')
    print(f'URL log   : {url_csv_path()}')
    if once:
        run_once()
    else:
        while True:
            run_once()
            print(f'  Sleeping {INTERVAL_MINS} min...')
            time.sleep(INTERVAL_MINS * 60)

if __name__ == '__main__':
    main()
