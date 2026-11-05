#!/usr/bin/env python3
"""
speedmon_bg.py — background companion for Internet Speed Monitor v2.2c+
Version 2.3b-bg1  2026-09-26

Design:
  - M-Lab NDT7 (official ndt7-client CLI): download/upload bandwidth
  - Official NDT7 final-summary MinRTT: latency
  - Three independent witness sites: connectivity/dropout classification
  - CSV schema remains compatible with the browser monitor

Requirements:
  Python 3
  M-Lab ndt7-client in PATH or ~/go/bin/ndt7-client

Install ndt7-client:
  go install -v github.com/m-lab/ndt7-client-go/cmd/ndt7-client@latest

Usage:
  python3 speedmon_bg.py
  python3 speedmon_bg.py --once
"""

import csv
import json
import math
import os
import shutil
import socket
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request
import platform
import logging
from logging.handlers import RotatingFileHandler
from datetime import datetime
from pathlib import Path

VERSION = '2.3b-bg1'

# ── config ────────────────────────────────────────────────────────────────────
MACHINE = 'Mac M4 Laptop'          # must match browser machine label
INTERVAL_MINS = 60                 # background full-test interval
TIMEOUT_SECS = 10                  # lightweight request timeout
NDT_TIMEOUT_SECS = 70              # whole ndt7-client subprocess limit
DROPOUT_RECHECK_SECS = 10

URL_TARGETS = [
    'ninetrees.com',
    'cambrianworks.com',
    'apple.com',
]

LOG_DIR = Path(__file__).parent / 'speedtest_logs'

# v2.3 explicit cross-tool CSV contract.
URL_HEADER = ['timestamp', 'url', 'state', 'ms', 'conn_ok', 'round']
SPEED_HEADER = ['timestamp','epoch','type','download_mbps','upload_mbps','ping_ms',
                'machine','browser','iface','result','server']


# ── helpers ───────────────────────────────────────────────────────────────────
def fmt_ts(dt=None):
    """Format timestamp matching browser app: M/D/YY H:MM AM/PM."""
    d = dt or datetime.now()
    return f'{d.month}/{d.day}/{d.year % 100:02d} {d.hour % 12 or 12}:{d.minute:02d} {"AM" if d.hour < 12 else "PM"}'


def quarter(dt=None):
    d = dt or datetime.now()
    return f'{d.year}-Q{math.ceil(d.month / 3)}'


def speed_csv_path():
    LOG_DIR.mkdir(exist_ok=True)
    name = ''.join(c for c in MACHINE.replace(' ', '_') if c.isascii() and (c.isalnum() or c in '_.-')) or 'machine'
    return LOG_DIR / f'speedtest_{name}_{quarter()}.csv'


def url_csv_path():
    LOG_DIR.mkdir(exist_ok=True)
    name = ''.join(c for c in MACHINE.replace(' ', '_') if c.isascii() and (c.isalnum() or c in '_.-')) or 'machine'
    return LOG_DIR / f'urlcheck_{name}_{quarter()}.csv'


def ssl_ctx():
    return ssl.create_default_context()


def normalize_url(raw):
    return raw if raw.startswith(('http://', 'https://')) else f'https://{raw}'


def ndt7_path():
    """Find the official M-Lab ndt7-client executable."""
    candidates = [
        shutil.which('ndt7-client'),
        str(Path.home() / 'go' / 'bin' / ('ndt7-client.exe' if os.name == 'nt' else 'ndt7-client')),
    ]
    for candidate in candidates:
        if candidate and Path(candidate).is_file() and os.access(candidate, os.X_OK):
            return candidate
    raise RuntimeError(
        'ndt7-client not found; install with: '
        'go install -v github.com/m-lab/ndt7-client-go/cmd/ndt7-client@latest'
    )


# ── M-Lab NDT7 bandwidth ──────────────────────────────────────────────────────
def run_ndt7():
    """Run official M-Lab ndt7-client once and parse its final summary JSON."""
    cmd = [
        ndt7_path(),
        '-format', 'json',
        '-quiet',
        '-client-name', 'speedmon-bg',
        '-timeout', '60s',
    ]
    proc = subprocess.run(
        cmd, capture_output=True, text=True,
        timeout=NDT_TIMEOUT_SECS, check=False,
    )

    if proc.returncode != 0:
        raise RuntimeError((proc.stderr.strip() or 'NDT7 client failed')[:240])
    return parse_ndt7_summary(proc.stdout)


