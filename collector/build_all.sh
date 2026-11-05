#!/bin/sh
# Builds the collector for every platform into collector/bin/. Needs Go only to rebuild.
cd "$(dirname "$0")" || exit 1
go test ./... || exit 1
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '-s -w' -o bin/speedmon-collector-mac-arm64 . || exit 1
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o bin/speedmon-collector-mac-intel . || exit 1
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o bin/speedmon-collector-linux-amd64 . || exit 1
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '-s -w' -o bin/speedmon-collector-linux-arm64 . || exit 1
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o bin/speedmon-collector-win.exe . || exit 1
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '-s -w -H=windowsgui' -o bin/speedmon-collector-win-bg.exe . || exit 1
ls -la bin
