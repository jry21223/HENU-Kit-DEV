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


GROUP_RE = re.compile(r"^##\s+第\s*(\d+)\s*组\s*$")
QUESTION_RE = re.compile(r"^###\s+\d+\.\s+(填空题|判断题)\s*$")
ANSWER_RE = re.compile(r"^答案：\s*(.*)$")


def normalize_text(lines: list[str]) -> str:
    text = " ".join(line.strip() for line in lines if line.strip())
    text = text.replace("\u200b", "")
    return re.sub(r"\s+", " ", text).strip()


def split_answer_candidates(answer: str) -> str | list[str]:
    normalized = normalize_text(answer.splitlines())
    if " / " not in normalized:
        return normalized

    candidates = [
        item.strip()
        for item in normalized.split(" / ")
        if item.strip()
    ]
    return candidates if len(candidates) > 1 else normalized


def parse_answer_block(first_line: str, lines: list[str], start: int) -> tuple[str | list[str], int]:
    if first_line.strip():
        return split_answer_candidates(first_line), start

    answer_lines: list[str] = []
    idx = start
    while idx < len(lines):
        stripped = lines[idx].strip()
        if GROUP_RE.match(stripped) or QUESTION_RE.match(stripped):
            break
        if stripped:
            numbered = re.match(r"^\d+\.\s*(.+)$", stripped)
            item = numbered.group(1).strip() if numbered else stripped
            answer_lines.append(normalize_text([item]))
        idx += 1

    return "; ".join(answer_lines), idx


def parse_markdown(path: Path) -> dict[str, Any]:
    lines = path.read_text(encoding="utf-8").splitlines()
    questions: list[dict[str, Any]] = []
    chapter = "1"
    idx = 0

    while idx < len(lines):
        stripped = lines[idx].strip()

        group_match = GROUP_RE.match(stripped)
        if group_match:
            chapter = group_match.group(1)
            idx += 1
            continue

        question_match = QUESTION_RE.match(stripped)
        if not question_match:
            idx += 1
            continue

        type_text = question_match.group(1)
        q_type = "blank" if type_text == "填空题" else "judge"
        idx += 1

        content_lines: list[str] = []
        answer: Any = None
        while idx < len(lines):
            current = lines[idx].strip()
            if GROUP_RE.match(current) or QUESTION_RE.match(current):
                break

            answer_match = ANSWER_RE.match(current)
            if answer_match:
                answer, idx = parse_answer_block(answer_match.group(1), lines, idx + 1)
                break

            if current and not current.startswith("#") and not current.startswith(">"):
                content_lines.append(current)
            idx += 1

        content = normalize_text(content_lines)
        if q_type == "judge":
            answer_text = str(answer or "").strip()
            if answer_text not in {"正确", "错误"}:
                raise ValueError(f"判断题答案无效: {answer_text!r} near question {len(questions) + 1}")
            answer = answer_text == "正确"
        elif not answer:
            raise ValueError(f"填空题答案为空 near question {len(questions) + 1}")

        question_no = len(questions) + 1
        questions.append(
            {
                "id": f"cf_{question_no:04d}",
                "number": str(question_no),
                "type": q_type,
                "chapter": chapter,
                "chapter_id": f"ch{int(chapter):02d}",
                "content": content,
                "options": None,
                "answer": answer,
                "analysis": "",
                "stats": {"total": 0, "correct": 0, "rate": 0},
            }
        )

    chapters = [
        {"id": f"ch{int(item):02d}", "name": item}
        for item in dict.fromkeys(question["chapter"] for question in questions)
    ]

    return {
        "meta": {
            "name": "专业英语-计算机基础",
            "version": "from-md",
            "color": "#2563eb",
            "total": len(questions),
            "chapters": chapters,
            "source_files": [str(path)],
        },
        "questions": questions,
    }


def validate(bank: dict[str, Any]) -> list[str]:
    issues: list[str] = []
    for expected_index, question in enumerate(bank["questions"], start=1):
        if question["id"] != f"cf_{expected_index:04d}":
            issues.append(f"{question['id']}: id not sequential")
        if question["number"] != str(expected_index):
            issues.append(f"{question['id']}: number not sequential")
        if not question["content"]:
            issues.append(f"{question['id']}: empty content")
        if question["type"] == "blank" and not question["answer"]:
            issues.append(f"{question['id']}: empty blank answer")
        if question["type"] == "judge" and not isinstance(question["answer"], bool):
            issues.append(f"{question['id']}: invalid judge answer")
        if question.get("options") is not None:
            issues.append(f"{question['id']}: non-null options")
    return issues


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("generated/computer_fundamentals.json"))
    parser.add_argument("--issues-output", type=Path, default=Path("generated/computer_fundamentals_issues.txt"))
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
    return 1 if issues else 0


if __name__ == "__main__":
    raise SystemExit(main())