def parse_ndt7_summary(output):
    """Only accept the official final summary, with explicit finite/unit checks."""
    summary = None
    for line in output.splitlines():
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(obj, dict) and 'Key' not in obj and 'Download' in obj and 'Upload' in obj:
            summary = obj
    if summary is None:
        raise RuntimeError('missing NDT7 final summary')
    def value(direction, metric, unit, maximum, optional=False):
        pair = (summary.get(direction) or {}).get(metric)
        if optional and (pair is None or (isinstance(pair, dict) and pair.get('Value') is None)):
            return ''
        if not isinstance(pair, dict) or pair.get('Unit') != unit:
            raise RuntimeError(f'unexpected {direction} {metric} units')
        try:
            number = float(pair['Value'])
        except (KeyError, TypeError, ValueError) as exc:
            raise RuntimeError(f'incomplete {direction} {metric}') from exc
        if not math.isfinite(number) or number < 0 or number > maximum or (metric == 'Throughput' and number == 0):
            raise RuntimeError(f'implausible {direction} {metric}: {number}')
        return round(number, 2 if metric == 'Throughput' else 1)
    return (value('Download', 'Throughput', 'Mbit/s', 2000),
            value('Upload', 'Throughput', 'Mbit/s', 500),
            value('Download', 'Latency', 'ms', 2000, optional=True),
            str(summary.get('ServerFQDN', '')))


# ── connectivity witnesses ────────────────────────────────────────────────────
def check_url(raw):
    """
    Lightweight GET witness check.

    Reads at most one byte and closes the response. Response time is retained
    only for temporary diagnostics; it is not a website-performance metric.
    """
    url = normalize_url(raw)
    req = urllib.request.Request(
        url,
        headers={
            'User-Agent': f'speedmon-bg/{VERSION}',
            'Range': 'bytes=0-0',
            'Cache-Control': 'no-cache',
        },
        method='GET',
    )
    t0 = time.perf_counter()

    try:
        with urllib.request.urlopen(
            req, timeout=TIMEOUT_SECS, context=ssl_ctx()
        ) as response:
            response.read(1)
        return 'up', round((time.perf_counter() - t0) * 1000)
    except (socket.timeout, TimeoutError):
        return 'timeout', round((time.perf_counter() - t0) * 1000)
    except urllib.error.HTTPError:
        # An HTTP response proves that the remote Internet host was reached.
        return 'up', round((time.perf_counter() - t0) * 1000)
    except urllib.error.URLError as exc:
        state = 'timeout' if isinstance(exc.reason, (socket.timeout, TimeoutError)) else 'down'
        return state, round((time.perf_counter() - t0) * 1000)
    except Exception:
        return 'down', round((time.perf_counter() - t0) * 1000)


def check_witnesses():
    return [(raw, *check_url(raw)) for raw in URL_TARGETS]


def any_reachable(results):
    return any(state == 'up' for _, state, _ in results)


def classify_connectivity():
    """
    Any 1/3 reachable => available.
    0/3 => wait 10 seconds and repeat.
    Only 0/3 twice => confirmed dropout.
    """
    first = check_witnesses()
    if any_reachable(first):
        return True, first, None

    print(f'  No witness response; confirming in {DROPOUT_RECHECK_SECS}s...')
    time.sleep(DROPOUT_RECHECK_SECS)
    second = check_witnesses()
    return any_reachable(second), first, second


# ── CSV writers ───────────────────────────────────────────────────────────────
def compatible_path(path, header):
    """Keep historical files intact; never append a different-width schema."""
    if path.exists() and path.stat().st_size:
        with path.open(newline='', encoding='utf-8-sig') as f:
            existing = next(csv.reader(f), [])
        if existing != header:
            alternate = path.with_name(path.stem + '_v23' + path.suffix)
            if alternate == path or '_v23' in path.stem:
                raise RuntimeError(f'CSV header mismatch: {path}')
            return compatible_path(alternate, header)
    return path


