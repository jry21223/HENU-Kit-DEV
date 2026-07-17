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

CHAPTER_ORDER = [
    ("intro", "绪论 担当复兴大任 成就时代新人"),
    ("ch01", "第一章 领悟人生真谛 把握人生方向"),
    ("ch02", "第二章 追求远大理想 坚定崇高信念"),
    ("ch03", "第三章 继承优良传统 弘扬中国精神"),
    ("ch04", "第四章 明确价值要求 践行价值准则"),
    ("ch05", "第五章 遵守道德规范 锤炼道德品格"),
    ("ch06", "第六章 学习法治思想 提升法治素养"),
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


def ocr_tail_pages(path: Path, pages: int) -> str:
    if pages <= 0 or not shutil.which("pdftoppm") or not shutil.which("tesseract"):
        return ""
    info = subprocess.run(
        ["pdfinfo", str(path)],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    match = re.search(r"^Pages:\s+(\d+)", info.stdout, re.MULTILINE)
    if not match:
        return ""
    total_pages = int(match.group(1))
    first_page = max(1, total_pages - pages + 1)
    with tempfile.TemporaryDirectory(prefix="sixiu_ocr_") as tmp:
        prefix = Path(tmp) / "page"
        subprocess.run(
            ["pdftoppm", "-r", "220", "-png", "-f", str(first_page), "-l", str(total_pages), str(path), str(prefix)],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        outputs: list[str] = []
        for image_path in sorted(Path(tmp).glob("*.png")):
            out_prefix = image_path.with_suffix("")
            subprocess.run(
                ["tesseract", str(image_path), str(out_prefix), "-l", "chi_sim+eng", "--psm", "6"],
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            txt = out_prefix.with_suffix(".txt")
            if txt.exists():
                outputs.append(txt.read_text(encoding="utf-8", errors="replace"))
        return "\n".join(outputs)


def normalize_text(text: str) -> str:
    text = text.replace("（", "(").replace("）", ")")
    text = text.replace("．", ".").replace("。", "。")
    text = text.replace("、", "、")
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def clean_lines(text: str) -> list[str]:
    lines: list[str] = []
    for raw in text.replace("\x0c", "\n").splitlines():
        line = normalize_text(raw.strip())
        if any(pattern.search(line) for pattern in CLEAN_PATTERNS):
            continue
        lines.append(line)
    return lines


def split_embedded_question_starts(line: str) -> list[str]:
    starts = [match.start() for match in re.finditer(r"(?<![A-Za-z0-9])\d+[.、]\s*", line)]
    starts = [idx for idx in starts if idx == 0 or line[idx - 1].isspace()]
    if len(starts) <= 1:
        return [line]
    parts: list[str] = []
    for pos, start in enumerate(starts):
        end = starts[pos + 1] if pos + 1 < len(starts) else len(line)
        part = line[start:end].strip()
        if part:
            parts.append(part)
    prefix = line[: starts[0]].strip()
    return ([prefix] if prefix else []) + parts


def chapter_key(chapter: str) -> str:
    if "绪论" in chapter and "第一章" in chapter:
        return "ch01"
    if chapter.startswith("绪论"):
        return "intro"
    match = re.search(r"第([一二三四五六])章", chapter)
    if not match:
        return chapter
    return {
        "一": "ch01",
        "二": "ch02",
        "三": "ch03",
        "四": "ch04",
        "五": "ch05",
        "六": "ch06",
    }[match.group(1)]


def detect_chapter(line: str) -> str | None:
    if "答案速查" in line or line == "目录":
        return None
    if re.match(r"^绪论\+第一章\b", line):
        return line
    if re.match(r"^绪论\s+担当复兴大任", line):
        return "绪论 担当复兴大任 成就时代新人"
    match = re.match(r"^(第[一二三四五六]章\s+.+)$", line)
    if match:
        return match.group(1)
    return None


def detect_type(line: str) -> str | None:
    if "单项选择题" in line:
        return "single"
    if "多项选择题" in line:
        return "multi"
    if "判断题" in line:
        return "judge"
    return None


def without_front_matter(text: str, skip_pages: int = 2) -> str:
    pages = text.split("\x0c")
    return "\n".join(pages[skip_pages:]) if len(pages) > skip_pages else text


def split_question_and_answer_text(text: str) -> tuple[str, str]:
    body = without_front_matter(text)
    idx = body.rfind("答案速查")
    if idx == -1:
        return body, ""
    return body[:idx], body[idx:]


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

    expanded_lines: list[str] = []
    for line in clean_lines(text):
        expanded_lines.extend(split_embedded_question_starts(line))

    for line in expanded_lines:
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


def answer_chapter_key(line: str) -> str | None:
    return chapter_key(line) if detect_chapter(line) else None


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
    for line in clean_lines(text):
        key = answer_chapter_key(line)
        if key:
            chapter = key
            answer_type = ""
            continue
        line = line.replace("，", "：", 1) if re.match(r"^(单选|多选|判断)，", line) else line
        line = line.replace(".", "：", 1) if re.match(r"^(单选|多选|判断)\.", line) else line
        line = line.replace(":", "：", 1) if re.match(r"^(单选|多选|判断):", line) else line
        ocr_single = re.match(r"^(Nive|Ne|Nivee|单选)[：:，,]\s*(.*)$", line)
        ocr_multi = re.match(r"^(%+\s*ii|[#%0-9]+\s*%?|多选)[：:，,]\s*(.*)$", line)
        ocr_judge = re.match(r"^(FART|判断)[：:，,]\s*(.*)$", line)
        if line.startswith("单选：") or ocr_single:
            answer_type = "single"
            grouped[(chapter, answer_type)].append(ocr_single.group(2) if ocr_single else line.split("：", 1)[1])
        elif line.startswith("多选：") or ocr_multi:
            answer_type = "multi"
            grouped[(chapter, answer_type)].append(ocr_multi.group(2) if ocr_multi else line.split("：", 1)[1])
        elif line.startswith("判断：") or ocr_judge:
            answer_type = "judge"
            grouped[(chapter, answer_type)].append(ocr_judge.group(2) if ocr_judge else line.split("：", 1)[1])
        elif answer_type and not line.startswith("如有其他补充"):
            grouped[(chapter, answer_type)].append(line)

    answers: dict[tuple[str, str, int], Any] = {}
    for (ch, q_type), parts in grouped.items():
        joined = " ".join(parts)
        if q_type == "single":
            parsed = parse_choice_ranges(joined)
        elif q_type == "multi":
            parsed = parse_multi_ranges(joined)
        else:
            parsed = parse_judge_ranges(joined)
        for number, answer in parsed.items():
            answers[(ch, q_type, number)] = answer
    return answers


def split_options(raw: RawQuestion) -> tuple[str, list[str] | None]:
    text = raw.text
    if raw.q_type == "judge":
        return re.sub(r"\(\s*\)|（）", "", text).strip(), None
    text = text.translate(str.maketrans({"Ａ": "A", "Ｂ": "B", "Ｃ": "C", "Ｄ": "D"}))
    matches = list(re.finditer(r"(?<![A-Za-z])([A-D])\s*[、.．,，]\s*", text))
    if not matches:
        return text, None
    stem = re.sub(r"\(\s*\)", "", text[: matches[0].start()]).strip()
    options: list[str] = []
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


def tokenize(text: str) -> list[str]:
    tokens: list[str] = []
    for match in re.finditer(r"[\u4e00-\u9fff]+|[A-Za-z0-9]+", text):
        value = match.group(0)
        if re.fullmatch(r"[\u4e00-\u9fff]+", value):
            tokens.extend(value[idx : idx + 2] for idx in range(max(1, len(value) - 1)))
        else:
            tokens.append(value.lower())
    return tokens


def split_lecture_sections(text: str) -> list[dict[str, Any]]:
    body = without_front_matter(text)
    sections: list[dict[str, Any]] = []
    current_chapter = ""
    current_lines: list[str] = []

    def flush() -> None:
        if current_chapter and current_lines:
            content = "\n".join(current_lines).strip()
            if content:
                sections.append(
                    {
                        "id": chapter_key(current_chapter),
                        "chapter": current_chapter,
                        "content": content,
                    }
                )

    for line in clean_lines(body):
        next_chapter = detect_chapter(line)
        if next_chapter:
            flush()
            current_chapter = next_chapter
            current_lines = []
            continue
        if current_chapter:
            current_lines.append(line)
    flush()
    return sections


def build_rag_chunks(sections: list[dict[str, Any]], season_key: str) -> list[dict[str, Any]]:
    chunks: list[dict[str, Any]] = []
    for section in sections:
        buffer: list[str] = []

        def flush() -> None:
            if not buffer:
                return
            body = " ".join(buffer).strip()
            if len(body) >= 20:
                chunks.append(
                    {
                        "id": f"{season_key}_chunk_{len(chunks) + 1:04d}",
                        "chapter_id": section["id"],
                        "chapter": section["chapter"],
                        "text": body,
                        "tokens": tokenize(body),
                    }
                )
            buffer.clear()

        for line in section["content"].splitlines():
            if len(" ".join(buffer)) > 650:
                flush()
            buffer.append(line)
        flush()
    return chunks


def retrieve(chunks: list[dict[str, Any]], question_text: str, ch_key: str, limit: int = 2) -> list[dict[str, Any]]:
    q_tokens = Counter(tokenize(question_text))
    scored: list[tuple[float, dict[str, Any]]] = []
    for chunk in chunks:
        c_tokens = Counter(chunk.get("tokens") or [])
        overlap = sum(min(count, c_tokens.get(token, 0)) for token, count in q_tokens.items())
        score = float(overlap)
        if ch_key == chunk.get("chapter_id"):
            score += 0.5
        if score > 0:
            scored.append((score, chunk))
    scored.sort(key=lambda item: item[0], reverse=True)
    return [chunk for _, chunk in scored[:limit]]


def build_season(season_key: str, display_name: str, question_pdf: Path, lecture_pdf: Path, ocr_tail_pages_count: int) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any], str, str, list[str]]:
    question_text = pdf_to_text(question_pdf)
    lecture_text = pdf_to_text(lecture_pdf)
    question_body, answer_body = split_question_and_answer_text(question_text)
    answer_body = "\n".join(part for part in [answer_body, ocr_tail_pages(question_pdf, ocr_tail_pages_count)] if part)
    raw_questions = parse_raw_questions(question_body)
    answers = parse_answers(answer_body)
    sections = split_lecture_sections(lecture_text)
    chunks = build_rag_chunks(sections, season_key)
    issues: list[str] = []
    questions: list[dict[str, Any]] = []

    for raw in raw_questions:
        ch_key = chapter_key(raw.chapter)
        answer = answers.get((ch_key, raw.q_type, raw.number))
        stem, options = split_options(raw)
        normalized_answer = answer_to_indices(answer, raw.q_type)
        if normalized_answer is None:
            issues.append(f"missing/invalid answer: {raw.chapter} {raw.q_type} #{raw.number}")
        if raw.q_type in {"single", "multi"} and (not options or len(options) != 4):
            issues.append(f"option count issue: {raw.chapter} {raw.q_type} #{raw.number} options={len(options or [])}")

        item = {
            "id": f"sixiu_{season_key}_{len(questions) + 1:04d}",
            "number": str(raw.number),
            "type": raw.q_type,
            "chapter_id": ch_key,
            "chapter": raw.chapter,
            "content": stem,
            "options": options,
            "answer": normalized_answer,
            "analysis": "",
            "source": str(question_pdf),
            "stats": {"total": 0, "correct": 0, "rate": 0},
        }
        contexts = retrieve(chunks, f"{stem} {' '.join(options or [])}", ch_key)
        if contexts:
            item["rag_refs"] = [ctx["id"] for ctx in contexts]
            item["rag_context"] = [
                {"id": ctx["id"], "chapter": ctx["chapter"], "text": ctx["text"][:500]}
                for ctx in contexts
            ]
        questions.append(item)

    bank = {
        "meta": {
            "name": display_name,
            "subject": "思想道德与法治",
            "season": season_key,
            "color": "#2e7d32",
            "total": len(questions),
            "source_files": [str(question_pdf), str(lecture_pdf)],
            "chapters": CHAPTER_ORDER,
        },
        "questions": questions,
    }
    rag = {
        "meta": {
            "name": f"{display_name}重点RAG索引",
            "subject": "思想道德与法治",
            "season": season_key,
            "source_file": str(lecture_pdf),
            "chunk_count": len(chunks),
        },
        "chunks": chunks,
    }
    materials = {
        "meta": {
            "name": f"{display_name}资料库",
            "subject": "思想道德与法治",
            "season": season_key,
            "source_files": [str(question_pdf), str(lecture_pdf)],
        },
        "sections": sections,
    }
    return bank, rag, materials, question_text, lecture_text, issues


def question_signature(question: dict[str, Any]) -> tuple[str, str, tuple[str, ...]]:
    content = re.sub(r"\s+", "", str(question.get("content") or ""))
    options = tuple(re.sub(r"\s+", "", str(option)) for option in (question.get("options") or []))
    return str(question.get("type") or ""), content, options


def fill_missing_answers_from_peer(target: dict[str, Any], peer: dict[str, Any]) -> int:
    answer_by_signature: dict[tuple[str, str, tuple[str, ...]], Any] = {}
    for question in peer.get("questions") or []:
        if question.get("answer") is not None:
            answer_by_signature[question_signature(question)] = question.get("answer")

    filled = 0
    for question in target.get("questions") or []:
        if question.get("answer") is None:
            answer = answer_by_signature.get(question_signature(question))
            if answer is not None:
                question["answer"] = answer
                question["answer_source"] = "matched_from_peer_season"
                filled += 1
    return filled


def validate_bank_questions(bank: dict[str, Any]) -> list[str]:
    issues: list[str] = []
    for question in bank.get("questions") or []:
        qid = question["id"]
        q_type = question.get("type")
        options = question.get("options")
        answer = question.get("answer")
        if answer is None:
            issues.append(f"{qid}: missing answer: {question['chapter']} {q_type} #{question['number']}")
        if q_type in {"single", "multi"} and (not options or len(options) != 4):
            issues.append(f"{qid}: option count issue: {question['chapter']} {q_type} #{question['number']} options={len(options or [])}")
    return issues


def write_markdown(path: Path, title: str, materials: dict[str, Any], bank: dict[str, Any]) -> None:
    lines = [f"# {title}", ""]
    lines.append("## 章节重点")
    for section in materials["sections"]:
        lines.extend(["", f"### {section['chapter']}", "", section["content"]])
    lines.extend(["", "## 习题索引"])
    grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for question in bank["questions"]:
        grouped[question["chapter"]].append(question)
    type_names = {"single": "单选", "multi": "多选", "judge": "判断"}
    for chapter, questions in grouped.items():
        counts = Counter(q["type"] for q in questions)
        lines.extend([
            "",
            f"### {chapter}",
            "",
            "，".join(f"{type_names.get(k, k)} {v} 题" for k, v in counts.items()),
        ])
    path.write_text("\n".join(lines).strip() + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Build categorized Sixiu resource library from PDF pairs.")
    parser.add_argument("--output-dir", type=Path, default=Path("generated/sixiu_resource_library"))
    parser.add_argument("--winter-question", type=Path, required=True)
    parser.add_argument("--winter-lecture", type=Path, required=True)
    parser.add_argument("--summer-question", type=Path, required=True)
    parser.add_argument("--summer-lecture", type=Path, required=True)
    parser.add_argument("--ocr-tail-pages", type=int, default=3, help="OCR this many tail pages from each question PDF and merge into answer parsing.")
    args = parser.parse_args()

    args.output_dir.mkdir(parents=True, exist_ok=True)
    seasons = [
        ("2025_winter", "思修2025冬", args.winter_question.expanduser(), args.winter_lecture.expanduser()),
        ("2026_summer", "思修2026夏", args.summer_question.expanduser(), args.summer_lecture.expanduser()),
    ]
    index: dict[str, Any] = {"subject": "思想道德与法治", "seasons": []}

    built: list[tuple[str, str, Path, dict[str, Any], dict[str, Any], dict[str, Any], str, str, list[str]]] = []
    for season_key, display_name, question_pdf, lecture_pdf in seasons:
        bank, rag, materials, question_text, lecture_text, issues = build_season(season_key, display_name, question_pdf, lecture_pdf, args.ocr_tail_pages)
        built.append((season_key, display_name, args.output_dir / season_key, bank, rag, materials, question_text, lecture_text, issues))

    if len(built) >= 2:
        fill_missing_answers_from_peer(built[1][3], built[0][3])
        fill_missing_answers_from_peer(built[0][3], built[1][3])

    for season_key, display_name, season_dir, bank, rag, materials, question_text, lecture_text, original_issues in built:
        season_dir = args.output_dir / season_key
        season_dir.mkdir(parents=True, exist_ok=True)
        issues = validate_bank_questions(bank)

        (season_dir / "question_bank.json").write_text(json.dumps(sanitize_source_metadata(bank), ensure_ascii=False, indent=2), encoding="utf-8")
        (season_dir / "lecture_rag.json").write_text(json.dumps(sanitize_source_metadata(rag), ensure_ascii=False, indent=2), encoding="utf-8")
        (season_dir / "materials.json").write_text(json.dumps(sanitize_source_metadata(materials), ensure_ascii=False, indent=2), encoding="utf-8")
        (season_dir / "raw_questions.txt").write_text(question_text, encoding="utf-8")
        (season_dir / "raw_lecture.txt").write_text(lecture_text, encoding="utf-8")
        (season_dir / "issues.txt").write_text("\n".join(issues) + ("\n" if issues else ""), encoding="utf-8")
        write_markdown(season_dir / "README.md", f"{display_name}资料库", materials, bank)

        counts = Counter(q["type"] for q in bank["questions"])
        index["seasons"].append(
            {
                "season": season_key,
                "name": display_name,
                "question_count": len(bank["questions"]),
                "type_counts": dict(counts),
                "lecture_sections": len(materials["sections"]),
                "rag_chunks": len(rag["chunks"]),
                "issues": len(issues),
                "path": str(season_dir),
            }
        )
        print(json.dumps(index["seasons"][-1], ensure_ascii=False))

    (args.output_dir / "index.json").write_text(json.dumps(index, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
