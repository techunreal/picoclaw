param(
    [string]$Version = "0.1.0-fork",
    [string]$GoExe = "go",
    [switch]$SkipOfficeRuntime,
    [switch]$SkipFetchAgentBrowser
)

$ErrorActionPreference = "Stop"

$root = (Get-Location).Path
$build = Join-Path $root "build"
$dist = Join-Path (Split-Path $root -Parent) "temp/dist"

$commit = (& git rev-parse --short HEAD).Trim()
if (-not $commit) {
    throw "Could not read git commit."
}
$go = (Get-Command $GoExe -ErrorAction SilentlyContinue).Source
if (-not $go) {
    throw "Go not found. Pass -GoExe <path-to-go.exe> or add Go to PATH."
}
$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$goVersion = (& $go version).Trim() -replace "^go version ", "" -replace " .*$", ""

Write-Host "Release $Version (commit $commit, go $goVersion)"

Push-Location (Join-Path $root "web/frontend")
try {
    pnpm build:backend
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed." }
} finally {
    Pop-Location
}

$ldflags = @(
    "-s",
    "-w",
    "-X github.com/sipeed/picoclaw/pkg/config.Version=$Version",
    "-X github.com/sipeed/picoclaw/pkg/config.GitCommit=$commit",
    "-X github.com/sipeed/picoclaw/pkg/config.BuildTime=$buildTime",
    "-X github.com/sipeed/picoclaw/pkg/config.GoVersion=$goVersion"
) -join " "

New-Item -ItemType Directory -Force -Path $build | Out-Null

Write-Host "Building picoclaw.exe"
& $go build -tags "goolm,stdjson" -trimpath -ldflags $ldflags -o (Join-Path $build "picoclaw.exe") ./cmd/picoclaw
if ($LASTEXITCODE -ne 0) { throw "picoclaw.exe build failed." }

Write-Host "Building picoclaw-launcher.exe"
# GUI subsystem: without it Windows opens a console window (Windows Terminal) that
# stays visible for the whole session.
$launcherLdflags = "$ldflags -H=windowsgui"
& $go build -tags "goolm,stdjson" -trimpath -ldflags $launcherLdflags -o (Join-Path $build "picoclaw-launcher.exe") ./web/backend
if ($LASTEXITCODE -ne 0) { throw "picoclaw-launcher.exe build failed." }

$packageArgs = @{
    BuildDir = "build"
    DistDir = $dist
    Version = $Version
}
if ($SkipOfficeRuntime) { $packageArgs.SkipOfficeRuntime = $true }
if ($SkipFetchAgentBrowser) { $packageArgs.SkipFetchAgentBrowser = $true }
& (Join-Path $root "scripts/package-windows.ps1") @packageArgs
if ($LASTEXITCODE -ne 0) { throw "Packaging failed." }

$zipPath = Join-Path $dist "PicoClaw-Windows-x86_64-$Version.zip"
if (-not (Test-Path $zipPath)) { throw "Expected ZIP missing: $zipPath" }

$hash = (Get-FileHash $zipPath -Algorithm SHA256).Hash
$sumsPath = Join-Path $dist "SHA256SUMS.txt"
"$hash  $(Split-Path $zipPath -Leaf)" | Set-Content -Encoding ascii $sumsPath

$size = (Get-Item $zipPath).Length
Write-Host ("Release ZIP: {0} ({1:N1} MiB)" -f $zipPath, ($size / 1MB))
Write-Host ("SHA256: {0}" -f $hash)
Write-Host ("Sums: {0}" -f $sumsPath)
