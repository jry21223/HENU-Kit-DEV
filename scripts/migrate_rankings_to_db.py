#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Migrate runtime JSON files into PostgreSQL."""

import argparse
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

import db_storage


def main() -> int:
    parser = argparse.ArgumentParser(description="Import runtime JSON files into PostgreSQL")
    parser.add_argument(
        "--rankings-file",
        default=str(ROOT / "rankings_v2.json"),
        help="rankings JSON path",
    )
    parser.add_argument(
        "--question-stats-file",
        default=str(ROOT / "question_stats.json"),
        help="question stats JSON path; skipped when missing",
    )
    args = parser.parse_args()

    if not db_storage.is_enabled():
        print("DATABASE_URL is required", file=sys.stderr)
        return 1

    db_storage.init_schema()
    rankings_path = pathlib.Path(args.rankings_file)
    if rankings_path.exists():
        payload = json.loads(rankings_path.read_text(encoding="utf-8"))
        result = db_storage.import_rankings_payload(payload)
        print(f"imported_users={result['users']}")
        print(f"imported_name_to_id={result['name_to_id']}")
    else:
        print(f"rankings file skipped: {rankings_path}")

    question_stats_path = pathlib.Path(args.question_stats_file)
    if question_stats_path.exists():
        payload = json.loads(question_stats_path.read_text(encoding="utf-8"))
        result = db_storage.import_question_stats_payload(payload)
        print(f"imported_question_stats={result['question_stats']}")
    else:
        print(f"question stats file skipped: {question_stats_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
