---
name: office-powerpoint
description: "Create, read, and edit Microsoft PowerPoint presentations (.pptx) without Microsoft Office. Use when the user asks for slides, a deck, a pitch, or wants text, notes, or images extracted from a .pptx file."
metadata: {"picoclaw":{"emoji":"📽️","requires":{"bins":[]}}}
---

# PowerPoint decks (PPTX)

The Office Pack bundles Python and `python-pptx`; Microsoft PowerPoint is not
needed.

## Runtime

- Python: `%PICOCLAW_OFFICE_PYTHON%` (set by the launcher). If empty, use `python`.
- Helper: `tools/office_cli.py` relative to the workspace.
- Never run `pip install`.

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pptx-create --input deck.json --output deck.pptx
```

## Create from a spec

```json
{
  "title": "Product Update",
  "subtitle": "October 2026",
  "slides": [
    {
      "title": "What changed",
      "bullets": ["Faster setup", "Offline mode", "New office tools"],
      "notes": "Keep this slide under one minute."
    },
    {
      "title": "Next steps",
      "bullets": ["Pilot with 5 users", "Collect feedback"],
      "image": "assets/screenshot.png"
    }
  ]
}
```

The helper uses the default title/content layouts, adds speaker notes, and places
an image on a slide when the path exists. It returns the slide count.

## Read or extract

```powershell
& $env:PICOCLAW_OFFICE_PYTHON tools/office_cli.py pptx-read --input deck.pptx
```

Returns per-slide text and notes as JSON. Use it to review a deck before editing.

## Design limits

`python-pptx` preserves existing layouts and themes reasonably well, but complex
animations, transitions, SmartArt, and some chart features are not fully
editable. For a design-heavy deck, offer a simpler outline or an HTML slide deck
instead of claiming pixel-perfect fidelity.

## Rules

- Do not overwrite the user's deck without a copy.
- Keep bullets short; one idea per line.
- Verify by reading the deck back and report slide count and output path.
