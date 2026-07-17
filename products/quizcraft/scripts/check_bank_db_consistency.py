#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Check tiku JSON question banks against PostgreSQL shadow tables."""

import json
import pathlib
import sys
from typing import Any, Dict

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

import db_storage
import server


def normalize_answer(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True)


def main() -> int:
    if not db_storage.is_enabled():
        print("DATABASE_URL is required", file=sys.stderr)
        return 1

    db_storage.init_schema()
    server.load_question_banks()
    db_snapshot = db_storage.bank_consistency_snapshot()

    errors = []
    for bank_key, bank in server.QUESTION_BANKS.items():
        questions = server.parse_question_bank(bank["data"], bank_key)
        db_bank = db_snapshot.get(bank_key)
        if not db_bank:
            errors.append(f"{bank_key}: missing in DB")
            continue
        if db_bank["total"] != len(questions):
            errors.append(f"{bank_key}: count json={len(questions)} db={db_bank['total']}")
        db_questions: Dict[str, Any] = db_bank["questions"]
        for question in questions:
            qid = str(question.get("id") or "")
            if qid not in db_questions:
                errors.append(f"{bank_key}/{qid}: missing in DB")
                continue
            if normalize_answer(question.get("answer")) != normalize_answer(db_questions[qid]):
                errors.append(f"{bank_key}/{qid}: answer mismatch")

    if errors:
        for error in errors:
            print(error)
        return 1

    print(f"ok banks={len(server.QUESTION_BANKS)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
