#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""PostgreSQL persistence for QuizCraft runtime and bank shadow data."""

import json
import os
from contextlib import contextmanager
from typing import Any, Dict, Iterable, List, Optional, Tuple

try:
    import psycopg
    from psycopg import errors as pg_errors
    from psycopg.types.json import Jsonb
except ImportError:  # pragma: no cover - dependency can be absent in local fallback mode
    psycopg = None
    pg_errors = None
    Jsonb = None


DATABASE_URL = os.getenv("DATABASE_URL", "").strip()
GENERATED_USER_ID_PREFIX = "qc_user_"


def generated_user_id(sequence: int) -> str:
    return f"{GENERATED_USER_ID_PREFIX}{sequence}"


def user_id_sequence_number(user_id: Any) -> Optional[int]:
    text = str(user_id or "").strip()
    if text.isdigit():
        return int(text)
    if text.startswith(GENERATED_USER_ID_PREFIX):
        suffix = text[len(GENERATED_USER_ID_PREFIX):]
        if suffix.isdigit():
            return int(suffix)
    return None


def is_enabled() -> bool:
    return bool(DATABASE_URL)


def is_available() -> bool:
    return is_enabled() and psycopg is not None


def require_available() -> None:
    if not is_enabled():
        raise RuntimeError("DATABASE_URL is not configured")
    if psycopg is None:
        raise RuntimeError("psycopg is not installed; install requirements.txt")


@contextmanager
def connect():
    require_available()
    with psycopg.connect(DATABASE_URL) as conn:
        yield conn


