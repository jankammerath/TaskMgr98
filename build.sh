#!/bin/bash
#!/bin/bash
set -e
# rsrc runs on the host (macOS); -arch picks the target COFF machine type for the .syso.
go run github.com/akavel/rsrc@latest -arch arm64 -ico media/icon.ico -o rsrc_windows_arm64.syso
GOOS=windows GOARCH=arm64 go build -buildvcs=false -ldflags "-H=windowsgui" -o TaskMgr98.exe .