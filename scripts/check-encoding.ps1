# Fails when a tracked text file has a BOM or a common UTF-8 mojibake sequence.
# Usage: powershell -NoProfile -File scripts/check-encoding.ps1
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$patterns = @(
    [char]0x00E2 + [char]0x20AC, # "â€" - corrupted em dash / quotes
    [char]0x00E2 + [char]0x2020, # "â†" - corrupted arrows
    [char]0x00C3 + [char]0x00A2, # "Ã¢"
    [char]0x00C3 + [char]0x00A9, # "Ã©"
    [char]0x00C3 + [char]0x00A7, # "Ã§"
    [char]0x00C3 + [char]0x00BC, # "Ã¼"
    [char]0x00C3 + [char]0x00B6  # "Ã¶"
)
$extensions = @(".go", ".md", ".json", ".ts", ".tsx", ".yml", ".yaml", ".ps1", ".sh")
$skipDirs = @("node_modules", ".git", "build", "dist", "temp", "vendor")

$hits = New-Object System.Collections.Generic.List[string]

Get-ChildItem -Path $root -Recurse -File | ForEach-Object {
    $file = $_
    $relative = $file.FullName.Substring($root.Length + 1)
    $parts = $relative -split "[\\/]"
    if ($parts | Where-Object { $skipDirs -contains $_ }) { return }
    if ($extensions -notcontains $file.Extension.ToLowerInvariant()) { return }

    $bytes = [System.IO.File]::ReadAllBytes($file.FullName)
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $hits.Add("${relative}: UTF-8 BOM")
    }

    $text = [System.Text.Encoding]::UTF8.GetString($bytes)
    $lineNumber = 0
    foreach ($line in ($text -split "`n")) {
        $lineNumber++
        foreach ($pattern in $patterns) {
            if ($line.Contains($pattern)) {
                $hits.Add("${relative}:${lineNumber}: mojibake")
                break
            }
        }
    }
}

if ($hits.Count -gt 0) {
    Write-Host "Encoding check failed:" -ForegroundColor Red
    $hits | Select-Object -First 50 | ForEach-Object { Write-Host "  $_" }
    if ($hits.Count -gt 50) { Write-Host "  ... and $($hits.Count - 50) more" }
    exit 1
}

Write-Host "Encoding check passed." -ForegroundColor Green
exit 0
