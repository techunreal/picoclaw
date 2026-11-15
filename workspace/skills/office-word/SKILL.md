---
name: office-word
description: "Create, read, and edit Microsoft Word documents (.docx/.dotx) without Microsoft Office. Use when the user asks for a Word document, report, letter, memo, template, or wants text/tables extracted from or changed in a .docx file."
metadata: {"picoclaw":{"emoji":"📄","requires":{"bins":[]}}}
---

# Word documents (DOCX)

The Office Pack bundles Python and `python-docx`; Microsoft Word is not needed.
Use the bundled runtime and helper instead of ad-hoc installs.

## Runtime

- Python: `%PICOCLAW_OFFICE_PYTHON%` (set by the launcher). If empty, use `python`.
- Helper: `tools/office_cli.py` relative to the workspace.
- Never run `pip install`; the runtime is already provisioned.

PowerShell example:

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py docx-create --input spec.json --output report.docx
```

If the variable is empty, replace `& $env:PICOCLAW_OFFICE_PYTHON` with `python`.

## Create from a spec

Write `spec.json` in the workspace, then run `docx-create`:

```json
{
  "title": "Quarterly Report",
  "subtitle": "Q3 2026",
  "sections": [
    {
      "heading": "Summary",
      "level": 1,
      "paragraphs": ["Revenue grew 12%."],
      "bullets": ["North America led growth", "Costs stayed flat"],
      "table": {
        "headers": ["Metric", "Value"],
        "rows": [["Revenue", "1.2M"], ["Cost", "0.8M"]]
      }
    }
  ]
}
```

The helper writes headings, paragraphs, bullet lists, and tables, and returns
JSON with the output path and byte count.

## Read or extract

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py docx-read --input report.docx
```

Returns paragraphs (with style names) and tables as JSON. Use it before editing
so you know the current structure.

## Edit an existing document

`python-docx` cannot preserve every complex feature. For simple edits, write a
short Python script that opens the file, changes paragraphs/tables, and saves a
copy. Keep a backup of the original. For tracked changes, comments, fields, or
strict template fidelity, say that Word is required or offer a PDF/HTML
alternative instead of silently damaging the file.

## Rules

- Work inside the workspace; do not overwrite the user's file without a copy.
- Use `.docx` (not legacy `.doc`) output unless the user asks otherwise.
- Report the absolute output path and a one-line summary.
- Do not claim a document is perfect; state what was verified (opened and read
  back with `docx-read`).
