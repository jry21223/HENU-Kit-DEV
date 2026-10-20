#!/usr/bin/env python3
"""Prepare traceable H3C sources using QuizCraft's standard bank builder."""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any


def _source_code(source_file: str) -> str:
    match = re.search(r"(?:GB0[-_]?|RS\+_)(192|330|372|382|392)", source_file, re.I)
    if not match:
        raise ValueError("source filename must identify its GB0 examination code")
    return match.group(1)


def _question_body(body: str, *, se_format: bool) -> dict[str, Any]:
    images = re.findall(r"\[H3C_IMAGE:([^\]]+)\]", body)
    source_regions = []
    for match in re.finditer(r"\[H3C_REGION:(\d+):([0-9.,-]+)\]", body):
        coordinates = [float(value) for value in match[2].split(",")]
        if len(coordinates) == 4:
            source_regions.append({"page": int(match[1]), "bbox": coordinates})
    body = re.sub(r"\[H3C_(?:IMAGE|PAGE):[^\]]+\]", "", body)
    body = re.sub(r"\[H3C_REGION:[^\]]+\]", "", body)
    answer_match = re.search(
        r"^[ \t]*(?:正确答案|答案|Answer)[ \t]*[:：]?[ \t]*([^\n]*)", body, re.M | re.I
    )
    source_answer = answer_match.group(1).strip() if answer_match else ""
    question_text = body[: answer_match.start()] if answer_match else body
    explanation = ""
    if se_format:
        match = re.search(r"说明/参考\s*[:：]\s*([\s\S]*)", body)
        if match:
            explanation = match.group(1).strip()
    else:
        match = re.search(r"^\s*(?:解析|答案解析)\s*[:：]\s*([\s\S]*)", body, re.M)
        if match:
            explanation = match.group(1).strip()

    lines = question_text.splitlines()
    stem: list[str] = []
    options: list[str] = []
    labels: list[str] = []
    current: list[str] | None = None
    for line in lines:
        stripped = line.strip()
        if not stripped or re.fullmatch(r"[★☆]+", stripped):
            continue
        option = re.match(r"^([A-H])[.．、:：)）]\s*(.*)$", stripped)
        if option and ord(option[1]) - 65 == len(labels):
            if current is not None:
                options.append("\n".join(current).strip())
            labels.append(option[1])
            current = [option[2]]
        elif current is not None:
            current.append(stripped)
        else:
            stem.append(stripped)
    if current is not None:
        options.append("\n".join(current).strip())

    reasons: list[str] = []
    if images or re.search(r"如图|图中|图所示|图示|见图|下图|上图", "\n".join(stem)):
        reasons.append("figure_required")
    if not source_answer:
        reasons.append("missing_answer")
    if "不一致" in source_answer:
        reasons.append("source_answer_conflict")
    answer_text = re.split(r"[（(]", source_answer, maxsplit=1)[0].strip()
    if options:
        if not re.fullmatch(r"[A-H](?:[A-H\s,，、]*[A-H])?", answer_text):
            reasons.append("invalid_choice_answer")
            answer: Any = None
            kind = "single"
        else:
            indices = [ord(letter) - 65 for letter in re.findall(r"[A-H]", answer_text)]
            if len(set(indices)) != len(indices) or any(i >= len(options) for i in indices):
                reasons.append("answer_option_mismatch")
            kind = "multi" if len(indices) > 1 else "single"
            answer = indices if kind == "multi" else indices[0]
        if len(options) < 2 or any(not opt for opt in options):
            reasons.append("incomplete_options")
    else:
        kind, answer = "blank", answer_text
        if re.fullmatch(r"[A-H]+", answer_text):
            reasons.append("missing_options")
        if answer_text in {"正确", "错误", "对", "错", "True", "False"}:
            kind, answer = "judge", answer_text in {"正确", "对", "True"}
    if not stem:
        reasons.append("missing_stem")
    return {
        "type": kind,
        "content": "\n".join(stem).strip(),
        "options": options or None,
        "answer": answer,
        "analysis": explanation,
        "source_answer": source_answer,
        "review_reasons": reasons,
        "source_images": images,
        "source_regions": source_regions,
    }