def init_schema() -> None:
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS users (
                    user_id TEXT PRIMARY KEY,
                    display_name TEXT NOT NULL,
                    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
                )
                """
            )
            cur.execute(
                """
                CREATE UNIQUE INDEX IF NOT EXISTS idx_users_display_name_unique
                ON users(display_name)
                WHERE display_name <> ''
                """
            )
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS user_stats (
                    user_id TEXT PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
                    correct INTEGER NOT NULL DEFAULT 0,
                    total INTEGER NOT NULL DEFAULT 0,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
                )
                """
            )
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS question_stats (
                    bank_key TEXT NOT NULL,
                    question_id TEXT NOT NULL,
                    total INTEGER NOT NULL DEFAULT 0,
                    correct INTEGER NOT NULL DEFAULT 0,
                    rate DOUBLE PRECISION NOT NULL DEFAULT 0,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                    PRIMARY KEY (bank_key, question_id)
                )
                """
            )
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS question_banks (
                    bank_key TEXT PRIMARY KEY,
                    name TEXT NOT NULL,
                    color TEXT NOT NULL,
                    source_file TEXT NOT NULL,
                    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
                )
                """
            )
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS bank_questions (
                    bank_key TEXT NOT NULL REFERENCES question_banks(bank_key) ON DELETE CASCADE,
                    question_id TEXT NOT NULL,
                    type TEXT NOT NULL,
                    chapter TEXT NOT NULL,
                    answer JSONB NOT NULL,
                    payload JSONB NOT NULL,
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                    PRIMARY KEY (bank_key, question_id)
                )
                """
            )
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS feedbacks (
                    feedback_id BIGSERIAL PRIMARY KEY,
                    question_index INTEGER NOT NULL CHECK (question_index > 0),
                    question_id TEXT,
                    question_content TEXT,
                    suggestion TEXT NOT NULL,
                    user_id TEXT,
                    source_page TEXT NOT NULL DEFAULT 'quiz',
                    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
                )
                """
            )
            cur.execute(
                "ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS question_bank TEXT"
            )
            cur.execute(
                "ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS question_id TEXT"
            )
            cur.execute(
                "ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS question_content TEXT"
            )
            cur.execute(
                "ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending'"
            )
            cur.execute(
                "ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ"
            )
            cur.execute(
                "ALTER TABLE feedbacks ADD COLUMN IF NOT EXISTS resolution_note TEXT NOT NULL DEFAULT ''"
            )
            cur.execute(
                "CREATE INDEX IF NOT EXISTS idx_feedbacks_created_at ON feedbacks(created_at DESC)"
            )
            cur.execute(
                "CREATE INDEX IF NOT EXISTS idx_feedbacks_question_index ON feedbacks(question_index)"
            )
            cur.execute(
                "CREATE INDEX IF NOT EXISTS idx_feedbacks_status ON feedbacks(status)"
            )
            cur.execute(
                "CREATE INDEX IF NOT EXISTS idx_feedbacks_resolved_at ON feedbacks(resolved_at DESC)"
            )
            cur.execute(
                """
                CREATE TABLE IF NOT EXISTS food_wheel_items (
                    id BIGSERIAL PRIMARY KEY,
                    owner_user_id TEXT NOT NULL,
                    owner_name TEXT NOT NULL DEFAULT '',
                    items JSONB NOT NULL DEFAULT '[]'::jsonb,
                    is_public BOOLEAN NOT NULL DEFAULT true,
                    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
                )
                """
            )
            cur.execute(
                """
                SELECT EXISTS (
                    SELECT 1
                    FROM information_schema.columns
                    WHERE table_name = 'food_wheel_items'
                        AND column_name = 'owner_user_id'
                )
                """
            )
            if not cur.fetchone()[0]:
                cur.execute("ALTER TABLE food_wheel_items RENAME TO food_wheel_items_legacy")
                cur.execute(
                    """
                    CREATE TABLE IF NOT EXISTS food_wheel_items (
                        id BIGSERIAL PRIMARY KEY,
                        owner_user_id TEXT NOT NULL,
                        owner_name TEXT NOT NULL DEFAULT '',
                        items JSONB NOT NULL DEFAULT '[]'::jsonb,
                        is_public BOOLEAN NOT NULL DEFAULT true,
                        created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                        updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
                    )
                    """
                )
                cur.execute(
                    """
                    INSERT INTO food_wheel_items (
                        owner_user_id,
                        owner_name,
                        items,
                        is_public,
                        created_at,
                        updated_at
                    )
                    SELECT
                        'legacy',
                        'legacy',
                        items,
                        true,
                        COALESCE(updated_at, now()),
                        COALESCE(updated_at, now())
                    FROM food_wheel_items_legacy
                    WHERE id = 1
                    """
                )
                cur.execute("DROP TABLE food_wheel_items_legacy")

            cur.execute(
                "ALTER TABLE food_wheel_items ADD COLUMN IF NOT EXISTS owner_user_id TEXT"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ADD COLUMN IF NOT EXISTS owner_name TEXT"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ALTER COLUMN owner_name SET DEFAULT ''"
            )
            cur.execute(
                "UPDATE food_wheel_items SET owner_name = '' WHERE owner_name IS NULL"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ALTER COLUMN owner_name SET NOT NULL"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT true"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now()"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ALTER COLUMN created_at SET DEFAULT now()"
            )
            cur.execute(
                "UPDATE food_wheel_items SET created_at = now() WHERE created_at IS NULL"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ALTER COLUMN created_at SET NOT NULL"
            )
            cur.execute(
                "UPDATE food_wheel_items SET owner_user_id = 'legacy' WHERE owner_user_id IS NULL"
            )
            cur.execute(
                "ALTER TABLE food_wheel_items ALTER COLUMN owner_user_id SET NOT NULL"
            )
            cur.execute(
                """
                SELECT con.conname
                FROM pg_constraint con
                JOIN pg_class rel ON rel.oid = con.conrelid
                WHERE rel.relname = 'food_wheel_items'
                  AND con.contype = 'p'
                """
            )
            for (pk_name,) in cur.fetchall():
                if pk_name:
                    cur.execute(f"ALTER TABLE food_wheel_items DROP CONSTRAINT IF EXISTS {pk_name}")

            cur.execute(
                """
                SELECT con.conname
                FROM pg_constraint con
                JOIN pg_class rel ON rel.oid = con.conrelid
                WHERE rel.relname = 'food_wheel_items'
                  AND con.contype = 'c'
                  AND pg_get_constraintdef(con.oid) LIKE '%%id = 1%%'
                """
            )
            for (check_name,) in cur.fetchall():
                if check_name:
                    cur.execute(f"ALTER TABLE food_wheel_items DROP CONSTRAINT IF EXISTS {check_name}")

            cur.execute(
                """
                SELECT column_default
                FROM information_schema.columns
                WHERE table_name = 'food_wheel_items'
                  AND column_name = 'id'
                """
            )
            id_default = cur.fetchone()[0]
            if not id_default or "nextval(" not in str(id_default):
                cur.execute("CREATE SEQUENCE IF NOT EXISTS food_wheel_items_id_seq")
                cur.execute("SELECT COALESCE(MAX(id), 0) FROM food_wheel_items")
                max_id = cur.fetchone()[0] or 0
                cur.execute(
                    "SELECT setval('food_wheel_items_id_seq', %s, true)",
                    (max_id,),
                )
                cur.execute(
                    "ALTER TABLE food_wheel_items ALTER COLUMN id SET DEFAULT nextval('food_wheel_items_id_seq')"
                )

            cur.execute(
                "ALTER TABLE food_wheel_items ADD PRIMARY KEY (id)"
            )
            cur.execute(
                "CREATE UNIQUE INDEX IF NOT EXISTS idx_food_wheel_items_owner_user_id ON food_wheel_items (owner_user_id)"
            )
            cur.execute(
                "CREATE INDEX IF NOT EXISTS idx_food_wheel_items_public_updated_at ON food_wheel_items (is_public, updated_at DESC)"
            )


