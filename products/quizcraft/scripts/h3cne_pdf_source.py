"""Convert the H3CNE colour-annotated PDF into reviewable QuizCraft records.

Red text is source answer data and blue text is source commentary.  Neither is
part of the student's question.  This adapter does not verify protocol facts or
create new explanations; its bank is a structural candidate for later review.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any

import pdfplumber
from pdfplumber.utils import extract_text


QUESTION_START = re.compile(r"^[？?]?\s*(\d{1,3})\.[ \t]*(?!\d)")
OPTION_START = re.compile(r"(?<!\S)A[ \t]*[.．、][ \t]*")


def _text(chars: list[dict[str, Any]]) -> str:
    return extract_text(chars, x_tolerance=2, y_tolerance=4).strip() if chars else ""


def _colour(char: dict[str, Any]) -> str:
    value = char.get("non_stroking_color")
    if isinstance(value, (tuple, list)) and len(value) == 3:
        red, green, blue = value
        if red > 0.8 and green < 0.2 and blue < 0.2:
            return "answer"
        if blue > 0.8 and red < 0.2 and green < 0.2:
            return "analysis"
    return "body"


def _bounds(chars: list[dict[str, Any]]) -> list[float]:
    return [round(min(char[key] for char in chars), 3) for key in ("x0", "top")] + [
        round(max(char[key] for char in chars), 3) for key in ("x1", "bottom")]


def _read_lines(path: Path) -> list[dict[str, Any]]:
    lines = []
    with pdfplumber.open(path) as pdf:
        for number, page in enumerate(pdf.pages, start=1):
            for line in page.extract_text_lines(x_tolerance=2, y_tolerance=4):
                if line["top"] < 60 or line["top"] > page.height - 58:
                    continue
                chars = line["chars"]
                parts = {key: _text([char for char in chars if _colour(char) == key])
                         for key in ("body", "answer", "analysis")}
                if re.sub(r"\s+", "", parts["body"]) == "H3C网络学院":
                    continue
                if (line["top"] > page.height * 0.85 and line["x0"] > page.width * 0.75
                        and parts["body"].isdigit() and not parts["answer"] and not parts["analysis"]):
                    continue
                black_chars = [char for char in chars if _colour(char) == "body"]
                red_chars = [char for char in chars if _colour(char) == "answer"]
                annotations = []
                if QUESTION_START.match(parts["body"]) and black_chars:
                    left_edge = min(char["x0"] for char in black_chars)
                    annotations = [char for char in red_chars if char["text"] in "？?" and char["x0"] < left_edge]
                    red_chars = [char for char in red_chars if char not in annotations]
                if parts["analysis"] and not parts["body"]:
                    parts["analysis"] = _text([char for char in chars if _colour(char) != "body"])
                    red_chars = []
                parts["answer"] = _text(red_chars)
                if not any(parts.values()):
                    continue
                lines.append({
                    **parts,
                    "page": number,
                    "bbox": [round(line[key], 3) for key in ("x0", "top", "x1", "bottom")],
                    "answer_bbox": _bounds(red_chars) if red_chars else None,
                    "annotations": [{"page": number, "bbox": _bounds(annotations), "text": _text(annotations)}]
                    if annotations else [],
                })
            for image in page.images:
                lines.append({
                    "body": "", "answer": "", "analysis": "", "page": number,
                    "bbox": [round(image[key], 3) for key in ("x0", "top", "x1", "bottom")],
                    "image": True,
                })
    return sorted(lines, key=lambda line: (line["page"], line["bbox"][1], line["bbox"][0]))


def _source_record(number: int, lines: list[dict[str, Any]], source: Path) -> dict[str, Any]:
    body = "\n".join(line["body"] for line in lines if line["body"])
    body = QUESTION_START.sub("", body, count=1).strip()
    body = re.sub(r"_+[ \t]+_+", lambda match: re.sub(r"\s", "", match[0]), body)
    first_option = OPTION_START.search(body)
    matches = [first_option] if first_option else []
    for label in "BCDEFGH":
        match = re.search(rf"{label}[ \t]*[.．、][ \t]*", body[matches[-1].end():]) if matches else None
        if not match:
            break
        offset = matches[-1].end()
        match = re.compile(rf"{label}[ \t]*[.．、][ \t]*").search(body, offset)
        matches.append(match)
    options = [body[match.end():matches[index + 1].start() if index + 1 < len(matches) else len(body)].strip()
               for index, match in enumerate(matches)]
    content = body[:matches[0].start()].strip() if matches else body
    answer = "".join(line["answer"] for line in lines).strip()
    choice_answer = re.sub(r"\s+", "", answer).upper()
    normalized_answer: Any = answer
    if options and re.fullmatch(r"[A-H]+", choice_answer):
        indices = [ord(letter) - 65 for letter in dict.fromkeys(choice_answer)]
        normalized_answer = indices if len(indices) > 1 else indices[0]
    analysis = "\n".join(line["analysis"] for line in lines if line["analysis"])
    analysis = re.sub(r"(?m)^\s*[*＊]\s*", "", analysis).strip()
    source_id = f"gb0192_main_q{number:04d}"
    return {
        "id": source_id,
        "source_question_id": source_id,
        "number": str(number),
        "type": ("multi" if len(answer) > 1 else "single") if options else "blank",
        "content": content,
        "options": options,
        "answer": normalized_answer,
        "analysis": analysis,
        "chapter": "GB0-192 主卷",
        "source_file": source.name,
        "source_pages": sorted({line["page"] for line in lines}),
        "source_answer": answer,
        "source_regions": [{"page": line["page"], "bbox": line["bbox"]} for line in lines],
        "answer_spans": [{"page": line["page"], "bbox": line["answer_bbox"], "text": line["answer"]}
                         for line in lines if line["answer"]],
        "annotations": [span for line in lines for span in line.get("annotations", [])],
        "images": [{"page": line["page"], "bbox": line["bbox"]} for line in lines if line.get("image")],
    }


def _write_assets(source: Path, destination: Path, records: list[dict[str, Any]]) -> None:
    import pymupdf as fitz

    with fitz.open(source) as document:
        for record in records:
            for index, image in enumerate(record["images"], start=1):
                asset_path = f"assets/{record['id']}_p{image['page']:03d}_img{index:02d}.png"
                target = destination / asset_path
                target.parent.mkdir(parents=True, exist_ok=True)
                page = document[image["page"] - 1]
                pixmap = page.get_pixmap(matrix=fitz.Matrix(2, 2), clip=fitz.Rect(image["bbox"]))
                pixmap.save(target)
                image["asset_path"] = asset_path
                image["sha256"] = hashlib.sha256(target.read_bytes()).hexdigest()


def _write_review_markdown(destination: Path, records: list[dict[str, Any]]) -> None:
    lines = ["# H3CNE 主卷来源整理", "", "原卷结构及原说明，尚未完成逐题答案核验。图题保留作资料审核，未进入练习候选集。"]
    for record in records:
        lines.extend(["", f"## {record['number']}. {record['source_question_id']}", "",
                      f"来源：{record['source_file']}，PDF物理页{record['source_pages']}。", "",
                      record["content"], ""])
        lines.extend(f"- {chr(65 + index)}. {value}" for index, value in enumerate(record["options"] or []))
        lines.extend(["", f"原答案：{record['source_answer']}", "", record["analysis"] or "原卷未提供说明。"])
        for image in record["images"]:
            lines.extend(["", f"![原卷第{image['page']}页配图]({image['asset_path']})"])
    (destination / "review.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def convert_h3cne_pdf(
    source_path: str | Path,
    output_dir: str | Path,
    *,
    expected_count: int = 446,
    name: str = "H3CNE GB0-192 题库",
) -> dict[str, Any]:
    """Write source records, a structural bank candidate and a review sidecar."""
    project_root = Path(__file__).resolve().parents[1]
    if str(project_root) not in sys.path:
        sys.path.insert(0, str(project_root))
    import server

    source = Path(source_path)
    destination = Path(output_dir)
    records = []
    current: list[dict[str, Any]] = []
    for line in _read_lines(source):
        if current and len(records) + 1 == expected_count and re.fullmatch(r"网\s*络", line["body"]):
            break
        match = QUESTION_START.match(line["body"])
        if match and int(match[1]) == len(records) + (2 if current else 1):
            if current:
                records.append(_source_record(len(records) + 1, current, source))
            current = [line]
        elif current:
            current.append(line)
    if current:
        records.append(_source_record(len(records) + 1, current, source))
    if len(records) != expected_count:
        raise ValueError(f"Expected {expected_count} source questions, found {len(records)}")

    destination.mkdir(parents=True, exist_ok=True)
    _write_assets(source, destination, records)
    candidates = []
    quarantine = []
    for record in records:
        reasons = []
        if not record["source_answer"]:
            reasons.append("missing_source_answer")
        elif record["options"] and not isinstance(record["answer"], (int, list)):
            reasons.append("invalid_source_answer")
        if record["images"]:
            reasons.append("image_requires_rendering")
        elif re.search(r"如图|图中|图示|下图|上图", record["content"]):
            reasons.append("missing_source_image")
        if reasons:
            quarantine.append({"source_question_id": record["source_question_id"], "reasons": reasons})
        else:
            candidates.append(record)
    bank = server.build_standard_bank_data(name, candidates) if candidates else {
        "meta": {"name": name, "total": 0, "chapter_count": 0}, "questions": [],
    }
    source_meta = {"file": source.name, "sha256": hashlib.sha256(source.read_bytes()).hexdigest(),
                   "question_count": len(records)}
    result = {"bank": bank, "source_questions": records, "quarantine": quarantine, "source": source_meta}
    for filename, data in (("bank.json", bank), ("source_questions.json", records),
                           ("review.json", quarantine), ("source.json", source_meta)):
        (destination / filename).write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
    _write_review_markdown(destination, records)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--expected-count", type=int, default=446)
    args = parser.parse_args()
    result = convert_h3cne_pdf(args.input, args.output_dir, expected_count=args.expected_count)
    print(json.dumps({"source_count": len(result["source_questions"]),
                      "candidate_count": len(result["bank"]["questions"])}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
