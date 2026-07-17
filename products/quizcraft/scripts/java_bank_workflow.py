#!/usr/bin/env python3
from __future__ import annotations

import copy
import json
import re
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from scripts.build_java_bank_from_md import parse_markdown


DEFAULT_JAVA_BANK_KEY = "java_programming"
DEFAULT_JAVA_BANK_NAME = "Java程序设计题库"
DEFAULT_JAVA_BANK_COLOR = "#f57c00"
DEFAULT_JAVA_START_NUMBER = 149
DEFAULT_DEEPSEEK_CHAT_COMPLETIONS_URL = "https://api.deepseek.com/chat/completions"


@dataclass
class IncrementalBankResult:
    bank: dict[str, Any]
    added_questions: list[dict[str, Any]]
    skipped_duplicate_count: int
    skipped_before_start_count: int

    @property
    def added_count(self) -> int:
        return len(self.added_questions)


def _question_number(question: dict[str, Any]) -> int | None:
    raw = question.get("number")
    try:
        return int(str(raw).strip())
    except (TypeError, ValueError):
        return None


def _compact_text(value: Any) -> str:
    text = str(value or "").casefold()
    return re.sub(r"\s+", "", text)


def fingerprint_question(question: dict[str, Any]) -> str:
    content = _compact_text(question.get("content"))
    options = question.get("options") or []
    option_text = "|".join(_compact_text(option) for option in options)
    return f"{content}::{option_text}"


def parse_java_markdown_bank(path: Path, start_number: int | None = None) -> dict[str, Any]:
    bank = parse_markdown(path)
    if start_number is not None:
        bank["questions"] = [
            question
            for question in bank["questions"]
            if (_question_number(question) or 0) >= int(start_number)
        ]
        bank["meta"]["total"] = len(bank["questions"])
    return bank


def _next_question_number(existing_questions: list[dict[str, Any]]) -> int:
    numbers = [_question_number(question) or 0 for question in existing_questions]
    return max(numbers or [0]) + 1


def _next_id_number(existing_questions: list[dict[str, Any]], id_prefix: str) -> int:
    pattern = re.compile(rf"^{re.escape(id_prefix)}_(\d+)$")
    values: list[int] = []
    for question in existing_questions:
        match = pattern.match(str(question.get("id") or ""))
        if match:
            values.append(int(match.group(1)))
    return max(values or [0]) + 1


def _normalize_incoming_question(
    question: dict[str, Any],
    *,
    id_prefix: str,
    used_ids: set[str],
    fallback_number: int,
    fallback_id_number: int,
) -> tuple[dict[str, Any], int, int]:
    normalized = copy.deepcopy(question)
    number = _question_number(normalized) or fallback_number
    qid = f"{id_prefix}_{number:04d}"
    id_number = fallback_id_number
    if qid in used_ids:
        while f"{id_prefix}_{id_number:04d}" in used_ids:
            id_number += 1
        qid = f"{id_prefix}_{id_number:04d}"
        id_number += 1

    normalized["id"] = qid
    normalized["number"] = str(number)
    normalized.setdefault("type", "single")
    normalized.setdefault("chapter_id", "ch01")
    normalized.setdefault("chapter", "Java题库")
    normalized.setdefault("stats", {"total": 0, "correct": 0, "rate": 0})
    return normalized, max(fallback_number, number) + 1, id_number


def build_incremental_bank(
    existing_bank: dict[str, Any],
    incoming_bank: dict[str, Any],
    *,
    start_number: int = DEFAULT_JAVA_START_NUMBER,
    id_prefix: str = "java",
) -> IncrementalBankResult:
    bank = copy.deepcopy(existing_bank)
    meta = bank.setdefault("meta", {})
    meta.setdefault("name", DEFAULT_JAVA_BANK_NAME)
    meta.setdefault("color", DEFAULT_JAVA_BANK_COLOR)
    questions = bank.setdefault("questions", [])

    existing_fingerprints = {
        fingerprint_question(question)
        for question in questions
        if question.get("content")
    }
    used_ids = {str(question.get("id")) for question in questions if question.get("id")}
    fallback_number = _next_question_number(questions)
    fallback_id_number = _next_id_number(questions, id_prefix)
    added: list[dict[str, Any]] = []
    skipped_duplicate = 0
    skipped_before_start = 0

    candidates = sorted(
        incoming_bank.get("questions", []),
        key=lambda question: (_question_number(question) or 0),
    )
    for question in candidates:
        number = _question_number(question)
        if number is not None and number < start_number:
            skipped_before_start += 1
            continue
        fingerprint = fingerprint_question(question)
        if fingerprint in existing_fingerprints:
            skipped_duplicate += 1
            continue
        normalized, fallback_number, fallback_id_number = _normalize_incoming_question(
            question,
            id_prefix=id_prefix,
            used_ids=used_ids,
            fallback_number=fallback_number,
            fallback_id_number=fallback_id_number,
        )
        questions.append(normalized)
        added.append(normalized)
        existing_fingerprints.add(fingerprint)
        used_ids.add(str(normalized["id"]))

    meta["total"] = len(questions)
    meta.setdefault("chapters", [{"id": "ch01", "name": "Java题库"}])
    return IncrementalBankResult(
        bank=bank,
        added_questions=added,
        skipped_duplicate_count=skipped_duplicate,
        skipped_before_start_count=skipped_before_start,
    )


