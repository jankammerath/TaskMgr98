$arch = (go env GOARCH).Trim()
go run github.com/akavel/rsrc@latest -arch $arch -ico media/icon.ico -o "rsrc_windows_$arch.syso"
go build -buildvcs=false -ldflags "-H=windowsgui" -o TaskMgr98.exe .