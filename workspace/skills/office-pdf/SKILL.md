---
name: office-pdf
description: "Create, read, merge, split, rotate, and fill PDF files without Adobe or Microsoft Office. Use when the user asks for a PDF, wants text extracted from a PDF, or needs PDFs combined, split, rotated, or form fields filled."
metadata: {"picoclaw":{"emoji":"📕","requires":{"bins":[]}}}
---

# PDF files

The Office Pack bundles Python, `pypdf`, and `reportlab`; Adobe Acrobat and
Microsoft Office are not needed. PicoClaw already extracts PDF text from chat
attachments; use this skill for files and for PDF creation/editing.

## Runtime

- Python: `%PICOCLAW_OFFICE_PYTHON%` (set by the launcher). If empty, use `python`.
- Helper: `tools/office_cli.py` relative to the workspace.
- Never run `pip install`.

## Create a PDF

```json
{
  "title": "Invoice 1042",
  "paragraphs": ["Due date: 2026-12-01", "Thank you for your business."],
  "table": {
    "headers": ["Item", "Amount"],
    "rows": [["Consulting", "1200"], ["Support", "300"]]
  }
}
```

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-create --input invoice.json --output invoice.pdf
```

## Read text

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-text --input report.pdf
```

Returns page-by-page text. Scanned/image-only PDFs have no text layer; OCR needs
Tesseract, which is not bundled. Say so instead of inventing content.

## Merge, split, rotate

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-merge --output all.pdf one.pdf two.pdf
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-split --input all.pdf --outdir pages
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-rotate --input scan.pdf --output fixed.pdf --degrees 90
```

## Form fields

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-fields --input form.pdf
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pdf-fill --input form.pdf --output filled.pdf --data values.json
```

`values.json` maps field names to values. Only fill fields the user provided;
never guess signatures or legal declarations. Verify with `pdf-text`/`pdf-fields`
after writing.

## Alternatives without the Office Pack

- HTML → PDF: Edge headless (`msedge --headless --print-to-pdf=...`).
- Text extraction from an uploaded PDF: PicoClaw chat attachments.

## Rules

- Work on copies; keep the original untouched unless asked.
- Report page counts, output paths, and any limits (OCR, encrypted PDFs).
- Encrypted PDFs may need a password from the user; never guess or brute-force.
