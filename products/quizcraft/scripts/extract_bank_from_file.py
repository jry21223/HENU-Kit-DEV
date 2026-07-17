#!/usr/bin/env python3
from __future__ import annotations

import argparse
import collections
import json
import sys
from pathlib import Path

from fastapi import HTTPException

PROJECT_ROOT = Path(__file__).resolve().parent.parent
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

import server  # noqa: E402


def parse_source(path: Path):
    suffix = path.suffix.lower()
    if suffix == ".pdf":
        text = server.extract_text_from_pdf(str(path))
        questions = server.parse_questions_from_text(text)
    elif suffix in {".docx", ".doc"}:
        text = server.extract_text_from_docx(str(path))
        questions = server.parse_questions_from_docx(str(path))
    elif suffix == ".json":
        text = path.read_text(encoding="utf-8-sig")
        questions = server.parse_questions_from_json_text(text)
    else:
        text = path.read_text(encoding="utf-8-sig")
        questions = server.parse_questions_from_text(text)
    return text, questions


def validate_bank(bank: dict) -> list[str]:
    issues: list[str] = []
    questions = bank.get("questions") or []
    ids = [question.get("id") for question in questions]
    duplicates = [item for item, count in collections.Counter(ids).items() if count > 1]
    if duplicates:
        issues.append(f"duplicate question ids: {duplicates[:10]}")

    for index, question in enumerate(questions, start=1):
        qid = question.get("id") or f"#{index}"
        q_type = question.get("type")
        options = question.get("options")
        answer = question.get("answer")
        if q_type in {"single", "multi"}:
            if not isinstance(options, list) or len(options) != 4:
                issues.append(f"{qid}: expected 4 options, got {len(options or [])}")
            if q_type == "single" and not isinstance(answer, int):
                issues.append(f"{qid}: single answer should be integer")
            if q_type == "multi" and not isinstance(answer, list):
                issues.append(f"{qid}: multi answer should be list")
        elif q_type == "judge":
            if not isinstance(answer, bool):
                issues.append(f"{qid}: judge answer should be boolean")
        else:
            issues.append(f"{qid}: unknown type {q_type!r}")
    return issues


def main() -> int:
    parser = argparse.ArgumentParser(description="Extract a source file into QuizCraft bank JSON.")
    parser.add_argument("--input", required=True, help="PDF/DOCX/TXT/JSON source file.")
    parser.add_argument("--name", required=True, help="Question bank display name.")
    parser.add_argument("--key", required=True, help="Question bank key / output filename stem.")
    parser.add_argument("--color", help="Bank color, for example #c62828.")
    parser.add_argument("--output", help="Output JSON path. Defaults to /tmp/<key>.json.")
    parser.add_argument("--allow-issues", action="store_true", help="Write output even if validation reports issues.")
    args = parser.parse_args()

    input_path = Path(args.input).expanduser().resolve()
    if not input_path.exists():
        raise SystemExit(f"Input file not found: {input_path}")

    output_path = (
        Path(args.output).expanduser().resolve()
        if args.output
        else Path("/tmp") / f"{args.key}.json"
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)

    try:
        _, questions = parse_source(input_path)
        bank = server.build_standard_bank_data(args.name, questions, args.color)
    except HTTPException as exc:
        raise SystemExit(f"Extraction failed: {exc.detail}") from exc
    issues = validate_bank(bank)

    type_counts = collections.Counter(question.get("type") for question in bank["questions"])
    print(f"input={input_path}")
    print(f"output={output_path}")
    print(f"total={bank['meta']['total']}")
    print(f"types={dict(type_counts)}")
    print(f"issues={len(issues)}")
    for issue in issues[:20]:
        print(f"[issue] {issue}")

    if issues and not args.allow_issues:
        raise SystemExit("Validation failed. Re-run with --allow-issues only after manual review.")

    output_path.write_text(json.dumps(bank, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
