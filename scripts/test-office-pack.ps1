param(
    [string]$RuntimeDir = "build/office-runtime",
    [string]$WorkDir = ""
)

$ErrorActionPreference = "Stop"

$root = (Get-Location).Path
$python = if ([IO.Path]::IsPathRooted($RuntimeDir)) {
    Join-Path $RuntimeDir "python.exe"
} else {
    Join-Path (Join-Path $root $RuntimeDir) "python.exe"
}
if (-not (Test-Path $python)) {
    throw "Office runtime not found at $python. Run scripts/prepare-office-runtime.ps1 first."
}

$cli = Join-Path $root "workspace/tools/office_cli.py"
if (-not (Test-Path $cli)) {
    throw "Office helper not found at $cli."
}

if ([string]::IsNullOrWhiteSpace($WorkDir)) {
    $WorkDir = Join-Path ([IO.Path]::GetTempPath()) ("picoclaw-office-test-" + [guid]::NewGuid().ToString("N"))
}
New-Item -ItemType Directory -Force -Path $WorkDir | Out-Null

function Invoke-Office {
    param([string[]]$Arguments)
    $output = & $python $cli @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "office_cli failed: $($output -join "`n")"
    }
    return ($output -join "`n")
}

$docxSpec = Join-Path $WorkDir "docx.json"
$docxOut = Join-Path $WorkDir "test.docx"
@'
{"title":"Test","sections":[{"heading":"Summary","paragraphs":["Hello"],"bullets":["One","Two"],"table":{"headers":["A","B"],"rows":[[1,2]]}}]}
'@ | Set-Content -Encoding utf8 $docxSpec
Invoke-Office @("docx-create", "--input", $docxSpec, "--output", $docxOut) | Out-Null
$docxRead = Invoke-Office @("docx-read", "--input", $docxOut)
if ($docxRead -notmatch '"Hello"') { throw "DOCX read-back failed: $docxRead" }

$xlsxSpec = Join-Path $WorkDir "xlsx.json"
$xlsxOut = Join-Path $WorkDir "test.xlsx"
@'
{"sheets":[{"name":"Sheet1","rows":[["A","B"],[1,2]],"formulas":{"C1":"=A2+B2"},"bold_header":true}]}
'@ | Set-Content -Encoding utf8 $xlsxSpec
Invoke-Office @("xlsx-create", "--input", $xlsxSpec, "--output", $xlsxOut) | Out-Null
$xlsxRead = Invoke-Office @("xlsx-read", "--input", $xlsxOut)
if ($xlsxRead -notmatch "Sheet1") { throw "XLSX read-back failed: $xlsxRead" }

$pptxSpec = Join-Path $WorkDir "pptx.json"
$pptxOut = Join-Path $WorkDir "test.pptx"
@'
{"title":"Deck","slides":[{"title":"One","bullets":["x","y"],"notes":"note"}]}
'@ | Set-Content -Encoding utf8 $pptxSpec
Invoke-Office @("pptx-create", "--input", $pptxSpec, "--output", $pptxOut) | Out-Null
$pptxRead = Invoke-Office @("pptx-read", "--input", $pptxOut)
if ($pptxRead -notmatch '"One"') { throw "PPTX read-back failed: $pptxRead" }

$pdfSpec = Join-Path $WorkDir "pdf.json"
$pdfOut = Join-Path $WorkDir "test.pdf"
@'
{"title":"PDF","paragraphs":["Hello PDF"],"table":{"headers":["A","B"],"rows":[["1","2"]]}}
'@ | Set-Content -Encoding utf8 $pdfSpec
Invoke-Office @("pdf-create", "--input", $pdfSpec, "--output", $pdfOut) | Out-Null
$pdfText = Invoke-Office @("pdf-text", "--input", $pdfOut)
if ($pdfText -notmatch "Hello PDF") { throw "PDF text read-back failed: $pdfText" }

$merged = Join-Path $WorkDir "merged.pdf"
Invoke-Office @("pdf-merge", "--output", $merged, $pdfOut, $pdfOut) | Out-Null
$splitDir = Join-Path $WorkDir "pages"
Invoke-Office @("pdf-split", "--input", $merged, "--outdir", $splitDir) | Out-Null
$rotated = Join-Path $WorkDir "rotated.pdf"
Invoke-Office @("pdf-rotate", "--input", $pdfOut, "--output", $rotated, "--degrees", "90") | Out-Null

$results = @(
    (Get-Item $docxOut),
    (Get-Item $xlsxOut),
    (Get-Item $pptxOut),
    (Get-Item $pdfOut),
    (Get-Item $merged),
    (Get-Item $rotated)
)
Write-Host "Office pack smoke test passed."
$results | Select-Object Name, Length
Write-Host "Work dir: $WorkDir"
