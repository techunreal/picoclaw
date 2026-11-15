param(
    [string]$Destination = "build/office-runtime",
    [string]$PythonExe = "python",
    [string]$PythonVersion = "3.13.15",
    [string]$EmbedSha256 = "D1F04D990AEE1253D8569E8E5104E30FA9F5FA830899F14843448872D936A2CF",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

$root = (Get-Location).Path
$dest = if ([IO.Path]::IsPathRooted($Destination)) {
    $Destination
} else {
    Join-Path $root $Destination
}
$cache = Join-Path $root "build/office-runtime-cache"
$embedZip = Join-Path $cache "python-$PythonVersion-embed-amd64.zip"
$embedUrl = "https://www.python.org/ftp/python/$PythonVersion/python-$PythonVersion-embed-amd64.zip"

function Get-PythonCommand {
    param([string]$Candidate)
    if ($Candidate -and (Get-Command $Candidate -ErrorAction SilentlyContinue)) {
        return (Get-Command $Candidate).Source
    }
    throw "Python not found. Install Python 3.13 x64 or pass -PythonExe <path>."
}

$hostPython = Get-PythonCommand $PythonExe
$hostVersion = (& $hostPython -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')").Trim()
if ($hostVersion -ne "3.13") {
    throw "Office runtime must be built with Python 3.13 x64 (found $hostVersion)."
}

if ($Force -and (Test-Path $dest)) {
    Remove-Item -Recurse -Force $dest
}
if (Test-Path (Join-Path $dest "python.exe")) {
    Write-Host "Office runtime already present at $dest (use -Force to rebuild)."
    exit 0
}

New-Item -ItemType Directory -Force -Path $cache | Out-Null
if (-not (Test-Path $embedZip)) {
    Write-Host "Downloading $embedUrl"
    Invoke-WebRequest -UseBasicParsing $embedUrl -OutFile $embedZip -TimeoutSec 300
}
$actualHash = (Get-FileHash $embedZip -Algorithm SHA256).Hash.ToUpperInvariant()
if ($EmbedSha256 -and $actualHash -ne $EmbedSha256.ToUpperInvariant()) {
    throw "Python embed SHA256 mismatch: $actualHash (expected $EmbedSha256). Update -EmbedSha256 after reviewing the download."
}

if (Test-Path $dest) {
    Remove-Item -Recurse -Force $dest
}
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Expand-Archive -Path $embedZip -DestinationPath $dest -Force

$sitePackages = Join-Path $dest "Lib/site-packages"
New-Item -ItemType Directory -Force -Path $sitePackages | Out-Null

$packages = @(
    "python-docx==1.2.0",
    "openpyxl==3.1.5",
    "python-pptx==1.0.2",
    "pypdf==7.1.0",
    "reportlab==5.1.0"
)
Write-Host "Installing office packages into $sitePackages"
& $hostPython -m pip install --disable-pip-version-check --no-input --only-binary=:all: --target $sitePackages @packages
if ($LASTEXITCODE -ne 0) {
    throw "pip install failed for the office runtime."
}

# Embeddable Python ignores site-packages unless the ._pth file opts in.
$pth = Get-ChildItem $dest -Filter "python*._pth" | Select-Object -First 1
if (-not $pth) {
    throw "python*._pth not found in the embeddable runtime."
}
@(
    "python$($PythonVersion.Split('.')[0])$($PythonVersion.Split('.')[1]).zip",
    ".",
    "Lib\site-packages",
    "import site"
) | Set-Content -Encoding ascii $pth.FullName

# Drop caches and test suites; keep package metadata for license auditing.
Get-ChildItem $sitePackages -Recurse -Directory -Filter "__pycache__" -ErrorAction SilentlyContinue |
    Remove-Item -Recurse -Force
Get-ChildItem $sitePackages -Recurse -Directory -Filter "tests" -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notmatch "dist-info" } |
    Remove-Item -Recurse -Force
Get-ChildItem $sitePackages -Recurse -File -Filter "*.pyc" -ErrorAction SilentlyContinue |
    Remove-Item -Force

$probe = & (Join-Path $dest "python.exe") -c "import docx, openpyxl, pptx, pypdf, reportlab; print('ok')"
if ($probe.Trim() -ne "ok") {
    throw "Office runtime import probe failed: $probe"
}

@"
PicoClaw Office Pack runtime
Python: $PythonVersion (embeddable, amd64)
Packages:
  python-docx 1.2.0
  openpyxl 3.1.5
  python-pptx 1.0.2
  pypdf 7.1.0
  reportlab 5.1.0
Built: $(Get-Date -Format o)
"@ | Set-Content -Encoding utf8 (Join-Path $dest "RUNTIME.txt")

$size = (Get-ChildItem $dest -Recurse -File | Measure-Object Length -Sum).Sum
Write-Host ("Office runtime ready: {0} ({1:N1} MB)" -f $dest, ($size / 1MB))
