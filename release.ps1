# Builds windows/amd64 and windows/arm64 release zips and MSI installers,
# mirroring release.sh (plus MSI packaging via the WiX toolset).
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

New-Item -ItemType Directory -Force -Path release | Out-Null

$dateStamp = Get-Date -Format "yyyyMMdd"

$wixAvailable = [bool](Get-Command wix -ErrorAction SilentlyContinue)
$wxsPath = Join-Path $PSScriptRoot "Package.wxs"
if (-not $wixAvailable) {
    Write-Warning "wix CLI not found (dotnet tool install --global wix); skipping MSI creation"
} elseif (-not (Test-Path $wxsPath)) {
    Write-Warning "$wxsPath not found; skipping MSI creation"
    $wixAvailable = $false
} else {
    # Idempotent; needed once for the WixUI feature tree dialogs.
    wix extension add -g WixToolset.UI.wixext | Out-Null
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

    $BaseName = "$BaseName.$dateStamp"

    $env:GOOS = "windows"
    $env:GOARCH = $GoArch
    go build -buildvcs=false -trimpath -ldflags "-H=windowsgui" -o TaskMgr98.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $GoArch" }

    Compress-Archive -Path TaskMgr98.exe -DestinationPath "release/$BaseName.zip" -Force

    if ($wixAvailable) {
        # wix.exe can fail to see files on mapped network drives, so stage all its
        # inputs and the output in a local temp folder.
        $stage = Join-Path ([IO.Path]::GetTempPath()) "TaskMgr98-msi"
        New-Item -ItemType Directory -Force -Path $stage | Out-Null
        Copy-Item $wxsPath (Join-Path $stage "Package.wxs") -Force
        Copy-Item (Join-Path $PSScriptRoot "TaskMgr98.exe") (Join-Path $stage "TaskMgr98.exe") -Force
        Copy-Item (Join-Path $PSScriptRoot "media\icon.ico") (Join-Path $stage "icon.ico") -Force

        # WixUI's license dialog needs RTF; wrap the plain-text LICENSE on the fly.
        $licenseText = Get-Content (Join-Path $PSScriptRoot "LICENSE") -Raw
        $licenseText = $licenseText.Replace('\', '\\').Replace('{', '\{').Replace('}', '\}')
        $licenseText = $licenseText -replace "\r?\n", "\par `r`n"
        $licenseRtf = Join-Path $stage "license.rtf"
        Set-Content -Path $licenseRtf -Encoding Ascii -Value ("{\rtf1\ansi\deff0{\fonttbl{\f0 Courier New;}}\f0\fs16 " + $licenseText + "}")

        $stagedMsi = Join-Path $stage "$BaseName.msi"
        wix build (Join-Path $stage "Package.wxs") -ext WixToolset.UI.wixext -arch $MsiArch `
            -d "ExePath=$(Join-Path $stage 'TaskMgr98.exe')" `
            -d "IconPath=$(Join-Path $stage 'icon.ico')" `
            -bindvariable "WixUILicenseRtf=$licenseRtf" `
            -o "$stagedMsi"
        if ($LASTEXITCODE -ne 0) { throw "wix build failed for $MsiArch" }

        Move-Item $stagedMsi (Join-Path $PSScriptRoot "release\$BaseName.msi") -Force
        Remove-Item -Recurse -Force $stage
    }

    Remove-Item TaskMgr98.exe
}

Remove-Item -Force -ErrorAction SilentlyContinue release/TaskMgr98.x64.*.zip, release/TaskMgr98.Arm64.*.zip,
    release/TaskMgr98.x64.*.msi, release/TaskMgr98.Arm64.*.msi

Build-Release -GoArch amd64 -MsiArch x64 -BaseName TaskMgr98.x64 -VersionInfoFlags @("-64")
Build-Release -GoArch arm64 -MsiArch arm64 -BaseName TaskMgr98.Arm64 -VersionInfoFlags @("-64", "-arm")

Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue

$created = "release/TaskMgr98.x64.$dateStamp.zip, release/TaskMgr98.Arm64.$dateStamp.zip"
if ($wixAvailable) {
    $created += ", release/TaskMgr98.x64.$dateStamp.msi, release/TaskMgr98.Arm64.$dateStamp.msi"
}
Write-Host "$created created"