def _rate(correct: int, total: int) -> float:
    return round(correct / total * 100, 1) if total else 0.0


def _coerce_int(value: Any) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def _normalize_food_wheel_items(values: Any) -> List[str]:
    if not isinstance(values, list):
        return []

    normalized: List[str] = []
    seen = set()
    for value in values:
        text = str(value or "").strip()
        if not text or text in seen:
            continue
        seen.add(text)
        normalized.append(text)
    return normalized


def load_runtime_state() -> Tuple[Dict[str, Dict[str, Any]], Dict[str, str], int]:
    users: Dict[str, Dict[str, Any]] = {}
    name_to_id: Dict[str, str] = {}
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT u.user_id, u.display_name, COALESCE(s.correct, 0), COALESCE(s.total, 0)
                FROM users u
                LEFT JOIN user_stats s ON s.user_id = u.user_id
                ORDER BY u.user_id
                """
            )
            for user_id, display_name, correct, total in cur.fetchall():
                correct_i = _coerce_int(correct)
                total_i = _coerce_int(total)
                users[str(user_id)] = {
                    "name": str(display_name or user_id),
                    "correct": correct_i,
                    "total": total_i,
                    "practice_history": [],
                }
                if display_name:
                    name_to_id[str(display_name)] = str(user_id)

    sequence_ids = [
        sequence
        for uid in users
        if (sequence := user_id_sequence_number(uid)) is not None
    ]
    next_user_id = max(sequence_ids + [0]) + 1
    return users, name_to_id, next_user_id


def load_question_stats() -> Dict[str, Dict[str, Dict[str, float]]]:
    stats: Dict[str, Dict[str, Dict[str, float]]] = {}
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT bank_key, question_id, total, correct, rate
                FROM question_stats
                """
            )
            for bank_key, question_id, total, correct, rate in cur.fetchall():
                stats.setdefault(str(bank_key), {})[str(question_id)] = {
                    "total": _coerce_int(total),
                    "correct": _coerce_int(correct),
                    "rate": float(rate or 0),
                }
    return stats


def upsert_user(user_id: str, display_name: Optional[str] = None) -> Dict[str, Any]:
    name = (display_name or user_id).strip() or user_id
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO users (user_id, display_name)
                VALUES (%s, %s)
                ON CONFLICT (user_id) DO UPDATE
                SET display_name = EXCLUDED.display_name
                """,
                (user_id, name),
            )
            cur.execute(
                """
                INSERT INTO user_stats (user_id, correct, total)
                VALUES (%s, 0, 0)
                ON CONFLICT (user_id) DO NOTHING
                """,
                (user_id,),
            )
            cur.execute(
                """
                SELECT correct, total
                FROM user_stats
                WHERE user_id = %s
                """,
                (user_id,),
            )
            row = cur.fetchone()
    correct = _coerce_int(row[0]) if row else 0
    total = _coerce_int(row[1]) if row else 0
    return {
        "name": name,
        "correct": correct,
        "total": total,
        "practice_history": [],
    }


def find_user_by_name(name: str) -> Optional[Tuple[str, Dict[str, Any]]]:
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT u.user_id, u.display_name, COALESCE(s.correct, 0), COALESCE(s.total, 0)
                FROM users u
                LEFT JOIN user_stats s ON s.user_id = u.user_id
                WHERE u.display_name = %s
                """,
                (name,),
            )
            row = cur.fetchone()
    if not row:
        return None
    user_id, display_name, correct, total = row
    return str(user_id), {
        "name": str(display_name or user_id),
        "correct": _coerce_int(correct),
        "total": _coerce_int(total),
        "practice_history": [],
    }


