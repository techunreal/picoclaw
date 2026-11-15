#!/usr/bin/env python3
"""PicoClaw Office Pack helper.

Run with the bundled runtime:
  "%PICOCLAW_OFFICE_PYTHON%" tools/office_cli.py <command> ...

Commands:
  docx-create --input spec.json --output out.docx
  docx-read   --input in.docx
  xlsx-create --input spec.json --output out.xlsx
  xlsx-read   --input in.xlsx
  pptx-create --input spec.json --output out.pptx
  pptx-read   --input in.pptx
  pdf-create  --input spec.json --output out.pdf
  pdf-text    --input in.pdf
  pdf-merge   --output out.pdf in1.pdf in2.pdf ...
  pdf-split   --input in.pdf --outdir dir
  pdf-rotate  --input in.pdf --output out.pdf --degrees 90
  pdf-fields  --input in.pdf
  pdf-fill    --input in.pdf --output out.pdf --data fields.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path


def fail(message: str, code: int = 2) -> None:
    print(json.dumps({"ok": False, "error": message}), file=sys.stderr)
    raise SystemExit(code)


def load_json(path: str) -> dict:
    try:
        with open(path, "r", encoding="utf-8-sig") as handle:
            return json.load(handle)
    except FileNotFoundError:
        fail(f"input file not found: {path}")
    except json.JSONDecodeError as exc:
        fail(f"invalid JSON in {path}: {exc}")
    return {}


def write_result(result: dict) -> None:
    print(json.dumps(result, ensure_ascii=False))


def ensure_parent(path: str) -> None:
    Path(path).resolve().parent.mkdir(parents=True, exist_ok=True)


def cmd_docx_create(args: argparse.Namespace) -> None:
    from docx import Document
    from docx.shared import Pt

    spec = load_json(args.input)
    document = Document()
    styles = document.styles["Normal"]
    styles.font.name = "Calibri"
    styles.font.size = Pt(11)

    if spec.get("title"):
        document.add_heading(str(spec["title"]), level=0)
    if spec.get("subtitle"):
        document.add_paragraph(str(spec["subtitle"]))

    for section in spec.get("sections", []):
        level = max(1, min(int(section.get("level", 1)), 9))
        if section.get("heading"):
            document.add_heading(str(section["heading"]), level=level)
        for paragraph in section.get("paragraphs", []):
            document.add_paragraph(str(paragraph))
        for bullet in section.get("bullets", []):
            document.add_paragraph(str(bullet), style="List Bullet")
        table = section.get("table")
        if table and table.get("headers"):
            headers = [str(value) for value in table["headers"]]
            rows = table.get("rows", [])
            table_obj = document.add_table(rows=1, cols=len(headers))
            table_obj.style = "Table Grid"
            for index, header in enumerate(headers):
                table_obj.rows[0].cells[index].text = header
            for row in rows:
                cells = table_obj.add_row().cells
                for index, value in enumerate(row[: len(headers)]):
                    cells[index].text = str(value)

    ensure_parent(args.output)
    document.save(args.output)
    write_result({"ok": True, "file": args.output, "bytes": os.path.getsize(args.output)})


def cmd_docx_read(args: argparse.Namespace) -> None:
    from docx import Document

    document = Document(args.input)
    paragraphs = [
        {"style": paragraph.style.name, "text": paragraph.text}
        for paragraph in document.paragraphs
        if paragraph.text.strip()
    ]
    tables = []
    for table in document.tables:
        tables.append([[cell.text for cell in row.cells] for row in table.rows])
    write_result({"ok": True, "paragraphs": paragraphs, "tables": tables})


def cmd_xlsx_create(args: argparse.Namespace) -> None:
    from openpyxl import Workbook
    from openpyxl.styles import Font

    spec = load_json(args.input)
    workbook = Workbook()
    workbook.remove(workbook.active)

    for sheet_spec in spec.get("sheets", []):
        sheet = workbook.create_sheet(title=str(sheet_spec.get("name", "Sheet"))[:31])
        rows = sheet_spec.get("rows", [])
        for row_index, row in enumerate(rows, start=1):
            for column_index, value in enumerate(row, start=1):
                sheet.cell(row=row_index, column=column_index, value=value)
        for cell_ref, formula in sheet_spec.get("formulas", {}).items():
            sheet[cell_ref] = formula
        if sheet_spec.get("bold_header") and rows:
            for cell in sheet[1]:
                cell.font = Font(bold=True)
        for index, width in enumerate(sheet_spec.get("widths", []), start=1):
            sheet.column_dimensions[sheet.cell(row=1, column=index).column_letter].width = float(width)
        sheet.freeze_panes = sheet_spec.get("freeze", "A2")

    if not workbook.sheetnames:
        workbook.create_sheet("Sheet1")

    ensure_parent(args.output)
    workbook.save(args.output)
    write_result({
        "ok": True,
        "file": args.output,
        "sheets": workbook.sheetnames,
        "bytes": os.path.getsize(args.output),
    })


def cmd_xlsx_read(args: argparse.Namespace) -> None:
    from openpyxl import load_workbook

    workbook = load_workbook(args.input, data_only=False)
    sheets = {}
    for sheet in workbook.worksheets:
        sheets[sheet.title] = [
            [cell.value for cell in row]
            for row in sheet.iter_rows()
            if any(cell.value is not None for cell in row)
        ]
    write_result({"ok": True, "sheets": sheets})


def cmd_pptx_create(args: argparse.Namespace) -> None:
    from pptx import Presentation
    from pptx.util import Inches

    spec = load_json(args.input)
    presentation = Presentation()
    title_text = str(spec.get("title", "Presentation"))
    title_slide = presentation.slides.add_slide(presentation.slide_layouts[0])
    title_slide.shapes.title.text = title_text
    if spec.get("subtitle") and len(title_slide.placeholders) > 1:
        title_slide.placeholders[1].text = str(spec["subtitle"])

    for slide_spec in spec.get("slides", []):
        slide = presentation.slides.add_slide(presentation.slide_layouts[1])
        slide.shapes.title.text = str(slide_spec.get("title", ""))
        body = slide.placeholders[1].text_frame
        body.clear()
        bullets = slide_spec.get("bullets", [])
        for index, bullet in enumerate(bullets):
            paragraph = body.paragraphs[0] if index == 0 else body.add_paragraph()
            paragraph.text = str(bullet)
            paragraph.level = 0
        if slide_spec.get("notes"):
            slide.notes_slide.notes_text_frame.text = str(slide_spec["notes"])
        image = slide_spec.get("image")
        if image and os.path.exists(str(image)):
            slide.shapes.add_picture(str(image), Inches(1), Inches(4.5), width=Inches(6))

    ensure_parent(args.output)
    presentation.save(args.output)
    write_result({"ok": True, "file": args.output, "slides": len(presentation.slides)})


def cmd_pptx_read(args: argparse.Namespace) -> None:
    from pptx import Presentation

    presentation = Presentation(args.input)
    slides = []
    for index, slide in enumerate(presentation.slides, start=1):
        texts = []
        for shape in slide.shapes:
            if hasattr(shape, "text") and shape.text.strip():
                texts.append(shape.text.strip())
        notes = ""
        if slide.has_notes_slide:
            notes = slide.notes_slide.notes_text_frame.text.strip()
        slides.append({"slide": index, "text": texts, "notes": notes})
    write_result({"ok": True, "slides": slides})


def cmd_pdf_create(args: argparse.Namespace) -> None:
    from reportlab.lib import colors
    from reportlab.lib.pagesizes import A4
    from reportlab.lib.styles import getSampleStyleSheet
    from reportlab.platypus import Paragraph, SimpleDocTemplate, Spacer, Table, TableStyle

    spec = load_json(args.input)
    ensure_parent(args.output)
    document = SimpleDocTemplate(args.output, pagesize=A4)
    styles = getSampleStyleSheet()
    story = []
    if spec.get("title"):
        story.append(Paragraph(str(spec["title"]), styles["Title"]))
        story.append(Spacer(1, 12))
    for paragraph in spec.get("paragraphs", []):
        story.append(Paragraph(str(paragraph), styles["BodyText"]))
        story.append(Spacer(1, 8))
    table = spec.get("table")
    if table and table.get("headers"):
        data = [[str(cell) for cell in table["headers"]]]
        for row in table.get("rows", []):
            data.append([str(cell) for cell in row])
        table_obj = Table(data, repeatRows=1)
        table_obj.setStyle(TableStyle([
            ("BACKGROUND", (0, 0), (-1, 0), colors.lightgrey),
            ("GRID", (0, 0), (-1, -1), 0.5, colors.grey),
            ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
        ]))
        story.append(table_obj)
    if not story:
        story.append(Paragraph(" ", styles["BodyText"]))
    document.build(story)
    write_result({"ok": True, "file": args.output, "bytes": os.path.getsize(args.output)})


def cmd_pdf_text(args: argparse.Namespace) -> None:
    from pypdf import PdfReader

    reader = PdfReader(args.input)
    pages = []
    for index, page in enumerate(reader.pages, start=1):
        pages.append({"page": index, "text": page.extract_text() or ""})
    write_result({"ok": True, "pages": pages})


def cmd_pdf_merge(args: argparse.Namespace) -> None:
    from pypdf import PdfWriter

    if len(args.inputs) < 2:
        fail("pdf-merge needs at least two input PDFs")
    writer = PdfWriter()
    for path in args.inputs:
        writer.append(path)
    ensure_parent(args.output)
    with open(args.output, "wb") as handle:
        writer.write(handle)
    write_result({"ok": True, "file": args.output, "inputs": len(args.inputs)})


def cmd_pdf_split(args: argparse.Namespace) -> None:
    from pypdf import PdfReader, PdfWriter

    reader = PdfReader(args.input)
    Path(args.outdir).mkdir(parents=True, exist_ok=True)
    outputs = []
    for index, page in enumerate(reader.pages, start=1):
        writer = PdfWriter()
        writer.add_page(page)
        path = os.path.join(args.outdir, f"page-{index:03d}.pdf")
        with open(path, "wb") as handle:
            writer.write(handle)
        outputs.append(path)
    write_result({"ok": True, "pages": len(outputs), "files": outputs})


def cmd_pdf_rotate(args: argparse.Namespace) -> None:
    from pypdf import PdfReader, PdfWriter

    reader = PdfReader(args.input)
    writer = PdfWriter()
    for page in reader.pages:
        page.rotate(args.degrees)
        writer.add_page(page)
    ensure_parent(args.output)
    with open(args.output, "wb") as handle:
        writer.write(handle)
    write_result({"ok": True, "file": args.output, "degrees": args.degrees})


def cmd_pdf_fields(args: argparse.Namespace) -> None:
    from pypdf import PdfReader

    reader = PdfReader(args.input)
    fields = {}
    for page in reader.pages:
        page_fields = page.get_fields() or {}
        for name, field in page_fields.items():
            fields[name] = {
                "type": field.get("/FT"),
                "value": str(field.get("/V", "")),
                "alternatives": [str(item) for item in field.get("/_States_", [])],
            }
    write_result({"ok": True, "fields": fields})


def cmd_pdf_fill(args: argparse.Namespace) -> None:
    from pypdf import PdfReader, PdfWriter

    values = load_json(args.data)
    reader = PdfReader(args.input)
    writer = PdfWriter()
    writer.append(reader)
    for page in writer.pages:
        page_fields = page.get_fields() or {}
        applicable = {key: str(value) for key, value in values.items() if key in page_fields}
        if applicable:
            page.update_page_form_field_values(page, applicable, auto_regenerate=False)
    if "/AcroForm" in writer._root_object:
        writer._root_object["/AcroForm"].update({"/NeedAppearances": True})  # noqa: SLF001
    ensure_parent(args.output)
    with open(args.output, "wb") as handle:
        writer.write(handle)
    write_result({"ok": True, "file": args.output, "filled": len(values)})


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="PicoClaw Office Pack helper")
    sub = parser.add_subparsers(dest="command", required=True)

    for name, handler in (
        ("docx-create", cmd_docx_create),
        ("docx-read", cmd_docx_read),
        ("xlsx-create", cmd_xlsx_create),
        ("xlsx-read", cmd_xlsx_read),
        ("pptx-create", cmd_pptx_create),
        ("pptx-read", cmd_pptx_read),
        ("pdf-create", cmd_pdf_create),
        ("pdf-text", cmd_pdf_text),
        ("pdf-fields", cmd_pdf_fields),
    ):
        command = sub.add_parser(name)
        command.add_argument("--input", required=True)
        if name.endswith("-create"):
            command.add_argument("--output", required=True)
        command.set_defaults(func=handler)

    merge = sub.add_parser("pdf-merge")
    merge.add_argument("--output", required=True)
    merge.add_argument("inputs", nargs="+")
    merge.set_defaults(func=cmd_pdf_merge)

    split = sub.add_parser("pdf-split")
    split.add_argument("--input", required=True)
    split.add_argument("--outdir", required=True)
    split.set_defaults(func=cmd_pdf_split)

    rotate = sub.add_parser("pdf-rotate")
    rotate.add_argument("--input", required=True)
    rotate.add_argument("--output", required=True)
    rotate.add_argument("--degrees", type=int, default=90)
    rotate.set_defaults(func=cmd_pdf_rotate)

    fill = sub.add_parser("pdf-fill")
    fill.add_argument("--input", required=True)
    fill.add_argument("--output", required=True)
    fill.add_argument("--data", required=True)
    fill.set_defaults(func=cmd_pdf_fill)

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
    try:
        args.func(args)
    except ImportError as exc:
        fail(f"office library missing: {exc}. Run the bundled Office Pack runtime.")
    except Exception as exc:  # noqa: BLE001 - report errors as JSON for the agent
        fail(f"{type(exc).__name__}: {exc}", code=1)


if __name__ == "__main__":
    main()
