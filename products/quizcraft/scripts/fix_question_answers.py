#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Batch-correct question answers in PostgreSQL-backed question banks."""

import argparse
import csv
import json
import pathlib
import re
import sys
from typing import Any, List, Tuple

ROOT = pathlib.Path(__file__).resolve().parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

import db_storage  # noqa: E402


def parse_answer(raw: str) -> Any:
    """
    Parse answer text to python value.

    Accepted formats:
    - JSON value (numbers, booleans, arrays, strings)
    - letter form (A/B/C/D/E...), including comma-separated multi-answer list
    - numeric form (single index)
    - comma-separated numeric list for multi-answer
    """
    text = (raw or "").strip()
    if not text:
        raise ValueError("empty answer")

    lower = text.lower()
    if lower in {"true", "false"}:
        return lower == "true"

    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass

    # A/B/C/D/E ... or A, C, D ...
    tokens = [t.strip() for t in re.split(r"[,/|\\s]+", text) if t.strip()]
    if tokens and all(re.fullmatch(r"[A-Za-z]", t) for t in tokens):
        idx_map = {ch: i for i, ch in enumerate("ABCDE")}
        indices = [idx_map[t.upper()] for t in tokens if t.upper() in idx_map]
        if len(indices) != len(tokens):
            raise ValueError(f"unsupported letter option in {text!r}")
        return indices[0] if len(indices) == 1 else indices

    # comma-separated indexes
    if "," in text and all(part.strip().isdigit() for part in tokens):
        values = [int(part.strip()) for part in tokens]
        return values[0] if len(values) == 1 else values

    if text.isdigit() or (text.startswith("-") and text[1:].isdigit()):
        return int(text)

    if text.startswith("{") or text.startswith("["):
        raise ValueError(f"invalid json answer: {text!r}")

    return text


def parse_item(item: str) -> Tuple[str, str, Any]:
    raw = (item or "").strip()
    if ":" not in raw:
        raise ValueError(f"bad item format (expect bank:question_id:answer): {item!r}")
    bank_key, question_id, answer_text = raw.split(":", 2)
    bank_key = bank_key.strip()
    question_id = question_id.strip()
    if not bank_key or not question_id:
        raise ValueError(f"bad item format (empty bank/question_id): {item!r}")
    answer = parse_answer(answer_text)
    return bank_key, question_id, answer


def load_csv(csv_path: str) -> List[Tuple[str, str, Any]]:
    path = pathlib.Path(csv_path).expanduser()
    if not path.exists():
        raise FileNotFoundError(f"CSV not found: {path}")

    items: List[Tuple[str, str, Any]] = []
    with path.open("r", encoding="utf-8-sig", newline="") as f:
        reader = csv.reader(f)
        rows = list(reader)
    if not rows:
        return items

    start_idx = 0
    first = [c.strip().lower() for c in rows[0]]
    if len(first) >= 3 and first[0] == "bank" and first[1] == "question_id":
        start_idx = 1

    for row in rows[start_idx:]:
        if not row or len(row) < 3:
            continue
        bank_key = str(row[0]).strip()
        question_id = str(row[1]).strip()
        answer = parse_answer(str(row[2]).strip())
        if not bank_key or not question_id:
            continue
        items.append((bank_key, question_id, answer))
    return items


def load_json(json_path: str) -> List[Tuple[str, str, Any]]:
    path = pathlib.Path(json_path).expanduser()
    if not path.exists():
        raise FileNotFoundError(f"JSON file not found: {path}")
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, list):
        raise ValueError(f"JSON format invalid: expect list of items in {path}")
    items = []
    for index, item in enumerate(payload, start=1):
        if not isinstance(item, dict):
            raise ValueError(f"JSON item #{index} should be object: {item!r}")
        bank_key = str(item.get("bank") or item.get("bank_key") or "").strip()
        question_id = str(item.get("question_id") or item.get("id") or "").strip()
        if "answer" not in item:
            raise ValueError(f"JSON item #{index} missing answer: {item!r}")
        answer = item["answer"]
        if not bank_key or not question_id:
            raise ValueError(f"JSON item #{index} missing bank/question_id: {item!r}")
        items.append((bank_key, question_id, answer))
    return items


def fix_questions(items: List[Tuple[str, str, Any]], dry_run: bool = False) -> int:
    if not items:
        print("no items to fix")
        return 1

    if not db_storage.is_enabled():
        print("DATABASE_URL is required", file=sys.stderr)
        return 1

    db_storage.init_schema()
    success = 0
    fail = 0

    with db_storage.connect() as conn:
        with conn.cursor() as cur:
            for bank_key, question_id, answer in items:
                cur.execute(
                    "SELECT 1 FROM bank_questions WHERE bank_key=%s AND question_id=%s",
                    (bank_key, question_id),
                )
                row = cur.fetchone()
                if row is None:
                    print(f"[skip] missing question: {bank_key}/{question_id}")
                    fail += 1
                    continue

                if dry_run:
                    print(
                        f"[dry-run] {bank_key}/{question_id}: set answer -> {json.dumps(answer, ensure_ascii=False)}"
                    )
                    success += 1
                    continue

                cur.execute(
                    """
                    UPDATE bank_questions
                    SET answer=%s,
                        payload=jsonb_set(
                            COALESCE(payload, '{}'::jsonb),
                            '{answer}',
                            %s::jsonb,
                            true
                        )
                    WHERE bank_key=%s AND question_id=%s
                    """,
                    (
                        answer,
                        json.dumps(answer, ensure_ascii=False),
                        bank_key,
                        question_id,
                    ),
                )
                if cur.rowcount:
                    success += 1
                    print(f"[ok] {bank_key}/{question_id}: {json.dumps(answer, ensure_ascii=False)}")
                else:
                    fail += 1
                    print(f"[fail] {bank_key}/{question_id}")

    print(f"total={len(items)} success={success} fail={fail}")
    return 0 if fail == 0 else 1


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Batch fix question answers in the PostgreSQL question bank."
    )
    parser.add_argument(
        "items",
        nargs="*",
        help="inline fixes with format bank:question_id:answer",
    )
    parser.add_argument(
        "--item",
        action="append",
        default=[],
        help="same as positional arg; repeatable",
    )
    parser.add_argument(
        "--csv",
        help="CSV file with columns: bank,question_id,answer",
    )
    parser.add_argument(
        "--json",
        help="JSON file with list of objects: {bank,question_id,answer} or {bank_key,id,answer}",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="preview SQL changes without writing to database",
    )
    args = parser.parse_args()

    all_items: List[Tuple[str, str, Any]] = []

    for raw in args.item + list(args.items):
        all_items.append(parse_item(raw))

    if args.csv:
        all_items.extend(load_csv(args.csv))

    if args.json:
        all_items.extend(load_json(args.json))

    return fix_questions(all_items, dry_run=args.dry_run)


if __name__ == "__main__":
    raise SystemExit(main())
