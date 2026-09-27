# Builds windows/amd64 and windows/arm64 release zips, mirroring release.sh.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

New-Item -ItemType Directory -Force -Path release | Out-Null

# goversioninfo embeds VERSIONINFO + icon + manifest (see versioninfo.json);
# -64 = amd64, -64 -arm = arm64 COFF .syso.
function Build-Release {
    param(
        [string]$GoArch,
        [string]$ZipName,
        [string[]]$VersionInfoFlags
    )
    go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest @VersionInfoFlags -o "rsrc_windows_$GoArch.syso" versioninfo.json
    if ($LASTEXITCODE -ne 0) { throw "goversioninfo failed" }

    $env:GOOS = "windows"
    $env:GOARCH = $GoArch
    go build -buildvcs=false -trimpath -ldflags "-H=windowsgui" -o TaskMgr98.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $GoArch" }

    Compress-Archive -Path TaskMgr98.exe -DestinationPath "release/$ZipName" -Force
    Remove-Item TaskMgr98.exe
}

Remove-Item -Force -ErrorAction SilentlyContinue release/TaskMgr98.x64.zip, release/TaskMgr98.Arm64.zip

Build-Release -GoArch amd64 -ZipName TaskMgr98.x64.zip -VersionInfoFlags @("-64")
Build-Release -GoArch arm64 -ZipName TaskMgr98.Arm64.zip -VersionInfoFlags @("-64", "-arm")

Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue

Write-Host "release/TaskMgr98.x64.zip and release/TaskMgr98.Arm64.zip created"