def create_feedback(
    question_index: int,
    suggestion: str,
    user_id: Optional[str] = None,
    question_bank: Optional[str] = None,
    question_id: Optional[str] = None,
    question_content: Optional[str] = None,
    source_page: str = "quiz",
) -> Dict[str, Any]:
    normalized_suggestion = str(suggestion or "").strip()
    index_value = int(question_index)
    normalized_bank = (question_bank or "").strip() or None
    normalized_question_id = (question_id or "").strip() or None
    normalized_question_content = (question_content or "").strip() or None

    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO feedbacks (
                    question_index,
                    question_id,
                    question_content,
                    suggestion,
                    user_id,
                    question_bank,
                    source_page
                )
                VALUES (%s, %s, %s, %s, %s, %s, %s)
                RETURNING feedback_id, created_at
                """,
                (
                    index_value,
                    normalized_question_id,
                    normalized_question_content,
                    normalized_suggestion,
                    user_id,
                    normalized_bank,
                    source_page,
                ),
            )
            feedback_id, created_at = cur.fetchone()

    return {
        "feedback_id": int(feedback_id),
        "question_index": index_value,
        "question_id": normalized_question_id,
        "question_content": normalized_question_content,
        "suggestion": normalized_suggestion,
        "created_at": str(created_at),
        "user_id": user_id,
        "question_bank": normalized_bank,
        "source_page": source_page,
    }


def _normalize_feedback_status(status: Optional[str]) -> str:
    normalized = str(status or "pending").strip().lower()
    return normalized if normalized in {"pending", "resolved", "archived"} else "pending"


def _serialize_feedback_row(row: Tuple[Any, ...]) -> Dict[str, Any]:
    (
        feedback_id,
        question_index,
        question_id,
        question_content,
        question_bank,
        suggestion,
        user_id,
        source_page,
        created_at,
        status,
        resolved_at,
        resolution_note,
    ) = row
    return {
        "feedback_id": int(feedback_id or 0),
        "question_index": int(question_index or 0),
        "question_id": str(question_id or "") or None,
        "question_content": str(question_content or "") or None,
        "question_bank": str(question_bank or "") or None,
        "suggestion": str(suggestion or ""),
        "user_id": str(user_id or "") or None,
        "source_page": str(source_page or "quiz") or "quiz",
        "created_at": str(created_at or ""),
        "status": _normalize_feedback_status(status),
        "resolved_at": str(resolved_at or "") or None,
        "resolution_note": str(resolution_note or ""),
    }


def _feedback_select_sql() -> str:
    return """
        SELECT
            feedback_id,
            question_index,
            question_id,
            question_content,
            question_bank,
            suggestion,
            user_id,
            source_page,
            created_at,
            status,
            resolved_at,
            resolution_note
        FROM feedbacks
    """


def get_feedback_dashboard(
    today_start: Any,
    today_end: Any,
    pending_limit: int = 100,
    resolved_limit: int = 100,
    archived_limit: int = 100,
) -> Dict[str, Any]:
    pending_limit = max(1, min(int(pending_limit or 100), 500))
    resolved_limit = max(1, min(int(resolved_limit or 100), 500))
    archived_limit = max(1, min(int(archived_limit or 100), 500))

    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT
                    COUNT(*) FILTER (WHERE created_at >= %s AND created_at < %s) AS today_total,
                    COUNT(*) FILTER (WHERE status = 'pending') AS pending_total,
                    COUNT(*) FILTER (WHERE status = 'resolved') AS resolved_total,
                    COUNT(*) FILTER (WHERE status = 'archived') AS archived_total
                FROM feedbacks
                """,
                (today_start, today_end),
            )
            today_total, pending_total, resolved_total, archived_total = cur.fetchone()

            cur.execute(
                _feedback_select_sql()
                + """
                WHERE status = 'pending'
                ORDER BY created_at DESC, feedback_id DESC
                LIMIT %s
                """,
                (pending_limit,),
            )
            pending_items = [_serialize_feedback_row(row) for row in cur.fetchall()]

            cur.execute(
                _feedback_select_sql()
                + """
                WHERE status = 'resolved'
                ORDER BY resolved_at DESC NULLS LAST, created_at DESC, feedback_id DESC
                LIMIT %s
                """,
                (resolved_limit,),
            )
            resolved_items = [_serialize_feedback_row(row) for row in cur.fetchall()]

            cur.execute(
                _feedback_select_sql()
                + """
                WHERE status = 'archived'
                ORDER BY created_at DESC, feedback_id DESC
                LIMIT %s
                """,
                (archived_limit,),
            )
            archived_items = [_serialize_feedback_row(row) for row in cur.fetchall()]

    return {
        "summary": {
            "today_total": int(today_total or 0),
            "pending_total": int(pending_total or 0),
            "resolved_total": int(resolved_total or 0),
            "archived_total": int(archived_total or 0),
        },
        "pending_items": pending_items,
        "resolved_items": resolved_items,
        "archived_items": archived_items,
    }


