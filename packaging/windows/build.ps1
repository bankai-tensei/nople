# Builds the installer of Nople for Windows into dist\:
#
#   pwsh packaging\windows\build.ps1 -Version 0.1.0
#
# Needs Go and Inno Setup 6 (winget install JRSoftware.InnoSetup).
param(
    [string]$Version = "0.0.0"
)
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\..\.."

$iscc = (Get-Command iscc -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
    $iscc = @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $iscc) { throw "Inno Setup 6 is not installed: winget install JRSoftware.InnoSetup" }

Push-Location $root
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=$Version" -o dist\windows\nople.exe .
    if ($LASTEXITCODE) { throw "go build failed" }
    & $iscc /Q "/DAppVersion=$Version" packaging\windows\nople.iss
    if ($LASTEXITCODE) { throw "ISCC failed" }
    Get-Item "dist\nople-$Version-windows-x64-setup.exe"
} finally {
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}