def build_save_payload(
    bank: dict[str, Any],
    *,
    key: str = DEFAULT_JAVA_BANK_KEY,
    name: str | None = None,
    color: str | None = None,
    overwrite: bool = True,
) -> dict[str, Any]:
    meta = bank.get("meta") if isinstance(bank.get("meta"), dict) else {}
    return {
        "key": key,
        "name": name or meta.get("name") or DEFAULT_JAVA_BANK_NAME,
        "color": color or meta.get("color") or DEFAULT_JAVA_BANK_COLOR,
        "questions": bank.get("questions", []),
        "overwrite": overwrite,
    }


def read_bank(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def write_bank(path: Path, bank: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(bank, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def build_java_answer_prompt(question: dict[str, Any]) -> str:
    options = "\n".join(
        f"{chr(65 + index)}. {option}"
        for index, option in enumerate(question.get("options") or [])
    )
    return f"""请为下面这道 Java 程序设计单选题判定正确答案并生成解析。

要求：
1. 只输出 JSON，不要 Markdown。
2. answer 输出 0-3 的整数，分别代表 A-D。
3. analysis 用中文，50-120 字，说明为什么该项正确，必要时指出其他选项错误点。
4. 如两个选项文本完全相同且都正确，优先选择靠前的选项。

题目：{question.get("content", "")}
选项：
{options}
"""


def normalize_java_answer(raw: Any) -> int | None:
    if isinstance(raw, int) and 0 <= raw <= 3:
        return raw
    if isinstance(raw, str):
        text = raw.strip().upper()
        if text[:1] in "ABCD":
            return ord(text[0]) - 65
        if text.isdigit() and 0 <= int(text) <= 3:
            return int(text)
    return None


def _chat_completions_url(api_url: str | None) -> str:
    if not api_url:
        return DEFAULT_DEEPSEEK_CHAT_COMPLETIONS_URL
    clean = api_url.rstrip("/")
    if clean.endswith("/chat/completions"):
        return clean
    return clean + "/chat/completions"


def _post_deepseek_json(
    *,
    api_key: str,
    api_url: str | None,
    model: str,
    question: dict[str, Any],
    timeout: int,
) -> dict[str, Any]:
    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": "你是严谨的 Java 课程题库答案校验与解析助手。"},
            {"role": "user", "content": build_java_answer_prompt(question)},
        ],
        "temperature": 0,
        "response_format": {"type": "json_object"},
    }
    request = urllib.request.Request(
        _chat_completions_url(api_url),
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {api_key}"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        data = json.loads(response.read().decode("utf-8"))
    content = data["choices"][0]["message"]["content"]
    return json.loads(content)


def fill_java_questions_with_deepseek(
    questions: list[dict[str, Any]],
    *,
    api_key: str,
    api_url: str | None = None,
    model: str = "deepseek-chat",
    timeout: int = 120,
) -> list[dict[str, Any]]:
    for question in questions:
        if question.get("answer") is not None and str(question.get("analysis") or "").strip():
            continue
        result = _post_deepseek_json(
            api_key=api_key,
            api_url=api_url,
            model=model,
            question=question,
            timeout=timeout,
        )
        answer = normalize_java_answer(result.get("answer"))
        if answer is None:
            raise ValueError(f"invalid answer for {question.get('id')}: {result!r}")
        analysis = str(result.get("analysis") or result.get("reason") or "").strip()
        if not analysis:
            raise ValueError(f"missing analysis for {question.get('id')}: {result!r}")
        question["answer"] = answer
        question["analysis"] = analysis
    return questions