def update_feedback_status(
    feedback_id: int,
    status: str,
    resolution_note: Optional[str] = None,
) -> Optional[Dict[str, Any]]:
    normalized_status = _normalize_feedback_status(status)
    note = str(resolution_note or "").strip()
    if len(note) > 1000:
        note = note[:1000]

    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                _feedback_select_sql()
                + """
                WHERE feedback_id = %s
                FOR UPDATE
                """,
                (int(feedback_id),),
            )
            existing = cur.fetchone()
            if not existing:
                return None

            cur.execute(
                """
                UPDATE feedbacks
                SET
                    status = %s,
                    resolution_note = %s,
                    resolved_at = CASE
                        WHEN %s = 'resolved' THEN COALESCE(resolved_at, now())
                        ELSE NULL
                    END
                WHERE feedback_id = %s
                RETURNING
                    feedback_id,
                    question_index,
                    question_id,
                    question_content,
                    question_bank,
                    suggestion,
                    user_id,
                    source_page,
                    created_at,
                    status,
                    resolved_at,
                    resolution_note
                """,
                (normalized_status, note, normalized_status, int(feedback_id)),
            )
            row = cur.fetchone()

    return _serialize_feedback_row(row) if row else None


def increment_user_stats(user_id: str, display_name: Optional[str], is_correct: bool) -> Dict[str, Any]:
    name = (display_name or user_id).strip() or user_id
    correct_inc = 1 if is_correct else 0
    resolved_user_id = str(user_id)

    with connect() as conn:
        with conn.cursor() as cur:
            # 优先复用当前 user_id；不存在时按 display_name 回查复用历史用户，避免 display_name 唯一约束冲突。
            cur.execute(
                "SELECT display_name FROM users WHERE user_id = %s",
                (resolved_user_id,),
            )
            user_row = cur.fetchone()

            if user_row is None:
                if name:
                    cur.execute(
                        "SELECT user_id, display_name FROM users WHERE display_name = %s",
                        (name,),
                    )
                    user_row = cur.fetchone()
                    if user_row is not None:
                        resolved_user_id = str(user_row[0])

            if user_row is None:
                try:
                    cur.execute(
                        """
                        INSERT INTO users (user_id, display_name)
                        VALUES (%s, %s)
                        """,
                        (resolved_user_id, name),
                    )
                except Exception as exc:
                    conn.rollback()
                    if pg_errors is not None and isinstance(exc, pg_errors.UniqueViolation):
                        cur.execute(
                            "SELECT user_id, display_name FROM users WHERE display_name = %s",
                            (name,),
                        )
                        user_row = cur.fetchone()
                        if user_row is None:
                            raise
                        resolved_user_id = str(user_row[0])
                        name = str(user_row[1] or resolved_user_id)
                    else:
                        raise
            elif user_row and not str(user_row[0]).strip() and name:
                cur.execute(
                    """
                    UPDATE users
                    SET display_name = %s
                    WHERE user_id = %s
                    """,
                    (name, resolved_user_id),
                )

            if resolved_user_id != user_id:
                cur.execute(
                    "SELECT display_name FROM users WHERE user_id = %s",
                    (resolved_user_id,),
                )
                name_row = cur.fetchone()
                if name_row is not None and name_row[0]:
                    name = str(name_row[0])

            cur.execute(
                """
                INSERT INTO user_stats (user_id, correct, total)
                VALUES (%s, 0, 0)
                ON CONFLICT (user_id) DO NOTHING
                """,
                (resolved_user_id,),
            )
            cur.execute(
                """
                UPDATE user_stats
                SET correct = correct + %s,
                    total = total + 1,
                    updated_at = now()
                WHERE user_id = %s
                RETURNING correct, total
                """,
                (correct_inc, resolved_user_id),
            )
            correct, total = cur.fetchone()
    correct_i = _coerce_int(correct)
    total_i = _coerce_int(total)
    return {
        "user_id": resolved_user_id,
        "name": name,
        "correct": correct_i,
        "total": total_i,
        "practice_history": [],
        "rate": _rate(correct_i, total_i),
    }


