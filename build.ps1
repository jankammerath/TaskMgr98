$arch = (go env GOARCH).Trim()
$flags = switch ($arch) {
    "amd64" { @("-64") }
    "arm64" { @("-64", "-arm") }
    default { @() }
}
go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest @flags -o "rsrc_windows_$arch.syso" versioninfo.json
go build -buildvcs=false -ldflags "-H=windowsgui" -o TaskMgr98.exe .