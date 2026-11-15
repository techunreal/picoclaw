# Office Pack

The Windows ZIP ships a portable Python runtime and four office skills so the
agent can create and read Word, Excel, PowerPoint, and PDF files without
Microsoft Office, Adobe Acrobat, or a system Python install.

## What ships

- `workspace/skills/office-word/SKILL.md`
- `workspace/skills/office-excel/SKILL.md`
- `workspace/skills/office-powerpoint/SKILL.md`
- `workspace/skills/office-pdf/SKILL.md`
- `workspace/tools/office_cli.py` — shared JSON-driven helper
- `tools/python/` in the ZIP — embeddable Python 3.13 (amd64) plus pinned
  packages

The launcher and CLI set `PICOCLAW_OFFICE_PYTHON` to
`<app>\tools\python\python.exe` when that file exists. Skills use it and fall
back to `python` only when the variable is empty.

## Runtime contents

| Component | Version | License |
|---|---|---|
| Python (embeddable, amd64) | 3.13.15 | PSF |
| python-docx | 1.2.0 | MIT |
| openpyxl | 3.1.5 | MIT |
| python-pptx | 1.0.2 | MIT |
| pypdf | 7.1.0 | BSD-3-Clause |
| reportlab | 5.1.0 | BSD-3-Clause |
| Pillow, lxml, XlsxWriter, et-xmlfile, typing_extensions, charset-normalizer | transitive | MIT/BSD/Apache-2.0/HPND |

The embeddable Python download is verified with SHA256
`D1F04D990AEE1253D8569E8E5104E30FA9F5FA830899F14843448872D936A2CF`
(3.13.15 amd64).

## Build

```powershell
powershell -File scripts/prepare-office-runtime.ps1 -Force
powershell -File scripts/package-windows.ps1 -Version dev
```

`package-windows.ps1` prepares the runtime automatically when it is missing.
Use `-SkipOfficeRuntime` for a lean build without office support.

The runtime is built with Python 3.13 x64; `pip install --target` writes only
cp313/win_amd64 wheels into `tools/python/Lib/site-packages`. Nothing is
installed system-wide.

## Capabilities

- **Word:** create documents with title, headings, paragraphs, bullets, tables;
  read paragraphs/tables; simple edits with `python-docx`.
- **Excel:** create workbooks with sheets, values, formulas, widths, freeze
  panes; read values/formulas; analysis with `pandas` if needed.
- **PowerPoint:** create decks with title/content slides, notes, images; read
  slide text/notes.
- **PDF:** create text/table PDFs; extract text; merge, split, rotate; list and
  fill AcroForm fields.

## Limitations

- No OCR. Scanned PDFs need Tesseract, which is not bundled.
- Complex Office features (tracked changes, comments, animations, SmartArt,
  some charts) are not fully preserved or editable.
- `openpyxl` does not calculate formulas; Excel or LibreOffice is required to
  recalculate.
- The runtime is Windows x64 only, matching the ZIP.
- Office work happens only on demand; no background process or idle RAM.

## Tests

The build smoke test creates and reads back one DOCX, XLSX, PPTX, and PDF using
the packaged runtime, then checks merge/split/rotate. Run:

```powershell
powershell -File scripts/test-office-pack.ps1 -RuntimeDir build/office-runtime
```

## Updates

Python and library versions are pinned. Updating them means rebuilding the
runtime and shipping a new ZIP; there is no silent auto-update. Review release
notes and CVEs when bumping versions.
