#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import subprocess
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    from scripts.bank_metadata import sanitize_source_metadata
except ModuleNotFoundError:
    from bank_metadata import sanitize_source_metadata


QUESTION_CLEAN_PATTERNS = [
    re.compile(r"更多考试资料请加.*"),
    re.compile(r"河南大学考试墙\s*QQ.*"),
    re.compile(r"^\s*\d+\s*$"),
    re.compile(r"^\s*$"),
]


@dataclass
class RawQuestion:
    chapter: str
    q_type: str
    number: int
    text: str


def pdf_to_text(path: Path) -> str:
    result = subprocess.run(
        ["pdftotext", "-layout", str(path), "-"],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return result.stdout.decode("utf-8", errors="replace")


def clean_lines(text: str) -> list[str]:
    lines: list[str] = []
    for raw in text.replace("\x0c", "\n").splitlines():
        line = raw.strip()
        if any(pattern.search(line) for pattern in QUESTION_CLEAN_PATTERNS):
            continue
        lines.append(normalize_text(line))
    return lines


def normalize_text(text: str) -> str:
    text = text.replace("（", "(").replace("）", ")")
    text = text.replace("．", ".").replace("。", "。")
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def detect_chapter(line: str) -> str | None:
    if line.startswith("答案速查"):
        return None
    if re.match(r"^(导论|第[一二三四五六七八九十]+章)\b", line):
        return line
    if "第一章 毛泽东思想" in line:
        return "第一章 毛泽东思想及其历史地位"
    return None


def detect_type(line: str) -> str | None:
    if "单项选择题" in line:
        return "single"
    if "多项选择题" in line:
        return "multi"
    if "判断题" in line:
        return "judge"
    return None


def extract_question_area(lines: list[str]) -> list[str]:
    start = 0
    for idx, line in enumerate(lines):
        if detect_chapter(line):
            start = idx
            break
    end = len(lines)
    for idx, line in enumerate(lines):
        if "答案速查" in line:
            end = idx
            break
    return lines[start:end]


def parse_raw_questions(lines: list[str]) -> list[RawQuestion]:
    questions: list[RawQuestion] = []
    chapter = "默认章节"
    q_type: str | None = None
    current_no: int | None = None
    current_parts: list[str] = []

    def flush() -> None:
        nonlocal current_no, current_parts
        if q_type and current_no is not None and current_parts:
            questions.append(
                RawQuestion(
                    chapter=chapter,
                    q_type=q_type,
                    number=current_no,
                    text=" ".join(current_parts),
                )
            )
        current_no = None
        current_parts = []

    for line in lines:
        next_chapter = detect_chapter(line)
        next_type = detect_type(line)
        if next_chapter:
            flush()
            chapter = next_chapter
            q_type = None
            continue
        if next_type:
            flush()
            q_type = next_type
            continue
        if not q_type:
            continue

        match = re.match(r"^(\d+)[.、]\s*(.+)$", line)
        if match:
            flush()
            current_no = int(match.group(1))
            current_parts = [match.group(2).strip()]
        elif current_no is not None:
            current_parts.append(line)

    flush()
    return questions


def answer_area(lines: list[str]) -> list[str]:
    for idx, line in enumerate(lines):
        if "答案速查" in line:
            return lines[idx + 1 :]
    return []


def answer_chapter_key(line: str) -> str | None:
    match = re.match(r"^(第[一二三四五六七八九十]+章|导论).*$", line)
    if match:
        return match.group(1)
    return None


def parse_choice_ranges(text: str) -> dict[int, str]:
    answers: dict[int, str] = {}
    pattern = re.compile(r"(\d+)-(\d+)|(\d+)\s*(?=[A-D])")
    matches = list(pattern.finditer(text))
    for idx, match in enumerate(matches):
        start = int(match.group(1) or match.group(3))
        end = int(match.group(2) or match.group(3))
        body_start = match.end()
        body_end = matches[idx + 1].start() if idx + 1 < len(matches) else len(text)
        letters = re.sub(r"[^A-D]", "", text[body_start:body_end].upper())
        for offset, number in enumerate(range(start, end + 1)):
            if offset < len(letters):
                answers[number] = letters[offset]
    return answers


def parse_multi_ranges(text: str) -> dict[int, str]:
    answers: dict[int, str] = {}
    pattern = re.compile(r"(\d+)-(\d+)|(\d+)\s*(?=[A-D])")
    matches = list(pattern.finditer(text))
    for idx, match in enumerate(matches):
        start = int(match.group(1) or match.group(3))
        end = int(match.group(2) or match.group(3))
        body_start = match.end()
        body_end = matches[idx + 1].start() if idx + 1 < len(matches) else len(text)
        tokens = re.findall(r"[A-D]+", text[body_start:body_end].upper())
        for offset, number in enumerate(range(start, end + 1)):
            if offset < len(tokens):
                answers[number] = tokens[offset]
    return answers


def parse_judge_ranges(text: str) -> dict[int, bool]:
    answers: dict[int, bool] = {}
    pattern = re.compile(r"(\d+)-(\d+)|(\d+)\s*(?=[对错])")
    matches = list(pattern.finditer(text))
    for idx, match in enumerate(matches):
        start = int(match.group(1) or match.group(3))
        end = int(match.group(2) or match.group(3))
        body_start = match.end()
        body_end = matches[idx + 1].start() if idx + 1 < len(matches) else len(text)
        values = re.findall(r"[对错]", text[body_start:body_end])
        for offset, number in enumerate(range(start, end + 1)):
            if offset < len(values):
                answers[number] = values[offset] == "对"
    return answers


def parse_answers(lines: list[str]) -> dict[tuple[str, str, int], Any]:
    grouped: dict[tuple[str, str], list[str]] = defaultdict(list)
    chapter_key = ""
    answer_type = ""
    for line in answer_area(lines):
        key = answer_chapter_key(line)
        if key:
            chapter_key = key
            answer_type = ""
            continue
        if line.startswith("单选："):
            answer_type = "single"
            grouped[(chapter_key, answer_type)].append(line.split("：", 1)[1])
        elif line.startswith("多选："):
            answer_type = "multi"
            grouped[(chapter_key, answer_type)].append(line.split("：", 1)[1])
        elif line.startswith("判断："):
            answer_type = "judge"
            grouped[(chapter_key, answer_type)].append(line.split("：", 1)[1])
        elif answer_type and re.match(r"^\d", line):
            grouped[(chapter_key, answer_type)].append(line)

    answers: dict[tuple[str, str, int], Any] = {}
    for (chapter, q_type), parts in grouped.items():
        text = " ".join(parts)
        if q_type == "single":
            parsed = parse_choice_ranges(text)
        elif q_type == "multi":
            parsed = parse_multi_ranges(text)
        else:
            parsed = parse_judge_ranges(text)
        for number, answer in parsed.items():
            answers[(chapter, q_type, number)] = answer
    return answers


def chapter_key(chapter: str) -> str:
    if "第一章" in chapter:
        return "第一章"
    match = re.search(r"(第[一二三四五六七八九十]+章|导论)", chapter)
    return match.group(1) if match else chapter


def split_options(raw: RawQuestion) -> tuple[str, list[str] | None]:
    text = raw.text
    if raw.q_type == "judge":
        return re.sub(r"\(\s*\)", "", text).strip(), None

    matches = list(re.finditer(r"(?<![A-Za-z])([A-D])\s*[、.]\s*", text))
    if not matches:
        return text, None

    stem = text[: matches[0].start()].strip()
    options: list[str] = []
    for idx, match in enumerate(matches):
        start = match.end()
        end = matches[idx + 1].start() if idx + 1 < len(matches) else len(text)
        option = text[start:end].strip()
        option = re.sub(r"\s+", " ", option)
        if option:
            options.append(option)
    stem = re.sub(r"\(\s*\)", "", stem).strip()
    return stem, options


def answer_to_indices(answer: Any, q_type: str) -> Any:
    if q_type == "judge":
        return answer
    if not isinstance(answer, str):
        return None
    values = [ord(letter) - 65 for letter in answer.upper() if "A" <= letter <= "D"]
    if q_type == "single":
        return values[0] if len(values) == 1 else None
    return values


def tokenize(text: str) -> list[str]:
    tokens: list[str] = []
    for match in re.finditer(r"[\u4e00-\u9fff]+|[A-Za-z0-9]+", text):
        value = match.group(0)
        if re.fullmatch(r"[\u4e00-\u9fff]+", value):
            if len(value) == 1:
                tokens.append(value)
            else:
                tokens.extend(value[idx : idx + 2] for idx in range(len(value) - 1))
        else:
            tokens.append(value.lower())
    return tokens


def build_rag_chunks(text: str) -> list[dict[str, Any]]:
    lines = clean_lines(text)
    chunks: list[dict[str, Any]] = []
    chapter = "教材重点"
    buffer: list[str] = []

    def flush() -> None:
        if not buffer:
            return
        body = " ".join(buffer).strip()
        if len(body) >= 20:
            chunks.append(
                {
                    "id": f"chunk_{len(chunks) + 1:04d}",
                    "chapter": chapter,
                    "text": body,
                    "tokens": tokenize(body),
                }
            )
        buffer.clear()

    for line in lines:
        next_chapter = detect_chapter(line)
        if next_chapter:
            flush()
            chapter = next_chapter
            continue
        if len(" ".join(buffer)) > 650:
            flush()
        buffer.append(line)
    flush()
    return chunks


def retrieve(chunks: list[dict[str, Any]], question_text: str, chapter: str, limit: int = 2) -> list[dict[str, Any]]:
    q_tokens = Counter(tokenize(question_text))
    scored: list[tuple[float, dict[str, Any]]] = []
    ch_key = chapter_key(chapter)
    for chunk in chunks:
        c_tokens = Counter(chunk.get("tokens") or [])
        overlap = sum(min(count, c_tokens.get(token, 0)) for token, count in q_tokens.items())
        score = float(overlap)
        if ch_key and ch_key in str(chunk.get("chapter")):
            score += 0.5
        if score > 0:
            scored.append((score, chunk))
    scored.sort(key=lambda item: item[0], reverse=True)
    return [chunk for _, chunk in scored[:limit]]


def build_analysis(question: dict[str, Any], contexts: list[dict[str, Any]]) -> str:
    q_type = question["type"]
    answer = question["answer"]
    options = question.get("options") or []
    if q_type == "judge":
        answer_text = "对" if answer else "错"
    elif q_type == "single":
        answer_text = f"{chr(65 + answer)}. {options[answer]}" if isinstance(answer, int) and answer < len(options) else str(answer)
    else:
        answer_text = "、".join(
            f"{chr(65 + idx)}. {options[idx]}" for idx in answer if isinstance(idx, int) and idx < len(options)
        )

    if contexts:
        evidence = contexts[0]["text"][:150]
        return f"正确答案为{answer_text}。讲义相关依据：{evidence}"
    return f"正确答案为{answer_text}。本题依据题库答案速查整理，建议结合对应章节教材重点复习。"


def build_bank(question_pdf: Path, lecture_pdf: Path) -> tuple[dict[str, Any], dict[str, Any], list[str]]:
    question_text = pdf_to_text(question_pdf)
    lecture_text = pdf_to_text(lecture_pdf)
    lines = clean_lines(question_text)
    raw_questions = parse_raw_questions(extract_question_area(lines))
    answers = parse_answers(lines)
    chunks = build_rag_chunks(lecture_text)
    issues: list[str] = []
    questions: list[dict[str, Any]] = []

    for raw in raw_questions:
        key = (chapter_key(raw.chapter), raw.q_type, raw.number)
        answer = answers.get(key)
        stem, options = split_options(raw)
        normalized_answer = answer_to_indices(answer, raw.q_type)
        if normalized_answer is None:
            issues.append(f"missing/invalid answer: {raw.chapter} {raw.q_type} #{raw.number}")
            continue
        if raw.q_type in {"single", "multi"} and (not options or len(options) < 2):
            issues.append(f"missing options: {raw.chapter} {raw.q_type} #{raw.number}")
            continue

        question = {
            "id": f"maogai_{len(questions) + 1:04d}",
            "number": str(raw.number),
            "type": raw.q_type,
            "chapter": raw.chapter,
            "content": stem,
            "options": options,
            "answer": normalized_answer,
            "analysis": "",
            "stats": {"total": 0, "correct": 0, "rate": 0},
        }
        contexts = retrieve(chunks, f"{stem} {' '.join(options or [])}", raw.chapter)
        question["analysis"] = build_analysis(question, contexts)
        if contexts:
            question["rag_refs"] = [item["id"] for item in contexts]
            question["rag_context"] = [
                {
                    "id": item["id"],
                    "chapter": item["chapter"],
                    "text": item["text"][:500],
                }
                for item in contexts
            ]
        questions.append(question)

    bank = {
        "meta": {
            "name": "毛泽东思想和中国特色社会主义理论体系概论",
            "version": "2026-summer",
            "color": "#b71c1c",
            "total": len(questions),
            "source_files": [str(question_pdf), str(lecture_pdf)],
        },
        "questions": questions,
    }
    rag = {
        "meta": {
            "name": "毛概2026夏讲义RAG索引",
            "source_file": str(lecture_pdf),
            "chunk_count": len(chunks),
        },
        "chunks": chunks,
    }
    return bank, rag, issues


def main() -> int:
    parser = argparse.ArgumentParser(description="Build Maogai QuizCraft bank JSON from the 2026 summer PDFs.")
    parser.add_argument("--questions-pdf", type=Path, required=True)
    parser.add_argument("--lecture-pdf", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("generated/maogai_2026_summer.json"))
    parser.add_argument("--rag-output", type=Path, default=Path("generated/maogai_2026_summer_rag.json"))
    parser.add_argument("--issues-output", type=Path, default=Path("generated/maogai_2026_summer_issues.txt"))
    args = parser.parse_args()

    bank, rag, issues = build_bank(args.questions_pdf.expanduser(), args.lecture_pdf.expanduser())
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.rag_output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(sanitize_source_metadata(bank), ensure_ascii=False, indent=2), encoding="utf-8")
    args.rag_output.write_text(json.dumps(sanitize_source_metadata(rag), ensure_ascii=False, indent=2), encoding="utf-8")
    args.issues_output.write_text("\n".join(issues) + ("\n" if issues else ""), encoding="utf-8")

    print(json.dumps({
        "questions": len(bank["questions"]),
        "rag_chunks": len(rag["chunks"]),
        "issues": len(issues),
        "output": str(args.output),
        "rag_output": str(args.rag_output),
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