def save_user_snapshot(users: Dict[str, Dict[str, Any]]) -> None:
    with connect() as conn:
        with conn.cursor() as cur:
            for user_id, stats in users.items():
                if not isinstance(stats, dict):
                    continue
                display_name = str(stats.get("name") or user_id)
                correct = _coerce_int(stats.get("correct"))
                total = _coerce_int(stats.get("total"))
                cur.execute(
                    """
                    INSERT INTO users (user_id, display_name)
                    VALUES (%s, %s)
                    ON CONFLICT (user_id) DO UPDATE
                    SET display_name = EXCLUDED.display_name
                    """,
                    (str(user_id), display_name),
                )
                cur.execute(
                    """
                    INSERT INTO user_stats (user_id, correct, total)
                    VALUES (%s, %s, %s)
                    ON CONFLICT (user_id) DO UPDATE
                    SET correct = EXCLUDED.correct,
                        total = EXCLUDED.total,
                        updated_at = now()
                    """,
                    (str(user_id), correct, total),
                )


def increment_question_stats(bank_key: str, question_id: str, is_correct: bool) -> Dict[str, float]:
    correct_inc = 1 if is_correct else 0
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO question_stats (bank_key, question_id, total, correct, rate)
                VALUES (%s, %s, 0, 0, 0)
                ON CONFLICT (bank_key, question_id) DO NOTHING
                """,
                (bank_key, question_id),
            )
            cur.execute(
                """
                SELECT total, correct
                FROM question_stats
                WHERE bank_key = %s AND question_id = %s
                FOR UPDATE
                """,
                (bank_key, question_id),
            )
            total, correct = cur.fetchone()
            next_total = _coerce_int(total) + 1
            next_correct = _coerce_int(correct) + correct_inc
            next_rate = _rate(next_correct, next_total)
            cur.execute(
                """
                UPDATE question_stats
                SET total = %s,
                    correct = %s,
                    rate = %s,
                    updated_at = now()
                WHERE bank_key = %s AND question_id = %s
                """,
                (next_total, next_correct, next_rate, bank_key, question_id),
            )
    return {"total": next_total, "correct": next_correct, "rate": next_rate}


def upsert_question_bank(
    bank_key: str,
    name: str,
    color: str,
    source_file: str,
    metadata: Dict[str, Any],
    questions: Iterable[Dict[str, Any]],
) -> None:
    question_list = list(questions)
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO question_banks (bank_key, name, color, source_file, metadata)
                VALUES (%s, %s, %s, %s, %s)
                ON CONFLICT (bank_key) DO UPDATE
                SET name = EXCLUDED.name,
                    color = EXCLUDED.color,
                    source_file = EXCLUDED.source_file,
                    metadata = EXCLUDED.metadata,
                    updated_at = now()
                """,
                (bank_key, name, color, source_file, Jsonb(metadata or {})),
            )
            cur.execute("DELETE FROM bank_questions WHERE bank_key = %s", (bank_key,))
            for question in question_list:
                qid = str(question.get("id") or "")
                if not qid:
                    continue
                cur.execute(
                    """
                    INSERT INTO bank_questions (
                        bank_key, question_id, type, chapter, answer, payload
                    )
                    VALUES (%s, %s, %s, %s, %s, %s)
                    """,
                    (
                        bank_key,
                        qid,
                        str(question.get("type") or ""),
                        str(question.get("chapter") or ""),
                        Jsonb(question.get("answer")),
                        Jsonb(question),
                    ),
                )


