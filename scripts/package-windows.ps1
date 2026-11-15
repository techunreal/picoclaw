param(
    [string]$BuildDir = "build",
    [string]$DistDir = "",
    [string]$Version = "dev",
    [switch]$SkipFetchAgentBrowser,
    [switch]$SkipOfficeRuntime,
    [string]$OfficeRuntimeDir = "build/office-runtime"
)

$ErrorActionPreference = "Stop"

$root = (Get-Location).Path
$build = Join-Path $root $BuildDir
if ([string]::IsNullOrWhiteSpace($DistDir)) {
    $DistDir = Join-Path (Split-Path $root -Parent) "temp/dist"
}
$dist = if ([IO.Path]::IsPathRooted($DistDir)) {
    $DistDir
} else {
    Join-Path $root $DistDir
}
$stage = Join-Path $dist "stage-windows"

foreach ($required in @("picoclaw.exe", "picoclaw-launcher.exe")) {
    $path = Join-Path $build $required
    if (-not (Test-Path $path)) {
        throw "$required is missing from $build. Run the build and build-launcher targets first."
    }
}

if (-not $SkipFetchAgentBrowser) {
    $agentBrowser = Join-Path $build "agent-browser.exe"
    & (Join-Path $root "scripts/fetch-agent-browser.ps1") -Destination $agentBrowser
}

$agentBrowserPath = Join-Path $build "agent-browser.exe"
if (-not (Test-Path $agentBrowserPath)) {
    throw "agent-browser.exe is missing; the ZIP must include it for the browser tool."
}

if (Test-Path $stage) {
    Remove-Item -Recurse -Force $stage
}
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item (Join-Path $build "picoclaw.exe") $stage
Copy-Item (Join-Path $build "picoclaw-launcher.exe") $stage
Copy-Item $agentBrowserPath $stage

$agentBrowserLicense = Join-Path $build "agent-browser-LICENSE"
if (Test-Path $agentBrowserLicense) {
    Copy-Item $agentBrowserLicense (Join-Path $stage "agent-browser-LICENSE")
}

if (-not $SkipOfficeRuntime) {
    $runtime = if ([IO.Path]::IsPathRooted($OfficeRuntimeDir)) {
        $OfficeRuntimeDir
    } else {
        Join-Path $root $OfficeRuntimeDir
    }
    if (-not (Test-Path (Join-Path $runtime "python.exe"))) {
        Write-Host "Office runtime not found; preparing it now."
        & (Join-Path $root "scripts/prepare-office-runtime.ps1") -Destination $runtime
        if ($LASTEXITCODE -ne 0) {
            throw "Office runtime preparation failed."
        }
    }
    $stagePython = Join-Path $stage "tools/python"
    New-Item -ItemType Directory -Force -Path $stagePython | Out-Null
    Copy-Item (Join-Path $runtime "*") $stagePython -Recurse -Force

    $officeNotes = @"
PicoClaw Office Pack
====================

Word (.docx), Excel (.xlsx), PowerPoint (.pptx) ve PDF islemleri icin
gerekli Python calisma zamani ve kutuphaneleri "tools\python" altindadir.
Microsoft Office, Adobe Acrobat veya ayri bir Python kurulumu GEREKMEZ.

Desteklenen islemler:
- Word: olustur/oku, baslik/paragraf/liste/tablo
- Excel: olustur/oku, formul/hucre/sayfa, CSV
- PowerPoint: olustur/oku, slayt/not/gorsel
- PDF: olustur, metin cikar, birlestir, bol, dondur, form doldur

Kullanim: WebUI'da Agent > Skills bolumunde office-* skill'lerini acin ve
sohbette "Word belgesi olustur", "Excel tablosu yap", "PDF birlestir" gibi
isteklerde bulunun.

Sinirlar:
- OCR (taranmis PDF) icin Tesseract dahil degildir.
- Karmasik Office sablonlari/animasyonlari sinirli duzenlenir.
- Ofis islemi yalnizca istendiginde calisir; normal sohbette ek RAM kullanmaz.
"@
    Set-Content -Path (Join-Path $stage "OFFICE-PACK.txt") -Value $officeNotes -Encoding UTF8
}

foreach ($doc in @("LICENSE", "README.md")) {
    $source = Join-Path $root $doc
    if (Test-Path $source) {
        Copy-Item $source $stage
    }
}

$quickStart = @"
PicoClaw - Windows Hizli Baslangic
==================================

1. Bu ZIP'i bir klasore cikar (ornek: C:\PicoClaw).
2. "picoclaw-launcher.exe" dosyasina cift tikla.
3. Tarayicida http://localhost:18800 otomatik acilir.
4. Ilk acilista yonetici sifresi ister. Bu sifre sadece WebUI girisi icindir.
5. WebUI'da "Models" bolumunden bir model ekle (API anahtarinla).
6. Ustten "Start Gateway" dugmesine bas.
7. Artik sohbet edebilirsin.

Tarayici otomasyonu:
- "agent-browser.exe" pakete dahildir; ayri kurulum gerekmez.
- Tools > Browser'dan izinli siteleri ekleyip araci acin.
- Chrome/Edge yoksa arayuzden "Download browser engine" ile indirebilirsiniz.

Ofis paketi:
- Word, Excel, PowerPoint ve PDF icin gerekli Python "tools\python" altindadir.
- Microsoft Office veya ayri Python kurulumu gerekmez.
- Agent > Skills bolumunde office-word, office-excel, office-powerpoint, office-pdf gorunur.
- OCR (taranmis PDF) desteklenmez.

Notlar:
- "picoclaw.exe" arka planda calisan motordur; silme, tasima. Launcher onu ayni klasorde bulur.
- Kapatmak icin sistem tepsisindeki PicoClaw simgesine sag tik > Quit.
- Ayni anda iki PicoClaw calistirma; 18800 portu cakisir.
- Bu bir "$Version" surumudur. Eski kurulumunu yedek olarak sakla.
"@
Set-Content -Path (Join-Path $stage "BASLAT.txt") -Value $quickStart -Encoding UTF8

$zipPath = Join-Path $dist "PicoClaw-Windows-x86_64-$Version.zip"
if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zipPath -CompressionLevel Optimal

Write-Host "Package written to $zipPath"
Get-Item $zipPath | Select-Object Name, Length
