#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Daily feedback reminder workflow for QuizCraft.

Usage:
  python3 scripts/feedback_workflow.py --env-file /etc/quizcraft-cn.env
  python3 scripts/feedback_workflow.py --env-file /etc/quizcraft-cn.env --notify
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple
from urllib import request, error as urllib_error
from urllib.parse import urlparse

from zoneinfo import ZoneInfo

try:
    import psycopg
except Exception as exc:  # pragma: no cover
    psycopg = None
    _PG_IMPORT_ERROR = exc
else:
    _PG_IMPORT_ERROR = None


PROJECT_ROOT = Path(__file__).resolve().parents[1]


def _read_env_file(path: Optional[str]) -> Dict[str, str]:
    if not path:
        return {}
    env_path = Path(path)
    if not env_path.exists():
        return {}

    values: Dict[str, str] = {}
    for line in env_path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("\"'")
        values[key] = value
    return values


def _load_feedback_file() -> Path:
    root = PROJECT_ROOT / "feedbacks.json"
    env_path = Path("/etc/quizcraft-cn.env")
    if env_path.exists():
        env = _read_env_file(str(env_path))
        custom = env.get("FEEDBACK_FILE")
        if custom:
            maybe = Path(custom)
            if not maybe.is_absolute():
                maybe = PROJECT_ROOT / maybe
            if maybe.exists():
                return maybe
    if root.exists():
        return root
    return Path("feedbacks.json")


@dataclass
class FeedbackRecord:
    feedback_id: int
    question_index: int
    question_bank: str
    user_id: str
    source_page: str
    suggestion: str
    created_at: str


def _parse_created_at(value: str) -> datetime:
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)


def _load_from_file(
    start_time: datetime, end_time: datetime
) -> Tuple[List[FeedbackRecord], Dict[str, int], int]:
    feedback_path = _load_feedback_file()
    if not feedback_path.exists():
        return [], {}, 0

    raw = json.loads(feedback_path.read_text(encoding="utf-8"))
    if not isinstance(raw, list):
        return [], {}, 0

    records: List[FeedbackRecord] = []
    bank_count: Counter[str] = Counter()

    for item in raw:
        if not isinstance(item, dict):
            continue
        try:
            created_at = _parse_created_at(str(item.get("created_at", "")))
        except Exception:
            continue
        if not (start_time <= created_at < end_time):
            continue

        fid = int(item.get("feedback_id", 0) or 0)
        qidx = int(item.get("question_index", 0) or 0)
        bank = str(item.get("question_bank") or "")
        record = FeedbackRecord(
            feedback_id=fid,
            question_index=qidx,
            question_bank=bank,
            user_id=str(item.get("user_id") or ""),
            source_page=str(item.get("source_page") or "quiz"),
            suggestion=str(item.get("suggestion") or ""),
            created_at=str(item.get("created_at") or ""),
        )
        records.append(record)
        bank_count[bank] += 1

    return records, dict(bank_count), len(records)


