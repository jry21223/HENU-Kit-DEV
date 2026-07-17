#!/usr/bin/env python3
from __future__ import annotations

import json
import re
from collections import Counter
from pathlib import Path
from typing import Any

try:
    from scripts.bank_metadata import sanitize_source_metadata
except ModuleNotFoundError:
    from bank_metadata import sanitize_source_metadata

from build_sixiu_resource_library import retrieve


BASE = Path("generated/sixiu_resource_library")
OUT_BANK = Path("generated/sixiu_merged.json")
OUT_RAG = Path("generated/sixiu_merged_rag.json")
OUT_CONTEXT = Path("generated/sixiu_merged_full_context.txt")
OUT_ISSUES = Path("generated/sixiu_merged_issues.txt")


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def normalize(text: Any) -> str:
    return re.sub(r"\s+", "", str(text or ""))


def signature(question: dict[str, Any]) -> tuple[str, str, tuple[str, ...]]:
    options = question.get("options") or []
    return (
        str(question.get("type") or ""),
        normalize(question.get("content")),
        tuple(normalize(option) for option in options[:4]),
    )


def sanitize_question(question: dict[str, Any], new_id: str, source_season: str) -> tuple[dict[str, Any], list[str]]:
    cleaned = dict(question)
    issues: list[str] = []
    cleaned["id"] = new_id
    cleaned["source_season"] = source_season
    cleaned["analysis"] = ""
    cleaned["stats"] = {"total": 0, "correct": 0, "rate": 0}

    q_type = cleaned.get("type")
    options = cleaned.get("options")
    answer = cleaned.get("answer")
    if q_type in {"single", "multi"}:
        if not options:
            content = str(cleaned.get("content") or "")
            matches = list(re.finditer(r"(?<![A-Za-z])([A-D])\s*[、.．,，]?\s*", content))
            if len(matches) >= 4:
                cleaned["content"] = content[: matches[0].start()].strip()
                extracted: list[str] = []
                for idx, match in enumerate(matches[:4]):
                    start = match.end()
                    end = matches[idx + 1].start() if idx + 1 < 4 else len(content)
                    extracted.append(re.sub(r"\s+", " ", content[start:end]).strip())
                cleaned["options"] = extracted
                options = extracted
        if isinstance(options, list) and len(options) > 4:
            cleaned["options"] = options[:4]
            issues.append(f"{new_id}: truncated options {len(options)} -> 4 from {source_season}")
        elif not isinstance(options, list) or len(options) != 4:
            issues.append(f"{new_id}: option count issue {len(options or [])} from {source_season}")
        if q_type == "single" and not isinstance(answer, int):
            issues.append(f"{new_id}: invalid single answer {answer!r}")
        if q_type == "multi" and not isinstance(answer, list):
            issues.append(f"{new_id}: invalid multi answer {answer!r}")
    elif q_type == "judge":
        cleaned["options"] = None
        if not isinstance(answer, bool):
            issues.append(f"{new_id}: invalid judge answer {answer!r}")
    else:
        issues.append(f"{new_id}: unknown type {q_type!r}")

    for key in ("rag_refs", "rag_context", "answer_source"):
        cleaned.pop(key, None)
    return cleaned, issues


def main() -> int:
    summer = load_json(BASE / "2026_summer" / "question_bank.json")
    winter = load_json(BASE / "2025_winter" / "question_bank.json")
    summer_rag = load_json(BASE / "2026_summer" / "lecture_rag.json")
    winter_rag = load_json(BASE / "2025_winter" / "lecture_rag.json")
    summer_materials = load_json(BASE / "2026_summer" / "materials.json")
    winter_materials = load_json(BASE / "2025_winter" / "materials.json")

    merged: list[dict[str, Any]] = []
    seen: set[tuple[str, str, tuple[str, ...]]] = set()
    issues: list[str] = []

    for source_season, source in (("2026_summer", summer), ("2025_winter", winter)):
        for question in source["questions"]:
            sig = signature(question)
            if sig in seen:
                continue
            seen.add(sig)
            new_id = f"sixiu_{len(merged) + 1:04d}"
            cleaned, q_issues = sanitize_question(question, new_id, source_season)
            issues.extend(q_issues)
            merged.append(cleaned)

    rag_chunks = []
    for source, prefix in ((summer_rag, "summer"), (winter_rag, "winter")):
        for chunk in source.get("chunks") or []:
            copied = dict(chunk)
            copied["id"] = f"sixiu_{prefix}_{copied['id']}"
            copied["source_season"] = prefix
            rag_chunks.append(copied)

    for question in merged:
        contexts = retrieve(
            rag_chunks,
            f"{question.get('content', '')} {' '.join(question.get('options') or [])}",
            str(question.get("chapter_id") or ""),
            limit=2,
        )
        if contexts:
            question["rag_refs"] = [ctx["id"] for ctx in contexts]
            question["rag_context"] = [
                {"id": ctx["id"], "chapter": ctx["chapter"], "text": ctx["text"][:500]}
                for ctx in contexts
            ]

    chapters = summer["meta"].get("chapters") or winter["meta"].get("chapters") or []
    bank = {
        "meta": {
            "name": "思想道德与法治",
            "version": "merged-2025-winter-2026-summer",
            "color": "#2e7d32",
            "total": len(merged),
            "source_files": [
                *summer["meta"].get("source_files", []),
                *winter["meta"].get("source_files", []),
            ],
            "chapters": chapters,
        },
        "questions": merged,
    }
    rag = {
        "meta": {
            "name": "思修合并重点RAG索引",
            "subject": "思想道德与法治",
            "chunk_count": len(rag_chunks),
            "source_seasons": ["2026_summer", "2025_winter"],
        },
        "chunks": rag_chunks,
    }
    context_parts = []
    for title, materials in (("思修2026夏重点", summer_materials), ("思修2025冬重点", winter_materials)):
        context_parts.append(f"# {title}")
        for section in materials.get("sections") or []:
            context_parts.append(f"## {section['chapter']}\n{section['content']}")

    OUT_BANK.write_text(json.dumps(sanitize_source_metadata(bank), ensure_ascii=False, indent=2), encoding="utf-8")
    OUT_RAG.write_text(json.dumps(sanitize_source_metadata(rag), ensure_ascii=False, indent=2), encoding="utf-8")
    OUT_CONTEXT.write_text("\n\n".join(context_parts).strip() + "\n", encoding="utf-8")
    OUT_ISSUES.write_text("\n".join(issues) + ("\n" if issues else ""), encoding="utf-8")

    print(json.dumps({
        "questions": len(merged),
        "types": dict(Counter(q["type"] for q in merged)),
        "rag_chunks": len(rag_chunks),
        "issues": len(issues),
        "output": str(OUT_BANK),
        "context": str(OUT_CONTEXT),
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
