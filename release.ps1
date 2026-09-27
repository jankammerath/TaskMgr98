# Builds windows/amd64 and windows/arm64 release zips and MSI installers,
# mirroring release.sh (plus MSI packaging via the WiX toolset).
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

New-Item -ItemType Directory -Force -Path release | Out-Null

$wixAvailable = [bool](Get-Command wix -ErrorAction SilentlyContinue)
if ($wixAvailable) {
    # Idempotent; needed once for the WixUI feature tree dialogs.
    wix extension add -g WixToolset.UI.wixext | Out-Null
} else {
    Write-Warning "wix CLI not found (dotnet tool install --global wix); skipping MSI creation"
}

# goversioninfo embeds VERSIONINFO + icon + manifest (see versioninfo.json);
# -64 = amd64, -64 -arm = arm64 COFF .syso.
function Build-Release {
    param(
        [string]$GoArch,
        [string]$MsiArch,
        [string]$BaseName,
        [string[]]$VersionInfoFlags
    )
    go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest @VersionInfoFlags -o "rsrc_windows_$GoArch.syso" versioninfo.json
    if ($LASTEXITCODE -ne 0) { throw "goversioninfo failed" }

    $env:GOOS = "windows"
    $env:GOARCH = $GoArch
    go build -buildvcs=false -trimpath -ldflags "-H=windowsgui" -o TaskMgr98.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $GoArch" }

    Compress-Archive -Path TaskMgr98.exe -DestinationPath "release/$BaseName.zip" -Force

    if ($wixAvailable) {
        wix build Package.wxs -ext WixToolset.UI.wixext -arch $MsiArch -d ExePath=TaskMgr98.exe -o "release/$BaseName.msi"
        if ($LASTEXITCODE -ne 0) { throw "wix build failed for $MsiArch" }
    }

    Remove-Item TaskMgr98.exe
}

Remove-Item -Force -ErrorAction SilentlyContinue release/TaskMgr98.x64.zip, release/TaskMgr98.Arm64.zip,
    release/TaskMgr98.x64.msi, release/TaskMgr98.Arm64.msi

Build-Release -GoArch amd64 -MsiArch x64 -BaseName TaskMgr98.x64 -VersionInfoFlags @("-64")
Build-Release -GoArch arm64 -MsiArch arm64 -BaseName TaskMgr98.Arm64 -VersionInfoFlags @("-64", "-arm")

Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue

Write-Host "release/TaskMgr98.x64.zip, release/TaskMgr98.Arm64.zip$(if ($wixAvailable) { ', release/TaskMgr98.x64.msi, release/TaskMgr98.Arm64.msi' }) created"