def _load_from_db(
    database_url: str,
    start_time: datetime,
    end_time: datetime,
    top: int,
) -> Tuple[List[FeedbackRecord], Dict[str, int], int]:
    if psycopg is None:
        raise RuntimeError(f"psycopg not available: {_PG_IMPORT_ERROR}")

    with psycopg.connect(database_url) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT COUNT(*)
                FROM feedbacks
                WHERE created_at >= %s AND created_at < %s
                """,
                (start_time, end_time),
            )
            total = int((cur.fetchone() or (0,))[0] or 0)

            cur.execute(
                """
                SELECT COALESCE(question_bank, ''), COUNT(*),
                       COALESCE(MAX(created_at) FILTER (WHERE created_at IS NOT NULL), now())
                FROM feedbacks
                WHERE created_at >= %s AND created_at < %s
                GROUP BY COALESCE(question_bank, '')
                ORDER BY COUNT(*) DESC
                LIMIT %s
                """,
                (start_time, end_time, int(top)),
            )
            by_bank_raw = cur.fetchall() or []

            cur.execute(
                """
                SELECT feedback_id, question_index, COALESCE(question_bank, ''),
                       COALESCE(user_id, ''), COALESCE(source_page, 'quiz'),
                       suggestion, created_at
                FROM feedbacks
                WHERE created_at >= %s AND created_at < %s
                ORDER BY created_at DESC
                LIMIT %s
                """,
                (start_time, end_time, int(top)),
            )
            rows = cur.fetchall() or []

    by_bank = {str(name): int(cnt) for name, cnt, _ in by_bank_raw}
    records = [
        FeedbackRecord(
            feedback_id=int(fid or 0),
            question_index=int(qidx or 0),
            question_bank=str(bank),
            user_id=str(user_id),
            source_page=str(source),
            suggestion=str(suggestion or ""),
            created_at=str(created_at or ""),
        )
        for fid, qidx, bank, user_id, source, suggestion, created_at in rows
    ]
    return records, by_bank, total


def _format_summary(
    start_time: datetime,
    end_time: datetime,
    total: int,
    by_bank: Dict[str, int],
    recent: List[FeedbackRecord],
) -> str:
    title = "【QuizCraft 反馈提醒】"
    lines = [
        title,
        f"周期: {start_time.strftime('%Y-%m-%d %H:%M')} - {end_time.strftime('%Y-%m-%d %H:%M')}",
        f"今日新增反馈: {total} 条",
        "",
    ]

    if not total:
        lines.append("今日无新增反馈，暂不需要处理。")
        return "\n".join(lines)

    if by_bank:
        lines.append("题库分布:")
        for idx, (bank, count) in enumerate(sorted(by_bank.items(), key=lambda it: (-it[1], it[0])), 1):
            bank_display = bank or "(未填写题库)"
            lines.append(f"{idx}. {bank_display}: {count} 条")
        lines.append("")

    if recent:
        lines.append("最近反馈:")
        for idx, item in enumerate(recent[:5], 1):
            bank = item.question_bank or "(未填写题库)"
            suggestion_preview = item.suggestion.replace("\n", " ")
            if len(suggestion_preview) > 28:
                suggestion_preview = suggestion_preview[:28] + "..."
            lines.append(
                f"{idx}. [{item.feedback_id}] {bank} 第{item.question_index}题 | {suggestion_preview}"
            )
    return "\n".join(lines)


def _send_webhook(webhook_url: str, text: str) -> None:
    payload = json.dumps({"text": text}, ensure_ascii=False).encode("utf-8")
    req = request.Request(
        webhook_url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with request.urlopen(req, timeout=10) as resp:
            _ = resp.read()
    except urllib_error.URLError as exc:  # pragma: no cover
        raise RuntimeError(f"webhook request failed: {exc}")


def _parse_time_range(hours: int, timezone: str) -> Tuple[datetime, datetime]:
    tz = ZoneInfo(timezone)
    now = datetime.now(tz)
    start = now - timedelta(hours=hours)
    return start, now


def main() -> int:
    parser = argparse.ArgumentParser(description="QuizCraft feedback workflow")
    parser.add_argument("--env-file", help="Path to env file with DATABASE_URL", default="/etc/quizcraft-cn.env")
    parser.add_argument("--hours", type=int, default=24, help="Look back window in hours")
    parser.add_argument("--top", type=int, default=10, help="Top items to fetch")
    parser.add_argument("--timezone", default="Asia/Shanghai", help="Timezone for time window")
    parser.add_argument("--webhook", help="Webhook URL to send reminders")
    parser.add_argument("--notify", action="store_true", help="Send webhook notification when reminder is generated")
    parser.add_argument("--output", help="Write summary to file")
    parser.add_argument("--quiet", action="store_true", help="Only write output file when set")
    parser.add_argument(
        "--source",
        choices=["auto", "db", "file"],
        default="auto",
        help="Data source",
    )

    args = parser.parse_args()

    env = _read_env_file(args.env_file)
    database_url = env.get("DATABASE_URL") or os.environ.get("DATABASE_URL")
    webhook_url = args.webhook or env.get("FEEDBACK_WEBHOOK_URL")

    start_at, end_at = _parse_time_range(args.hours, args.timezone)

    if args.source == "db" and database_url:
        records, by_bank, total = _load_from_db(database_url, start_at, end_at, args.top)
    elif args.source == "file":
        records, by_bank, total = _load_from_file(start_at, end_at)
    else:
        if database_url:
            records, by_bank, total = _load_from_db(database_url, start_at, end_at, args.top)
        else:
            records, by_bank, total = _load_from_file(start_at, end_at)

    summary = _format_summary(start_at, end_at, total, by_bank, records)

    if not args.quiet:
        print(summary)

    if args.output:
        out_path = Path(args.output)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        payload = {
            "start": start_at.isoformat(),
            "end": end_at.isoformat(),
            "total": total,
            "by_bank": by_bank,
            "latest": [
                {
                    "feedback_id": r.feedback_id,
                    "question_index": r.question_index,
                    "question_bank": r.question_bank,
                    "user_id": r.user_id,
                    "source_page": r.source_page,
                    "created_at": r.created_at,
                    "suggestion": r.suggestion,
                }
                for r in records
            ],
            "summary": summary,
        }
        out_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")

    if args.notify and webhook_url:
        try:
            _send_webhook(webhook_url, summary)
        except Exception as exc:
            print(f"notify_failed={exc}", file=sys.stderr)
            return 2

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
