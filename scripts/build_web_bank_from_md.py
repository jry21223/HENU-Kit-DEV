#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
from collections import Counter
from pathlib import Path
from typing import Any

try:
    from scripts.bank_metadata import sanitize_source_metadata
except ModuleNotFoundError:
    from bank_metadata import sanitize_source_metadata


QUESTION_RE = re.compile(r"^###\s+(\d+)\.\s+(单选题|多选题|判断题)\s*$")
SECTION_RE = re.compile(r"^##\s+(.+?)（第\d+-\d+题）\s*$")
OPTION_RE = re.compile(r"^-\s+([A-D])\.\s*(.*)$")


def parse_markdown(path: Path) -> dict[str, Any]:
    lines = path.read_text(encoding="utf-8").splitlines()
    questions: list[dict[str, Any]] = []
    chapter = "Web题库"
    idx = 0
    while idx < len(lines):
        line = lines[idx].strip()
        section = SECTION_RE.match(line)
        if section:
            chapter = section.group(1).strip()
            idx += 1
            continue
        match = QUESTION_RE.match(line)
        if not match:
            idx += 1
            continue

        number = int(match.group(1))
        type_text = match.group(2)
        q_type = {"单选题": "single", "多选题": "multi", "判断题": "judge"}[type_text]
        idx += 1
        content_parts: list[str] = []
        options: list[str] = []
        while idx < len(lines):
            raw = lines[idx]
            stripped = raw.strip()
            if stripped.startswith("---"):
                break
            if QUESTION_RE.match(stripped) or SECTION_RE.match(stripped):
                idx -= 1
                break
            option = OPTION_RE.match(stripped)
            if option:
                options.append(option.group(2).strip())
            elif stripped and not stripped.startswith("#"):
                content_parts.append(stripped)
            idx += 1
        content = "\n".join(content_parts).strip()
        questions.append(
            {
                "id": f"web_{len(questions) + 1:04d}",
                "number": str(number),
                "type": q_type,
                "chapter": chapter,
                "content": content,
                "options": options if q_type in {"single", "multi"} else None,
                "answer": None,
                "analysis": "",
                "stats": {"total": 0, "correct": 0, "rate": 0},
            }
        )
        idx += 1

    chapters = []
    for chapter_name in dict.fromkeys(q["chapter"] for q in questions):
        chapters.append({"id": f"ch{len(chapters) + 1:02d}", "name": chapter_name})
    chapter_id_by_name = {item["name"]: item["id"] for item in chapters}
    for q in questions:
        q["chapter_id"] = chapter_id_by_name[q["chapter"]]

    return {
        "meta": {
            "name": "Web前端题库",
            "version": "from-md",
            "color": "#00897b",
            "total": len(questions),
            "source_files": [str(path)],
            "chapters": chapters,
        },
        "questions": questions,
    }


def validate(bank: dict[str, Any]) -> list[str]:
    issues = []
    for q in bank["questions"]:
        if not q["content"]:
            issues.append(f"{q['id']}: empty content")
        if q["type"] in {"single", "multi"} and (not q.get("options") or len(q["options"]) != 4):
            issues.append(f"{q['id']}: expected 4 options, got {len(q.get('options') or [])}")
    return issues


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("generated/web_bank.json"))
    parser.add_argument("--issues-output", type=Path, default=Path("generated/web_bank_issues.txt"))
    args = parser.parse_args()

    bank = parse_markdown(args.input.expanduser())
    issues = validate(bank)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(sanitize_source_metadata(bank), ensure_ascii=False, indent=2), encoding="utf-8")
    args.issues_output.write_text("\n".join(issues) + ("\n" if issues else ""), encoding="utf-8")
    print(json.dumps({
        "questions": len(bank["questions"]),
        "types": dict(Counter(q["type"] for q in bank["questions"])),
        "chapters": len(bank["meta"]["chapters"]),
        "issues": len(issues),
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
