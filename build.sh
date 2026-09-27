#!/bin/bash
set -e
# goversioninfo runs on the host (macOS) and embeds VERSIONINFO + icon + manifest
# (see versioninfo.json); -64 -arm together produce an arm64 COFF .syso.
go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -64 -arm -o rsrc_windows_arm64.syso versioninfo.json
GOOS=windows GOARCH=arm64 go build -buildvcs=false -ldflags "-H=windowsgui" -o TaskMgr98.exe .