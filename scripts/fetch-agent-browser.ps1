param(
    [string]$Version = "0.43.0",
    [string]$Platform = "windows-x64",
    [string]$Destination = "build/agent-browser.exe",
    [string]$ExpectedSha256 = "EBCEF5EBEA93C7CCF0FE3F8C18EEE908AE9174DFEF8E52A3DDA6F24A59F630DC"
)

$ErrorActionPreference = "Stop"

$packageName = switch ($Platform) {
    "windows-x64" { "@agent-browser/cli-win32-x64-msvc" }
    "linux-x64" { "@agent-browser/cli-linux-x64-gnu" }
    "linux-x64-musl" { "@agent-browser/cli-linux-x64-musl" }
    "linux-arm64" { "@agent-browser/cli-linux-arm64-gnu" }
    "darwin-arm64" { "@agent-browser/cli-darwin-arm64" }
    "darwin-x64" { "@agent-browser/cli-darwin-x64" }
    default { throw "Unsupported platform: $Platform" }
}
$packageSlug = $packageName.Split("/")[1]

$destinationPath = if ([IO.Path]::IsPathRooted($Destination)) {
    $Destination
} else {
    [IO.Path]::GetFullPath((Join-Path (Get-Location) $Destination))
}
$destinationDir = Split-Path -Parent $destinationPath
New-Item -ItemType Directory -Force -Path $destinationDir | Out-Null

if (Test-Path $destinationPath) {
    $existingHash = (Get-FileHash $destinationPath -Algorithm SHA256).Hash
    if ($ExpectedSha256 -eq "" -or $existingHash -eq $ExpectedSha256) {
        Write-Host "agent-browser already present at $destinationPath"
        return
    }
}

$tarballUrl = "https://registry.npmjs.org/$packageName/-/$packageSlug-$Version.tgz"
$workDir = Join-Path (Get-Location) "temp/agent-browser-fetch"
New-Item -ItemType Directory -Force -Path $workDir | Out-Null
$tarball = Join-Path $workDir "$packageSlug-$Version.tgz"

Write-Host "Downloading $packageName $Version..."
Invoke-WebRequest -Uri $tarballUrl -OutFile $tarball

$extractDir = Join-Path $workDir "extract-$Platform"
if (Test-Path $extractDir) {
    Remove-Item -Recurse -Force $extractDir
}
New-Item -ItemType Directory -Force -Path $extractDir | Out-Null

& tar -xzf $tarball -C $extractDir
if ($LASTEXITCODE -ne 0) {
    throw "Failed to extract $tarball"
}

$binary = Get-ChildItem -Path (Join-Path $extractDir "package/bin") -File -Recurse |
    Where-Object { $_.Name -like "agent-browser*" -and $_.Extension -ne ".js" } |
    Select-Object -First 1
if ($null -eq $binary) {
    throw "agent-browser binary was not found in $packageName"
}

Copy-Item -LiteralPath $binary.FullName -Destination $destinationPath -Force

$actualHash = (Get-FileHash $destinationPath -Algorithm SHA256).Hash
if ($ExpectedSha256 -ne "" -and $actualHash -ne $ExpectedSha256) {
    Remove-Item -Force $destinationPath
    throw "SHA-256 mismatch for agent-browser: $actualHash (expected $ExpectedSha256)"
}

# Keep the Apache-2.0 license next to the binary for redistribution.
$licensePath = Join-Path $destinationDir "agent-browser-LICENSE"
if (-not (Test-Path $licensePath)) {
    $mainTarball = Join-Path $workDir "agent-browser-$Version.tgz"
    if (-not (Test-Path $mainTarball)) {
        Invoke-WebRequest -Uri "https://registry.npmjs.org/agent-browser/-/agent-browser-$Version.tgz" -OutFile $mainTarball
    }
    $licenseExtract = Join-Path $workDir "license-extract"
    if (Test-Path $licenseExtract) {
        Remove-Item -Recurse -Force $licenseExtract
    }
    New-Item -ItemType Directory -Force -Path $licenseExtract | Out-Null
    & tar -xzf $mainTarball -C $licenseExtract "package/LICENSE"
    $licenseSource = Join-Path $licenseExtract "package/LICENSE"
    if (Test-Path $licenseSource) {
        Copy-Item -LiteralPath $licenseSource -Destination $licensePath -Force
    }
}

Write-Host "agent-browser $Version written to $destinationPath"
Write-Host "SHA-256: $actualHash"
