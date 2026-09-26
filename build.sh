#!/bin/bash
GOOS=windows GOARCH=arm64 go build -buildvcs=false -ldflags "-H=windowsgui" -o TaskMgr98.exe .