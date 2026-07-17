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


QUESTION_RE = re.compile(r"^##\s+(\d+)\.\s+单选题\s*$")
OPTION_RE = re.compile(r"^-\s+([A-D])\.\s*(.*)$")


def flush_option(options: dict[str, list[str]], current: str | None, buffer: list[str]) -> None:
    if current is not None:
        options[current] = buffer[:]


def parse_markdown(path: Path) -> dict[str, Any]:
    lines = path.read_text(encoding="utf-8").splitlines()
    questions: list[dict[str, Any]] = []
    idx = 0
    while idx < len(lines):
        match = QUESTION_RE.match(lines[idx].strip())
        if not match:
            idx += 1
            continue
        number = int(match.group(1))
        idx += 1
        content_parts: list[str] = []
        options_parts: dict[str, list[str]] = {}
        current_option: str | None = None
        option_buffer: list[str] = []

        while idx < len(lines):
            line = lines[idx]
            stripped = line.strip()
            if QUESTION_RE.match(stripped):
                idx -= 1
                break
            if stripped == "---":
                idx += 1
                break
            if stripped.startswith("## 备注"):
                break
            option = OPTION_RE.match(stripped)
            if option:
                flush_option(options_parts, current_option, option_buffer)
                current_option = option.group(1)
                option_buffer = [option.group(2).rstrip()] if option.group(2).strip() else []
            elif current_option is not None:
                option_buffer.append(line.rstrip())
            elif stripped and not stripped.startswith("#"):
                content_parts.append(line.rstrip())
            idx += 1
        flush_option(options_parts, current_option, option_buffer)

        options = []
        for letter in "ABCD":
            parts = options_parts.get(letter, [])
            option_text = "\n".join(parts).strip()
            options.append(option_text)

        questions.append(
            {
                "id": f"java_{len(questions) + 1:04d}",
                "number": str(number),
                "type": "single",
                "chapter_id": "ch01",
                "chapter": "Java题库",
                "content": "\n".join(content_parts).strip(),
                "options": options,
                "answer": None,
                "analysis": "",
                "stats": {"total": 0, "correct": 0, "rate": 0},
            }
        )
        idx += 1

    return {
        "meta": {
            "name": "Java程序设计题库",
            "version": "from-md",
            "color": "#f57c00",
            "total": len(questions),
            "source_files": [str(path)],
            "chapters": [{"id": "ch01", "name": "Java题库"}],
        },
        "questions": questions,
    }


def validate(bank: dict[str, Any]) -> list[str]:
    issues = []
    for q in bank["questions"]:
        if not q["content"]:
            issues.append(f"{q['id']}: empty content")
        if len(q.get("options") or []) != 4 or any(not str(opt).strip() for opt in q.get("options") or []):
            issues.append(f"{q['id']}: option issue")
    return issues


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("generated/java_bank.json"))
    parser.add_argument("--issues-output", type=Path, default=Path("generated/java_bank_issues.txt"))
    args = parser.parse_args()

    bank = parse_markdown(args.input.expanduser())
    issues = validate(bank)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(sanitize_source_metadata(bank), ensure_ascii=False, indent=2), encoding="utf-8")
    args.issues_output.write_text("\n".join(issues) + ("\n" if issues else ""), encoding="utf-8")
    print(json.dumps({
        "questions": len(bank["questions"]),
        "types": dict(Counter(q["type"] for q in bank["questions"])),
        "issues": len(issues),
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
