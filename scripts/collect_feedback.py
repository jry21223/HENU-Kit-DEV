#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Collect and export feedback data.

Usage examples:
  1. Export latest feedback to CSV:
     python3 scripts/collect_feedback.py --format csv --output /tmp/feedback.csv

  2. Export a specific bank and question index range:
     python3 scripts/collect_feedback.py --bank ethics --question-index 178 --format json

  3. Export from fallback JSON file only:
     python3 scripts/collect_feedback.py --source file --format csv
"""

from __future__ import annotations

import argparse
import csv
import json
import os
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Iterable, List, Mapping, Optional
import sys

PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

try:
    import db_storage
except Exception as exc:  # pragma: no cover
    db_storage = None
    _DB_IMPORT_ERROR = exc
else:
    _DB_IMPORT_ERROR = None

try:
    import server
except Exception as exc:  # pragma: no cover
    server = None
    _SERVER_IMPORT_ERROR = exc
else:
    _SERVER_IMPORT_ERROR = None


@dataclass
class FeedbackRecord:
    feedback_id: int
    question_index: int
    question_bank: Optional[str]
    suggestion: str
    user_id: Optional[str]
    source_page: str
    created_at: str
    user_name: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        return {
            "feedback_id": self.feedback_id,
            "question_index": self.question_index,
            "question_bank": self.question_bank or "",
            "user_id": self.user_id or "",
            "user_name": self.user_name or "",
            "source_page": self.source_page,
            "created_at": self.created_at,
            "suggestion": self.suggestion,
        }


def _parse_datetime(value: str) -> str:
    try:
        value_dt = datetime.fromisoformat(value)
        return value_dt.isoformat()
    except ValueError:
        raise argparse.ArgumentTypeError(f"invalid time format: {value!r}; use ISO format, e.g. 2026-06-09T20:00:00")


def _coerce_feedback_records(raw: Mapping[str, Any]) -> FeedbackRecord:
    feedback_id = int(raw.get("feedback_id", 0) or 0)
    question_index = int(raw.get("question_index", 0) or 0)
    return FeedbackRecord(
        feedback_id=feedback_id,
        question_index=question_index,
        question_bank=raw.get("question_bank"),
        suggestion=str(raw.get("suggestion", "")).strip(),
        user_id=raw.get("user_id"),
        source_page=str(raw.get("source_page", "quiz")),
        created_at=str(raw.get("created_at", "")),
        user_name=raw.get("user_name") or None,
    )


def _load_json_file() -> List[FeedbackRecord]:
    if server is None:
        raise RuntimeError(
            f"server module not loadable, unable to locate fallback path: { _SERVER_IMPORT_ERROR }"
        )

    feedback_file = Path(getattr(server, "FEEDBACK_FILE", PROJECT_ROOT / "feedbacks.json"))
    if not feedback_file.exists():
        return []

    with feedback_file.open("r", encoding="utf-8") as f:
        rows = json.load(f)

    return [_coerce_feedback_records(item) for item in rows if isinstance(item, dict)]


def _build_where(filters: argparse.Namespace) -> tuple[str, list[Any]]:
    clauses: list[str] = []
    params: list[Any] = []

    if filters.bank:
        clauses.append("question_bank = %s")
        params.append(filters.bank)

    if filters.question_indices:
        placeholders = ",".join(["%s"] * len(filters.question_indices))
        clauses.append(f"question_index IN ({placeholders})")
        params.extend(filters.question_indices)

    if filters.user_id:
        clauses.append("user_id = %s")
        params.append(filters.user_id)

    if filters.source_page:
        clauses.append("source_page = %s")
        params.append(filters.source_page)

    if filters.min_id is not None:
        clauses.append("feedback_id >= %s")
        params.append(filters.min_id)

    if filters.max_id is not None:
        clauses.append("feedback_id <= %s")
        params.append(filters.max_id)

    if filters.start_time:
        clauses.append("created_at >= %s")
        params.append(filters.start_time)

    if filters.end_time:
        clauses.append("created_at <= %s")
        params.append(filters.end_time)

    where = f"WHERE {' AND '.join(clauses)}" if clauses else ""
    return where, params


def _load_from_db(filters: argparse.Namespace) -> List[FeedbackRecord]:
    if db_storage is None or _DB_IMPORT_ERROR is not None:
        raise RuntimeError(f"db_storage unavailable: {_DB_IMPORT_ERROR}")

    if not db_storage.is_available():
        raise RuntimeError("DATABASE_URL is not configured or psycopg unavailable")

    where, params = _build_where(filters)
    limit_sql = f"LIMIT {int(filters.limit)}" if filters.limit else ""

    sql = f"""
        SELECT
            f.feedback_id,
            f.question_index,
            f.question_bank,
            f.suggestion,
            f.user_id,
            COALESCE(u.display_name, '') AS user_name,
            f.source_page,
            f.created_at
        FROM feedbacks f
        LEFT JOIN users u ON u.user_id = f.user_id
        {where}
        ORDER BY f.feedback_id DESC
        {limit_sql}
    """

    with db_storage.connect() as conn:
        with conn.cursor() as cur:
            cur.execute(sql, params)
            rows = cur.fetchall()

    out = []
    for row in rows:
        if not row:
            continue
        feedback_id, question_index, question_bank, suggestion, user_id, user_name, source_page, created_at = row
        out.append(
            FeedbackRecord(
                feedback_id=int(feedback_id or 0),
                question_index=int(question_index or 0),
                question_bank=question_bank,
                suggestion=str(suggestion or "").strip(),
                user_id=user_id,
                source_page=str(source_page or "quiz"),
                created_at=str(created_at or ""),
                user_name=user_name,
            )
        )

    return out


def _load_feedback(
    source: str,
    filters: argparse.Namespace,
) -> List[FeedbackRecord]:
    records: list[FeedbackRecord]
    if source == "db":
        records = _load_from_db(filters)
    elif source == "file":
        records = _load_json_file()
    else:  # auto
        try:
            records = _load_from_db(filters)
        except Exception:
            records = _load_json_file()

    if filters.bank:
        records = [r for r in records if (r.question_bank or "") == filters.bank]

    if filters.min_index is not None:
        records = [r for r in records if r.question_index >= filters.min_index]

    if filters.max_index is not None:
        records = [r for r in records if r.question_index <= filters.max_index]

    if filters.limit and source == "file":
        records = records[: filters.limit]

    return records


def _export_csv(records: Iterable[FeedbackRecord], output: Optional[Path]) -> None:
    rows = [r.to_dict() for r in records]
    fields = [
        "feedback_id",
        "question_index",
        "question_bank",
        "user_id",
        "user_name",
        "source_page",
        "created_at",
        "suggestion",
    ]
    if output:
        out_f = output.open("w", encoding="utf-8", newline="")
    else:
        out_f = sys.stdout

    with out_f:
        writer = csv.DictWriter(out_f, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)


def _export_json(records: Iterable[FeedbackRecord], output: Optional[Path], pretty: bool) -> None:
    rows = [r.to_dict() for r in records]
    payload = {
        "count": len(rows),
        "items": rows,
    }

    if output:
        with output.open("w", encoding="utf-8") as f:
            json.dump(payload, f, ensure_ascii=False, indent=2 if pretty else None)
            if pretty:
                f.write("\n")
    else:
        print(json.dumps(payload, ensure_ascii=False, indent=2 if pretty else None))


def _add_filters(parser: argparse.ArgumentParser) -> argparse.ArgumentParser:
    parser.add_argument("--bank", help="Filter by question bank")
    parser.add_argument(
        "--question-index",
        dest="question_indices",
        action="append",
        type=int,
        help="Filter by one or more question indexes in bank metadata",
    )
    parser.add_argument("--min-index", type=int, help="Filter question_index >= this value")
    parser.add_argument("--max-index", type=int, help="Filter question_index <= this value")
    parser.add_argument("--user-id", help="Filter by user_id")
    parser.add_argument("--source-page", help="Filter source_page")
    parser.add_argument("--min-id", type=int, help="Filter feedback_id >= this value")
    parser.add_argument("--max-id", type=int, help="Filter feedback_id <= this value")
    parser.add_argument("--start-time", type=_parse_datetime, help="Filter created_at >= value")
    parser.add_argument("--end-time", type=_parse_datetime, help="Filter created_at <= value")
    parser.add_argument("--limit", type=int, default=0, help="Limit output count")
    parser.add_argument(
        "--source",
        choices=["auto", "db", "file"],
        default="auto",
        help="Feedback source, defaults to auto",
    )
    parser.add_argument("--format", choices=["csv", "json"], default="csv", help="Output format")
    parser.add_argument(
        "--output",
        type=Path,
        help="Output file path. If omitted, print to stdout",
    )
    parser.add_argument(
        "--json-pretty",
        action="store_true",
        help="Pretty print json output",
    )
    return parser


def main() -> int:
    parser = argparse.ArgumentParser(description="Export quiz feedback records")
    parser = _add_filters(parser)
    args = parser.parse_args()

    records = _load_feedback(args.source, args)

    if not records:
        print("feedback: no records")
        return 0

    if args.format == "json":
        _export_json(records, args.output, args.json_pretty)
    else:
        _export_csv(records, args.output)

    print(f"feedback count: {len(records)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
