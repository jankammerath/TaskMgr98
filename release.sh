#!/bin/bash
set -e
cd "$(dirname "$0")"

mkdir -p release

# goversioninfo runs on the host (macOS) and embeds VERSIONINFO + icon + manifest
# (see versioninfo.json); -64 = amd64, -64 -arm = arm64 COFF .syso.
build() {
	local goarch="$1" zipname="$2"
	shift 2
	go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest "$@" -o "rsrc_windows_$goarch.syso" versioninfo.json
	GOOS=windows GOARCH="$goarch" go build -buildvcs=false -trimpath -ldflags "-H=windowsgui" -o TaskMgr98.exe .
	zip -j "release/$zipname" TaskMgr98.exe
	rm TaskMgr98.exe
}

rm -f release/TaskMgr98.x64.zip release/TaskMgr98.Arm64.zip
build amd64 TaskMgr98.x64.zip -64
build arm64 TaskMgr98.Arm64.zip -64 -arm

echo "release/TaskMgr98.x64.zip and release/TaskMgr98.Arm64.zip created"
