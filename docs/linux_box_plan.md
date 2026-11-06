# Plan: run the background collector on the Linux box

Status: waiting for the box (decided 2026-11-04). Implementation deferred until a Linux reference host is available.

## Objective
Get a stable long-term record of the home internet connection: whether it works, is impaired,
varies by time of day, or drops out, with a clean before/after when fiber replaces cable.
The box is the reference: it stays put, never sleeps, and is wired. Laptops then mostly answer
"how is it here?", such as speeds around the property.

## The box
- Always-on Linux machine, also hosting legacy VMs that are used rarely (weeks or months apart),
  so their traffic should not skew tests.
- Collector runs on the **host OS, not in a VM**: a VM's virtual network would be measured too.
- **Wired** to the router, on a port and cable at least as fast as the line.
- Dropbox client installed, syncing this folder only as needed. Check its current Linux file
  system requirements before formatting. Keep VM disks out of Dropbox.
- Optional: a small UPS on the box and router, so power cuts are logged rather than left as gaps.

## Steps
Aim to finish before changing internet service.

1. Confirm the folder synced, then install, using `linux-amd64` or `linux-arm64` as fits:
   `collector/bin/speedmon-collector-linux-amd64 install --machine "<box name>"`
   This has never been run on Linux, so expect possible fixes. Its service is a systemd user unit.
2. So it runs without anyone logged in: `loginctl enable-linger <user>`.
3. Verify: check and speed rows appear in `data/<box name>/`, the network label is right, the box
   appears in the viewer from another computer, `query` works, and it survives a reboot.
4. Verify outage logic for real: unplug the router's internet cable for a few minutes, then the
   box's own cable. Confirm `internet` and `local` outages with correct lengths.
5. Add an annotation: box started as reference, with date.

## Features to build then
**Per-computer speed interval.** `speed_hours` now applies to all. Box: about every 4 hours.
Laptops: rare or manual. Laptops still run connectivity checks each minute.

**Watch mode**, for rare use when connection, power or Wi-Fi seem sketchy. For example `watch 24h`
on one computer.
- Connectivity checks every 15 seconds. These are the low-bandwidth rapid test and give outage
  length. Speed tests are not needed for that.
- **Must expire on its own**, so it cannot be forgotten.
- Speed tests only when triggered, with a daily cap of about 6-12: after an outage ends, when
  response time stays about double normal for 10+ minutes, or on a network change.
- Each triggered test logs why it ran. Watch periods are annotated automatically.
- Do not run speed tests on a fixed 10-15 minute schedule: at gigabit that is about 100 GB a day.

## Why these choices
- A speed test transfers about 1 GB at gigabit, so 4-hourly is about 200 GB a month of traffic.
  Stored logs stay tiny (tens of MB a year).
- M-Lab refuses strictly periodic clients, so test times stay randomized.
- Compare within one tool only: tools read very differently on the same day.
- CSVs stay the source of truth. SQLite is rebuilt locally, off Dropbox.

## What this enables afterward
Box vs. laptop separates Wi-Fi from line problems. Two computers failing together confirms a real
outage. Then build: Place field, status line, monthly summary, time-of-day heatmap, before/after service changes. The later LiveCode viewer reads the same logs.