def append_speed_row(ts, ping='', down='', up='', result='', server='', epoch=None):
    path = compatible_path(speed_csv_path(), SPEED_HEADER)
    write_header = not path.exists() or path.stat().st_size == 0
    with open(path, 'a', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        if write_header:
            writer.writerow(SPEED_HEADER)
        writer.writerow([
            ts, time.time() if epoch is None else epoch, 'full', down, up, ping,
            MACHINE, {'Darwin': 'mac', 'Windows': 'win'}.get(platform.system(), platform.system().lower()) + '-python',
            'unknown', result, server,
        ])


def append_url_rows(ts, results, conn_ok, round_label='initial'):
    path = compatible_path(url_csv_path(), URL_HEADER)
    write_header = not path.exists() or path.stat().st_size == 0
    with open(path, 'a', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        if write_header:
            writer.writerow(URL_HEADER)
        for url, state, ms in results:
            writer.writerow([ts, url, state, ms,
                             '' if conn_ok is None else str(conn_ok).lower(), round_label])


# ── main test ─────────────────────────────────────────────────────────────────
def run_once():
    started = datetime.now()
    ts = fmt_ts(started)
    epoch = started.timestamp()
    print(f'\n[{ts}] Starting background measurement...')

    ndt_error = None
    down = up = ping = ''
    server = ''
    try:
        down, up, ping, server = run_ndt7()
        server_note = f'  server={server}' if server else ''
        print(f'  NDT7: down={down} Mb/s  up={up} Mb/s  latency={ping} ms{server_note}')
    except Exception as exc:
        ndt_error = str(exc)
        print(f'  NDT7 FAILED: {ndt_error}')

    print('  Checking connectivity witnesses...')
    conn_ok, first, second = classify_connectivity()

    for raw, state, ms in first:
        print(f'    {raw}: {state} ({ms} ms diagnostic)')
    append_url_rows(ts, first, True if any_reachable(first) else None, 'initial')

    if second is not None:
        print('  Confirmation round:')
        for raw, state, ms in second:
            print(f'    {raw}: {state} ({ms} ms diagnostic)')
        append_url_rows(fmt_ts(), second, conn_ok, 'confirm')

    if ndt_error is None:
        append_speed_row(ts, ping=ping, down=down, up=up, result='', server=server, epoch=epoch)
    elif conn_ok:
        append_speed_row(ts, result=f'fail: ndt7 {ndt_error[:160]} [conn_ok=true]', epoch=epoch)
    else:
        append_speed_row(ts, result='dropout: confirmed', epoch=epoch)

    print('  Connectivity:', 'AVAILABLE' if conn_ok else 'CONFIRMED DROPOUT')


def main():
    once = '--once' in sys.argv
    if not once:
        logger = logging.getLogger('speedmon')
        logger.setLevel(logging.INFO)
        handler = RotatingFileHandler(Path(__file__).with_name('speedmon_bg.log'),
                                      maxBytes=2_000_000, backupCount=3, encoding='utf-8')
        logger.addHandler(handler)
        class LogStream:
            def write(self, message):
                if message.strip(): logger.info(message.rstrip())
                return len(message)
            def flush(self):
                handler.flush()
        sys.stdout = sys.stderr = LogStream()
    print(f'speedmon_bg.py v{VERSION}  machine={MACHINE}  interval={INTERVAL_MINS}min')
    print(f'Speed log  : {speed_csv_path()}')
    print(f'URL log    : {url_csv_path()}')

    try:
        client = ndt7_path()
        print(f'NDT7 client: {client}')
    except RuntimeError as exc:
        print(f'NDT7 SETUP ERROR: {exc}', file=sys.stderr)
        if once:
            raise SystemExit(1)
        # Stay alive rather than triggering launchd KeepAlive respawn loops.
        while True:
            time.sleep(INTERVAL_MINS * 60)
            try:
                client = ndt7_path()
                print(f'NDT7 client found: {client}')
                break
            except RuntimeError:
                pass

    if once:
        run_once()
        return

    interval = INTERVAL_MINS * 60
    next_run = time.monotonic()
    while True:
        now = time.monotonic()
        if now < next_run:
            time.sleep(next_run - now)
        try:
            run_once()
        except Exception as exc:
            print(f'Collection failed: {exc}')
        next_run += interval
        # If the machine slept through one or more slots, resume at the next
        # future slot rather than running a burst of catch-up tests.
        now = time.monotonic()
        while next_run <= now:
            next_run += interval


if __name__ == '__main__':
    main()

# /// EOF
