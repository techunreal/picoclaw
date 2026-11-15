---
name: office-excel
description: "Create, read, and edit Microsoft Excel workbooks (.xlsx/.xlsm/.csv) without Microsoft Office. Use when the user asks for a spreadsheet, table, budget, formula, chart data, or wants an existing workbook inspected or changed."
metadata: {"picoclaw":{"emoji":"📊","requires":{"bins":[]}}}
---

# Excel workbooks (XLSX)

The Office Pack bundles Python, `openpyxl`, and `pandas`; Microsoft Excel is not
needed.

## Runtime

- Python: `%PICOCLAW_OFFICE_PYTHON%` (set by the launcher). If empty, use `python`.
- Helper: `tools/office_cli.py` relative to the workspace.
- Never run `pip install`.

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py xlsx-create --input spec.json --output budget.xlsx
```

## Create from a spec

```json
{
  "sheets": [
    {
      "name": "Budget",
      "rows": [
        ["Item", "Cost", "Tax", "Total"],
        ["Hosting", 100, 20, null],
        ["Tools", 50, 10, null]
      ],
      "formulas": {
        "D2": "=B2+C2",
        "D3": "=B3+C3",
        "B4": "=SUM(B2:B3)"
      },
      "bold_header": true,
      "widths": [24, 12, 12, 14],
      "freeze": "A2"
    }
  ]
}
```

Keep formulas as formulas (do not precompute) so the workbook stays editable.
The helper returns sheet names and the output path.

## Read or inspect

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py xlsx-read --input budget.xlsx
```

Returns every sheet as rows. For analysis or cleaning, use `pandas` in a short
Python script; for formulas/styles/preservation, use `openpyxl`.

## Rules

- Preserve existing sheets and styles when editing; test by reading back.
- CSV is plain data only; use `.xlsx` when types, formulas, or formatting matter.
- Do not claim computed values unless you recalculated with Excel or verified
  them independently; `openpyxl` does not evaluate formulas.
- Work on a copy unless the user asked to modify the original.