def parse_h3c_text(text: str, *, source_file: str) -> dict[str, Any]:
    """Return every source item and only structurally complete bank candidates.

    Question starts are anchored whole-line labels, never bare numbers inside
    commands or addresses. A restarted source number creates a new segment.
    """
    from server import build_standard_bank_data

    code = _source_code(source_file)
    se_format = bool(re.search(r"^\s*问题\s+\d+\s*$", text, re.M))
    pattern = (r"^[ \t]*问题[ \t]+(\d+)[ \t]*$" if se_format else
               r"^[ \t]*[？?]?(\d+)(?:[.．][ \t]+(?=\S)|[.．](?=[^\d\s])|[、:：][ \t]*(?![:：])(?=\S))")
    numbered_starts = list(re.finditer(pattern, text, re.M))
    boundaries: list[tuple[int, int, re.Match[str] | None, str]] = [
        (match.start(), match.end(), match, "numbered") for match in numbered_starts
    ]
    if code == "372" and "新增" in source_file:
        boundaries.extend(
            (match.start(), match.end(), None, "un_numbered")
            for match in re.finditer(r"^[ \t]*AAA[ \t]+作为一种网络安全管理机制", text, re.M)
        )
    boundaries.sort(key=lambda item: item[0])
    source_questions: list[dict[str, Any]] = []
    candidates: list[dict[str, Any]] = []
    review: list[dict[str, Any]] = []
    segment = 1
    previous = 0
    version = re.search(r"\d{8}|\d{6}", source_file)
    source_kind = "main" if se_format else ("supp" if "新增" in source_file else "main") + (version[0] if version else "")
    un_numbered_index = 0
    for position, (start_offset, body_offset, start, boundary_kind) in enumerate(boundaries):
        if start is not None:
            number: int | str = int(start[1])
            if number <= previous:
                segment += 1
            previous = number
        else:
            un_numbered_index += 1
            number = f"un-numbered-{un_numbered_index}"
        end = boundaries[position + 1][0] if position + 1 < len(boundaries) else len(text)
        raw_text = text[start_offset : end].strip()
        body_start = start_offset if boundary_kind == "un_numbered" else body_offset
        question = _question_body(text[body_start : end], se_format=se_format)
        preceding_pages = re.findall(r"\[H3C_PAGE:(\d+)\]", text[:start_offset])
        pages = ([int(preceding_pages[-1])] if preceding_pages else []) + [
            int(page) for page in re.findall(r"\[H3C_PAGE:(\d+)\]", raw_text)
        ]
        un_numbered_suffix = f"_u{un_numbered_index:02d}" if boundary_kind == "un_numbered" else ""
        question.update({
            "id": (f"gb0{code}_{source_kind}_s{segment:02d}_q{number:04d}" if start is not None
                   else f"gb0{code}_{source_kind}_s{segment:02d}{un_numbered_suffix}_aaa"),
            "number": str(number),
            "chapter": f"GB0-{code} {'主卷' if se_format else '补充练习'}",
            "chapter_id": f"gb0{code}_{source_kind}",
            "source": {"file": Path(source_file).name, "segment": segment, "number": number,
                       "pages": sorted(set(pages)), "images": question.pop("source_images"),
                       "regions": question.pop("source_regions")},
            "raw_text": raw_text,
        })
        source_questions.append(question)
        reasons = question["review_reasons"]
        if not reasons:
            try:
                normalized = build_standard_bank_data(f"H3C GB0-{code}", [question])["questions"][0]
                candidates.append(normalized)
            except Exception as exc:
                reasons.append(f"normalization_failed: {exc}")
        if reasons:
            review.append({"id": question["id"], "reasons": reasons})
    return {"source_questions": source_questions, "questions": candidates, "review": review}


def convert_h3c_pdf(
    source_path: Path, output_dir: Path, *, expected_count: int
) -> dict[str, Any]:
    """Extract source text/figures and fail closed on a reviewed count mismatch.

    `bank.json` contains structurally usable candidates, not permission to
    publish them. `source_questions.json` also retains every withheld item.
    """
    import pymupdf
    from server import build_standard_bank_data

    source_path, output_dir = Path(source_path), Path(output_dir)
    source_sha256 = hashlib.sha256(source_path.read_bytes()).hexdigest()
    text_lines: list[str] = []
    pending_assets: list[tuple[str, bytes]] = []
    assets: list[dict[str, Any]] = []
    with pymupdf.open(source_path) as document:
        for page_number, page in enumerate(document, 1):
            text_lines.append(f"[H3C_PAGE:{page_number}]")
            image_number = 0
            for block in page.get_text("dict", sort=True)["blocks"]:
                if block["type"] == 0:
                    for line in block["lines"]:
                        if line.get("bbox"):
                            bbox = ",".join(f"{coordinate:.3f}" for coordinate in line["bbox"])
                            text_lines.append(f"[H3C_REGION:{page_number}:{bbox}]")
                        text_lines.append("".join(span["text"] for span in line["spans"]))
                elif block["type"] == 1:
                    image_number += 1
                    asset_id = f"{source_sha256[:12]}-p{page_number:03d}-i{image_number:02d}"
                    extension = block["ext"]
                    if extension not in {"png", "jpeg", "jpg", "bmp", "gif"}:
                        raise ValueError(f"unsupported extracted image format: {extension}")
                    relative_path = f"assets/{asset_id}.{extension}"
                    data = block["image"]
                    pending_assets.append((relative_path, data))
                    assets.append({"id": asset_id, "page": page_number, "bbox": list(block["bbox"]),
                                   "path": relative_path, "bytes": len(data),
                                   "sha256": hashlib.sha256(data).hexdigest()})
                    text_lines.append(f"[H3C_IMAGE:{asset_id}]")
    text = "\n".join(text_lines)
    result = parse_h3c_text(text, source_file=source_path.name)
    actual_count = len(result["source_questions"])
    if actual_count != expected_count:
        raise ValueError(f"expected {expected_count} source questions, found {actual_count}")
    result["assets"] = assets
    result["source"] = {"file": source_path.name, "sha256": source_sha256,
                        "question_count": actual_count}
    output_dir.mkdir(parents=True, exist_ok=True)
    for relative_path, data in pending_assets:
        target = output_dir / relative_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
    (output_dir / "source.txt").write_text(text, encoding="utf-8")
    for key in ("source_questions", "review", "assets", "source"):
        (output_dir / f"{key}.json").write_text(
            json.dumps(result[key], ensure_ascii=False, indent=2), encoding="utf-8"
        )
    if result["questions"]:
        bank = build_standard_bank_data(f"H3C GB0-{_source_code(source_path.name)}", result["questions"])
        (output_dir / "bank.json").write_text(json.dumps(bank, ensure_ascii=False, indent=2), encoding="utf-8")
    elif (output_dir / "bank.json").exists():
        (output_dir / "bank.json").unlink()
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--expected-count", type=int, required=True)
    args = parser.parse_args()
    result = convert_h3c_pdf(args.source, args.output, expected_count=args.expected_count)
    print(json.dumps({"source_questions": len(result["source_questions"]),
                      "candidates": len(result["questions"]), "withheld": len(result["review"])},
                     ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
    raise SystemExit(main())
