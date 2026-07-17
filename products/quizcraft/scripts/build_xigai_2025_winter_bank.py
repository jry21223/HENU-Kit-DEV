#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import tempfile
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    from scripts.bank_metadata import sanitize_source_metadata
except ModuleNotFoundError:
    from bank_metadata import sanitize_source_metadata


CLEAN_PATTERNS = [
    re.compile(r"更多(考试)?资料请加.*"),
    re.compile(r"河南大学考试墙\s*QQ.*"),
    re.compile(r"河南大学小过儿\s*QQ.*"),
    re.compile(r"严禁任何个人组织商家盗用或售卖"),
    re.compile(r"^\s*\d+\s*$"),
    re.compile(r"^\s*$"),
]

CN_NUM = {
    "一": 1,
    "二": 2,
    "三": 3,
    "四": 4,
    "五": 5,
    "六": 6,
    "七": 7,
    "八": 8,
    "九": 9,
    "十": 10,
    "十一": 11,
    "十二": 12,
    "十三": 13,
    "十四": 14,
    "十五": 15,
    "十六": 16,
    "十七": 17,
}


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


def ocr_tail_pages(path: Path, pages: int) -> str:
    if pages <= 0 or not shutil.which("pdftoppm") or not shutil.which("tesseract"):
        return ""
    info = subprocess.run(["pdfinfo", str(path)], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    match = re.search(r"^Pages:\s+(\d+)", info.stdout, re.MULTILINE)
    if not match:
        return ""
    total_pages = int(match.group(1))
    first_page = max(1, total_pages - pages + 1)
    with tempfile.TemporaryDirectory(prefix="xigai_ocr_") as tmp:
        prefix = Path(tmp) / "page"
        subprocess.run(["pdftoppm", "-r", "220", "-png", "-f", str(first_page), "-l", str(total_pages), str(path), str(prefix)], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        parts = []
        for image in sorted(Path(tmp).glob("*.png")):
            out_prefix = image.with_suffix("")
            subprocess.run(["tesseract", str(image), str(out_prefix), "-l", "chi_sim+eng", "--psm", "6"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            txt = out_prefix.with_suffix(".txt")
            if txt.exists():
                parts.append(txt.read_text(encoding="utf-8", errors="replace"))
        return "\n".join(parts)


def normalize_text(text: str) -> str:
    text = text.replace("（", "(").replace("）", ")")
    text = text.translate(str.maketrans({"Ａ": "A", "Ｂ": "B", "Ｃ": "C", "Ｄ": "D"}))
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def clean_lines(text: str) -> list[str]:
    lines: list[str] = []
    for raw in text.replace("\x0c", "\n").splitlines():
        line = normalize_text(raw.strip())
        has_answer_payload = any(marker in line for marker in ("单选", "多选", "判断"))
        if any(pattern.search(line) for pattern in CLEAN_PATTERNS) and not has_answer_payload:
            continue
        if has_answer_payload:
            line = re.sub(r"^.*?(?=(单选|多选|判断)[：:。，,])", "", line)
        lines.append(line)
    return lines


def without_front_matter(text: str, skip_pages: int = 1) -> str:
    pages = text.split("\x0c")
    return "\n".join(pages[skip_pages:]) if len(pages) > skip_pages else text


def detect_chapter(line: str) -> str | None:
    if "答案速查" in line or line == "目录":
        return None
    if line == "导论":
        return "导论"
    if re.match(r"^第[一二三四五六七八九十]+章$", line):
        return line
    match = re.match(r"^(第[一二三四五六七八九十]+章\s+.+)$", line)
    if match:
        return match.group(1)
    return None


def chapter_id(chapter: str) -> str:
    if chapter == "导论":
        return "intro"
    match = re.match(r"第([一二三四五六七八九十]+)章", chapter)
    if not match:
        return chapter
    return f"ch{CN_NUM[match.group(1)]:02d}"


def detect_type(line: str) -> str | None:
    if "单项选择题" in line:
        return "single"
    if "多项选择题" in line:
        return "multi"
    if "判断题" in line:
        return "judge"
    return None


def split_question_and_answer_text(text: str) -> tuple[str, str]:
    body = without_front_matter(text)
    matches = list(re.finditer(r"第[一二三四五六七八九十]+章\s*\n单选：|导论\s*\n单选：", body))
    if not matches:
        return body, ""
    idx = matches[0].start()
    return body[:idx], body[idx:]


def split_embedded_question_starts(line: str) -> list[str]:
    starts = [m.start() for m in re.finditer(r"(?<![A-Za-z0-9])\d+[.、]\s*", line)]
    starts = [idx for idx in starts if idx == 0 or line[idx - 1].isspace()]
    if len(starts) <= 1:
        return [line]
    parts: list[str] = []
    prefix = line[: starts[0]].strip()
    if prefix:
        parts.append(prefix)
    for pos, start in enumerate(starts):
        end = starts[pos + 1] if pos + 1 < len(starts) else len(line)
        part = line[start:end].strip()
        if part:
            parts.append(part)
    return parts


def parse_raw_questions(text: str) -> list[RawQuestion]:
    questions: list[RawQuestion] = []
    chapter = ""
    q_type: str | None = None
    current_no: int | None = None
    current_parts: list[str] = []

    def flush() -> None:
        nonlocal current_no, current_parts
        if chapter and q_type and current_no is not None and current_parts:
            questions.append(RawQuestion(chapter, q_type, current_no, " ".join(current_parts)))
        current_no = None
        current_parts = []

    expanded: list[str] = []
    for line in clean_lines(text):
        expanded.extend(split_embedded_question_starts(line))

    for line in expanded:
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
            current_parts = [match.group(2)]
        elif current_no is not None:
            current_parts.append(line)
    flush()
    return questions


def parse_choice_ranges(text: str) -> dict[int, str]:
    answers: dict[int, str] = {}
    pattern = re.compile(r"(\d+)-(\d+)|(\d+)\s*(?=[A-D])")
    matches = list(pattern.finditer(text))
    if not matches:
        letters = re.sub(r"[^A-D]", "", text.upper())
        return {idx: letter for idx, letter in enumerate(letters, start=1)}
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
    if not matches:
        tokens = re.findall(r"[A-D]+", text.upper())
        return {idx: token for idx, token in enumerate(tokens, start=1)}
    for idx, match in enumerate(matches):
        start = int(match.group(1) or match.group(3))
        end = int(match.group(2) or match.group(3))
        body_start = match.end()
        body_end = matches[idx + 1].start() if idx + 1 < len(matches) else len(text)
        tokens = re.findall(r"[A-D]+", text[body_start:body_end].upper())
        expected = end - start + 1
        while len(tokens) < expected:
            split_idx = next((i for i, token in enumerate(tokens) if len(token) > 4), None)
            if split_idx is None:
                break
            token = tokens.pop(split_idx)
            cut = max(1, min(4, len(token) // 2))
            tokens[split_idx:split_idx] = [token[:cut], token[cut:]]
        for offset, number in enumerate(range(start, end + 1)):
            if offset < len(tokens):
                answers[number] = tokens[offset]
    return answers


def parse_judge_ranges(text: str) -> dict[int, bool]:
    answers: dict[int, bool] = {}
    pattern = re.compile(r"(\d+)-(\d+)|(\d+)\s*(?=[对错])")
    matches = list(pattern.finditer(text))
    if not matches:
        values = re.findall(r"[对错]", text)
        return {idx: value == "对" for idx, value in enumerate(values, start=1)}
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


def parse_answers(text: str) -> dict[tuple[str, str, int], Any]:
    grouped: dict[tuple[str, str], list[str]] = defaultdict(list)
    chapter = ""
    answer_type = ""
    normalized_text = re.sub(r"\s+(第[一二三四五六七八九十]+章|导论)\s+", r"\n\1\n", text)
    normalized_text = re.sub(r"\s+(单选|多选|判断)[：:。，,]", r"\n\1:", normalized_text)
    for line in clean_lines(normalized_text):
        next_chapter = detect_chapter(line)
        if next_chapter:
            chapter = chapter_id(next_chapter)
            answer_type = ""
            continue
        line = re.sub(r"^(单选|多选|判断)[。。，,]", r"\1:", line)
        line = line.replace("：", ":", 1)
        ocr_single = re.match(r"^(单选|jt|2676|Biv|Nive)[.:：,，]\s*(.*)$", line, re.IGNORECASE)
        ocr_multi = re.match(r"^(多选|4%|%+|#6)[.:：,，]\s*(.*)$", line, re.IGNORECASE)
        ocr_judge = re.match(r"^(判断|BAZ|FART)[.:：,，]\s*(.*)$", line, re.IGNORECASE)
        if line.startswith("单选:") or ocr_single:
            answer_type = "single"
            grouped[(chapter, answer_type)].append(ocr_single.group(2) if ocr_single else line.split(":", 1)[1])
        elif line.startswith("多选:") or ocr_multi:
            answer_type = "multi"
            grouped[(chapter, answer_type)].append(ocr_multi.group(2) if ocr_multi else line.split(":", 1)[1])
        elif line.startswith("判断:") or ocr_judge:
            answer_type = "judge"
            grouped[(chapter, answer_type)].append(ocr_judge.group(2) if ocr_judge else line.split(":", 1)[1])
        elif answer_type and not line.startswith("如有其他补充"):
            grouped[(chapter, answer_type)].append(line)

    answers: dict[tuple[str, str, int], Any] = {}
    for (ch, q_type), parts in grouped.items():
        joined = " ".join(parts)
        parsed = parse_choice_ranges(joined) if q_type == "single" else parse_multi_ranges(joined) if q_type == "multi" else parse_judge_ranges(joined)
        for number, answer in parsed.items():
            answers[(ch, q_type, number)] = answer
    return answers


def split_options(raw: RawQuestion) -> tuple[str, list[str] | None]:
    text = raw.text
    if raw.q_type == "judge":
        return re.sub(r"\(\s*\)", "", text).strip(), None
    matches = list(re.finditer(r"(?<![A-Za-z])([A-D])\s*[、.．,，]\s*", text))
    if not matches:
        return text, None
    stem = re.sub(r"\(\s*\)", "", text[: matches[0].start()]).strip()
    options = []
    for idx, match in enumerate(matches):
        start = match.end()
        end = matches[idx + 1].start() if idx + 1 < len(matches) else len(text)
        option = re.sub(r"\s+", " ", text[start:end]).strip()
        if option:
            options.append(option)
    return stem, options


def answer_to_indices(answer: Any, q_type: str) -> Any:
    if q_type == "judge":
        return answer if isinstance(answer, bool) else None
    if not isinstance(answer, str):
        return None
    values = [ord(letter) - 65 for letter in answer.upper() if "A" <= letter <= "D"]
    if q_type == "single":
        return values[0] if len(values) == 1 else None
    return values


def apply_manual_answer_fixes(answers: dict[tuple[str, str, int], Any]) -> None:
    fixes = {
        ("ch02", "single"): list("ACBDADAB"),
        ("ch02", "multi"): ["ABC", "ACD", "ACD", "ABCD", "CD", "AC", "ABCD", "ABCD", "ACD", "ABCD", "ABCD"],
        ("ch02", "judge"): [False, False, True, True, True, True, True],
    }
    for (ch, q_type), values in fixes.items():
        for idx, value in enumerate(values, start=1):
            answers.setdefault((ch, q_type, idx), value)


def tokenize(text: str) -> list[str]:
    tokens = []
    for match in re.finditer(r"[\u4e00-\u9fff]+|[A-Za-z0-9]+", text):
        value = match.group(0)
        if re.fullmatch(r"[\u4e00-\u9fff]+", value):
            tokens.extend(value[i : i + 2] for i in range(max(1, len(value) - 1)))
        else:
            tokens.append(value.lower())
    return tokens


def split_lecture_sections(text: str) -> list[dict[str, Any]]:
    body = without_front_matter(text)
    sections = []
    chapter = ""
    buffer: list[str] = []

    def flush() -> None:
        if chapter and buffer:
            content = "\n".join(buffer).strip()
            if content:
                sections.append({"id": chapter_id(chapter), "chapter": chapter, "content": content})

    for line in clean_lines(body):
        next_chapter = detect_chapter(line)
        if next_chapter:
            flush()
            chapter = next_chapter
            buffer = []
            continue
        if chapter:
            buffer.append(line)
    flush()
    return sections


def build_rag_chunks(sections: list[dict[str, Any]]) -> list[dict[str, Any]]:
    chunks = []
    for section in sections:
        buffer: list[str] = []

        def flush() -> None:
            if not buffer:
                return
            text = " ".join(buffer).strip()
            if len(text) >= 20:
                chunks.append({"id": f"xigai_chunk_{len(chunks)+1:04d}", "chapter_id": section["id"], "chapter": section["chapter"], "text": text, "tokens": tokenize(text)})
            buffer.clear()

        for line in section["content"].splitlines():
            if len(" ".join(buffer)) > 700:
                flush()
            buffer.append(line)
        flush()
    return chunks


def retrieve(chunks: list[dict[str, Any]], text: str, ch_id: str, limit: int = 2) -> list[dict[str, Any]]:
    q_tokens = Counter(tokenize(text))
    scored = []
    for chunk in chunks:
        c_tokens = Counter(chunk.get("tokens") or [])
        score = float(sum(min(count, c_tokens.get(token, 0)) for token, count in q_tokens.items()))
        if chunk.get("chapter_id") == ch_id:
            score += 0.5
        if score > 0:
            scored.append((score, chunk))
    scored.sort(key=lambda item: item[0], reverse=True)
    return [chunk for _, chunk in scored[:limit]]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--questions-pdf", type=Path, required=True)
    parser.add_argument("--lecture-pdf", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("generated/xigai_2025_winter.json"))
    parser.add_argument("--rag-output", type=Path, default=Path("generated/xigai_2025_winter_rag.json"))
    parser.add_argument("--context-output", type=Path, default=Path("generated/xigai_2025_winter_full_context.txt"))
    parser.add_argument("--issues-output", type=Path, default=Path("generated/xigai_2025_winter_issues.txt"))
    parser.add_argument("--ocr-tail-pages", type=int, default=3)
    args = parser.parse_args()

    question_text = pdf_to_text(args.questions_pdf.expanduser())
    lecture_text = pdf_to_text(args.lecture_pdf.expanduser())
    question_body, answer_body = split_question_and_answer_text(question_text)
    raw_questions = parse_raw_questions(question_body)
    answers = parse_answers(answer_body)
    ocr_answers = parse_answers(ocr_tail_pages(args.questions_pdf.expanduser(), args.ocr_tail_pages))
    for key, value in ocr_answers.items():
        answers.setdefault(key, value)
    apply_manual_answer_fixes(answers)
    sections = split_lecture_sections(lecture_text)
    chunks = build_rag_chunks(sections)

    questions = []
    issues = []
    for raw in raw_questions:
        ch_id = chapter_id(raw.chapter)
        stem, options = split_options(raw)
        if raw.q_type in {"single", "multi"} and isinstance(options, list) and len(options) > 4:
            options = options[:4]
        answer = answer_to_indices(answers.get((ch_id, raw.q_type, raw.number)), raw.q_type)
        if answer is None:
            issues.append(f"missing answer: {raw.chapter} {raw.q_type} #{raw.number}")
        if raw.q_type in {"single", "multi"} and (not options or len(options) != 4):
            issues.append(f"option count issue: {raw.chapter} {raw.q_type} #{raw.number} options={len(options or [])}")
            continue
        question = {
            "id": f"xigai_{len(questions)+1:04d}",
            "number": str(raw.number),
            "type": raw.q_type,
            "chapter_id": ch_id,
            "chapter": raw.chapter,
            "content": stem,
            "options": options,
            "answer": answer,
            "analysis": "",
            "stats": {"total": 0, "correct": 0, "rate": 0},
        }
        contexts = retrieve(chunks, f"{stem} {' '.join(options or [])}", ch_id)
        if contexts:
            question["rag_refs"] = [ctx["id"] for ctx in contexts]
            question["rag_context"] = [{"id": ctx["id"], "chapter": ctx["chapter"], "text": ctx["text"][:500]} for ctx in contexts]
        questions.append(question)

    chapters = [{"id": section["id"], "name": section["chapter"]} for section in sections]
    bank = {
        "meta": {
            "name": "习近平新时代中国特色社会主义思想概论",
            "version": "2025-winter",
            "color": "#c62828",
            "total": len(questions),
            "source_files": [str(args.questions_pdf), str(args.lecture_pdf)],
            "chapters": chapters,
        },
        "questions": questions,
    }
    rag = {"meta": {"name": "习概2025冬重点RAG索引", "chunk_count": len(chunks)}, "chunks": chunks}
    context = "\n\n".join(f"## {section['chapter']}\n{section['content']}" for section in sections).strip() + "\n"

    for path in (args.output, args.rag_output, args.context_output, args.issues_output):
        path.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(sanitize_source_metadata(bank), ensure_ascii=False, indent=2), encoding="utf-8")
    args.rag_output.write_text(json.dumps(sanitize_source_metadata(rag), ensure_ascii=False, indent=2), encoding="utf-8")
    args.context_output.write_text(context, encoding="utf-8")
    args.issues_output.write_text("\n".join(issues) + ("\n" if issues else ""), encoding="utf-8")
    print(json.dumps({"questions": len(questions), "types": dict(Counter(q["type"] for q in questions)), "chapters": len(chapters), "rag_chunks": len(chunks), "issues": len(issues)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