def load_question_banks() -> Dict[str, Dict[str, Any]]:
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT bank_key, name, color, source_file, metadata
                FROM question_banks
                ORDER BY bank_key
                """
            )
            bank_rows = cur.fetchall()
            cur.execute(
                """
                SELECT bank_key, payload
                FROM bank_questions
                ORDER BY bank_key, question_id
                """
            )
            question_rows = cur.fetchall()

    banks: Dict[str, Dict[str, Any]] = {}
    for bank_key, name, color, source_file, metadata in bank_rows:
        key = str(bank_key)
        meta = metadata or {}
        if isinstance(meta, str):
            try:
                meta = json.loads(meta)
            except json.JSONDecodeError:
                meta = {}
        if not isinstance(meta, dict):
            meta = {}
        meta.setdefault("name", str(name or key))
        banks[key] = {
            "name": str(name or key),
            "color": str(color or ""),
            "source_file": str(source_file or f"postgresql:{key}"),
            "metadata": meta,
            "questions": [],
        }

    for bank_key, payload in question_rows:
        key = str(bank_key)
        question = payload
        if isinstance(payload, str):
            try:
                question = json.loads(payload)
            except json.JSONDecodeError:
                continue
        if not isinstance(question, dict):
            continue
        banks.setdefault(
            key,
            {
                "name": key,
                "color": "",
                "source_file": f"postgresql:{key}",
                "metadata": {"name": key},
                "questions": [],
            },
        )
        banks[key]["questions"].append(question)

    for key, bank in banks.items():
        bank["metadata"]["total"] = len(bank["questions"])

    return banks


def get_ranking(limit: Optional[int] = None) -> List[Dict[str, Any]]:
    with connect() as conn:
        with conn.cursor() as cur:
            if isinstance(limit, int) and limit > 0:
                query = """
                SELECT u.user_id, u.display_name, s.correct, s.total
                FROM user_stats s
                JOIN users u ON u.user_id = s.user_id
                WHERE s.total > 0
                ORDER BY s.correct DESC,
                         CASE WHEN s.total > 0 THEN s.correct::float / s.total ELSE 0 END DESC
                LIMIT %s
                """
                cur.execute(query, (limit,))
            else:
                query = """
                SELECT u.user_id, u.display_name, s.correct, s.total
                FROM user_stats s
                JOIN users u ON u.user_id = s.user_id
                WHERE s.total > 0
                ORDER BY s.correct DESC,
                         CASE WHEN s.total > 0 THEN s.correct::float / s.total ELSE 0 END DESC
                """
                cur.execute(query)
            rows = cur.fetchall()
    return [
        {
            "user_id": str(user_id),
            "name": str(display_name or user_id),
            "correct": _coerce_int(correct),
            "total": _coerce_int(total),
            "accuracy": _rate(_coerce_int(correct), _coerce_int(total)),
        }
        for user_id, display_name, correct, total in rows
    ]


def import_rankings_payload(payload: Dict[str, Any]) -> Dict[str, int]:
    users = payload.get("users", {}) if isinstance(payload, dict) else {}
    name_to_id = payload.get("name_to_id", {}) if isinstance(payload, dict) else {}
    imported = 0
    with connect() as conn:
        with conn.cursor() as cur:
            for user_id, stats in users.items():
                if not isinstance(stats, dict):
                    continue
                display_name = str(stats.get("name") or user_id)
                cur.execute(
                    """
                    INSERT INTO users (user_id, display_name)
                    VALUES (%s, %s)
                    ON CONFLICT (user_id) DO UPDATE
                    SET display_name = EXCLUDED.display_name
                    """,
                    (str(user_id), display_name),
                )
                correct = _coerce_int(stats.get("correct"))
                total = _coerce_int(stats.get("total"))
                cur.execute(
                    """
                    INSERT INTO user_stats (user_id, correct, total)
                    VALUES (%s, %s, %s)
                    ON CONFLICT (user_id) DO UPDATE
                    SET correct = EXCLUDED.correct,
                        total = EXCLUDED.total,
                        updated_at = now()
                    """,
                    (str(user_id), correct, total),
                )
                imported += 1

            for display_name, user_id in name_to_id.items():
                if not display_name or not user_id:
                    continue
                cur.execute(
                    """
                    UPDATE users
                    SET display_name = %s
                    WHERE user_id = %s
                    """,
                    (str(display_name), str(user_id)),
                )
    return {"users": imported, "name_to_id": len(name_to_id)}


def import_question_stats_payload(payload: Dict[str, Any]) -> Dict[str, int]:
    imported = 0
    with connect() as conn:
        with conn.cursor() as cur:
            for bank_key, stats in payload.items():
                if not isinstance(stats, dict):
                    continue
                for question_id, stat in stats.items():
                    if not isinstance(stat, dict):
                        continue
                    total = _coerce_int(stat.get("total"))
                    correct = _coerce_int(stat.get("correct"))
                    rate = float(stat.get("rate") or _rate(correct, total))
                    cur.execute(
                        """
                        INSERT INTO question_stats (bank_key, question_id, total, correct, rate)
                        VALUES (%s, %s, %s, %s, %s)
                        ON CONFLICT (bank_key, question_id) DO UPDATE
                        SET total = EXCLUDED.total,
                            correct = EXCLUDED.correct,
                            rate = EXCLUDED.rate,
                            updated_at = now()
                        """,
                        (str(bank_key), str(question_id), total, correct, rate),
                    )
                    imported += 1
    return {"question_stats": imported}


def bank_consistency_snapshot() -> Dict[str, Dict[str, Any]]:
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT qb.bank_key,
                       qb.name,
                       COUNT(bq.question_id) AS total
                FROM question_banks qb
                LEFT JOIN bank_questions bq ON bq.bank_key = qb.bank_key
                GROUP BY qb.bank_key, qb.name
                """
            )
            bank_rows = cur.fetchall()
            cur.execute(
                """
                SELECT bank_key, question_id, answer
                FROM bank_questions
                ORDER BY bank_key, question_id
                """
            )
            question_rows = cur.fetchall()

    snapshot: Dict[str, Dict[str, Any]] = {
        str(bank_key): {"name": str(name), "total": _coerce_int(total), "questions": {}}
        for bank_key, name, total in bank_rows
    }
    for bank_key, question_id, answer in question_rows:
        parsed_answer = answer
        if isinstance(answer, str):
            try:
                parsed_answer = json.loads(answer)
            except json.JSONDecodeError:
                parsed_answer = answer
        snapshot.setdefault(str(bank_key), {"name": str(bank_key), "total": 0, "questions": {}})
        snapshot[str(bank_key)]["questions"][str(question_id)] = parsed_answer
    return snapshot


def list_food_wheel_items(is_public: bool = True) -> List[Dict[str, Any]]:
    with connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT
                    fw.id,
                    fw.owner_user_id,
                    COALESCE(NULLIF(fw.owner_name, ''), u.display_name, fw.owner_user_id) AS owner_name,
                    fw.items,
                    fw.is_public,
                    fw.created_at,
                    fw.updated_at
                FROM food_wheel_items fw
                LEFT JOIN users u ON u.user_id = fw.owner_user_id
                WHERE fw.is_public = %s
                ORDER BY fw.updated_at DESC, fw.id DESC
                """,
                (is_public,),
            )
            rows = cur.fetchall()

    wheels: List[Dict[str, Any]] = []
    for row in rows:
        wheel_id, owner_user_id, owner_name, items, public_flag, created_at, updated_at = row
        normalized_items = _normalize_food_wheel_items(items)
        wheels.append(
            {
                "id": int(wheel_id),
                "owner_user_id": str(owner_user_id),
                "owner_name": str(owner_name or owner_user_id),
                "items": normalized_items,
                "is_public": bool(public_flag),
                "created_at": str(created_at),
                "updated_at": str(updated_at),
            }
        )
    return wheels


def upsert_food_wheel_items(owner_user_id: str, items: Iterable[str]) -> Dict[str, Any]:
    normalized = _normalize_food_wheel_items(list(items))
    owner = str(owner_user_id).strip()
    owner_name = ""

    with connect() as conn:
        with conn.cursor() as cur:
            if owner:
                cur.execute(
                    "SELECT display_name FROM users WHERE user_id = %s",
                    (owner,),
                )
                user_row = cur.fetchone()
                if user_row is not None:
                    owner_name = str(user_row[0] or "").strip()

            cur.execute(
                """
                INSERT INTO food_wheel_items (
                    owner_user_id,
                    owner_name,
                    items,
                    is_public
                )
                VALUES (%s, %s, %s, TRUE)
                ON CONFLICT (owner_user_id) DO UPDATE
                SET owner_name = CASE
                        WHEN NULLIF(EXCLUDED.owner_name, '') = ''
                        THEN food_wheel_items.owner_name
                        ELSE EXCLUDED.owner_name
                    END,
                    items = EXCLUDED.items,
                    is_public = TRUE,
                    updated_at = now()
                RETURNING
                    id,
                    owner_user_id,
                    owner_name,
                    items,
                    is_public,
                    created_at,
                    updated_at
                """,
                (owner, owner_name, Jsonb(normalized)),
            )
            row = cur.fetchone()

    if row is None:
        raise RuntimeError("保存美食转盘失败")

    wheel_id, wheel_owner, wheel_owner_name, raw_items, public_flag, created_at, updated_at = row
    normalized_items = _normalize_food_wheel_items(raw_items)
    return {
        "id": int(wheel_id),
        "owner_user_id": str(wheel_owner),
        "owner_name": str(wheel_owner_name or wheel_owner),
        "items": normalized_items,
        "is_public": bool(public_flag),
        "created_at": str(created_at),
        "updated_at": str(updated_at),
    }
