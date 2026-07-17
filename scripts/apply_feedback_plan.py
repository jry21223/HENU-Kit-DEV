#!/usr/bin/env python3
"""Apply a DeepSeek repair plan to QuizCraft database.

Usage:
  python scripts/apply_feedback_plan.py --plan /tmp/plan.json
  python scripts/apply_feedback_plan.py --plan /tmp/plan.json --set-feedback-status --yes
"""
import argparse
import json
import os
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

import psycopg
from psycopg.types.json import Jsonb

DEFAULT_ENV_FILE = "/etc/quizcraft-cn.env"
MIN_AUTO_FIX_CONFIDENCE = 0.75
VALID_VERDICTS = {"fix_needed", "no_change", "needs_human_review"}
VALID_TYPES = {"single", "multi", "judge", "blank"}
PATCH_FIELDS = {"answer", "type", "payload"}
SAFE_NAME_RE = re.compile(r"[^A-Za-z0-9_.-]+")


class PlanError(ValueError):
    pass


def _parse_env_line(line: str):
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        return None, None
    key, val = line.split("=", 1)
    return key.strip(), val.strip().strip("\"'")


def load_env_file(path: str) -> dict:
    env = {}
    p = Path(path)
    if not p.exists():
        return env
    for line in p.read_text(encoding="utf-8").splitlines():
        k, v = _parse_env_line(line)
        if k:
            env[k] = v
    return env


def _fail(message: str) -> None:
    print(f"ERROR: {message}", file=sys.stderr)
    sys.exit(1)


def _safe_name(value: Any, fallback: str = "unknown") -> str:
    text = str(value or "").strip() or fallback
    text = SAFE_NAME_RE.sub("_", text).strip("._-")
    return text or fallback


def _safe_backup_path(backup_dir: Path, *, bank_key: str, question_id: str, feedback_id: Any) -> Path:
    ts = datetime.now().strftime("%Y%m%d-%H%M%S")
    filename = (
        f"{_safe_name(bank_key, 'bank')}-"
        f"{_safe_name(question_id, 'question')}-"
        f"feedback-{_safe_name(feedback_id, 'none')}-{ts}.json"
    )
    backup_dir.mkdir(parents=True, exist_ok=True)
    backup_root = backup_dir.resolve()
    backup_path = (backup_dir / filename).resolve()
    try:
        backup_path.relative_to(backup_root)
    except ValueError as exc:
        raise PlanError(f"unsafe backup path: {backup_path}") from exc
    return backup_path


def _normalize_json(value: Any) -> Any:
    """Round-trip through JSON so Jsonb values and plan values compare consistently."""
    return json.loads(json.dumps(value, ensure_ascii=False, sort_keys=True, default=str))


def _json_equal(left: Any, right: Any) -> bool:
    return _normalize_json(left) == _normalize_json(right)


def _validate_plan(plan: dict) -> dict:
    if not isinstance(plan, dict):
        raise PlanError("plan must be a JSON object")

    source = plan.get("_source")
    if not isinstance(source, dict):
        raise PlanError("plan._source must be an object")

    bank_key = source.get("bank_key")
    question_id = source.get("question_id")
    feedback_id = source.get("feedback_id")
    if not bank_key or not question_id:
        raise PlanError("plan._source.bank_key and question_id are required")

    verdict = plan.get("verdict", "unknown")
    if verdict not in VALID_VERDICTS:
        raise PlanError(f"invalid verdict: {verdict!r}; expected one of {sorted(VALID_VERDICTS)}")

    confidence = plan.get("confidence", 0)
    if isinstance(confidence, bool) or not isinstance(confidence, (int, float)) or not 0 <= float(confidence) <= 1:
        raise PlanError("confidence must be a number between 0 and 1")
    confidence = float(confidence)

    if verdict == "fix_needed" and confidence < MIN_AUTO_FIX_CONFIDENCE:
        raise PlanError(
            f"fix_needed confidence {confidence:.2f} is below auto-fix threshold "
            f"{MIN_AUTO_FIX_CONFIDENCE:.2f}; use needs_human_review instead"
        )

    db_patch = plan.get("db_patch", {})
    if db_patch is None:
        db_patch = {}
    if not isinstance(db_patch, dict):
        raise PlanError("db_patch must be an object")
    unknown_fields = set(db_patch) - PATCH_FIELDS
    if unknown_fields:
        raise PlanError(f"db_patch contains unsupported fields: {sorted(unknown_fields)}")

    new_type = db_patch.get("type")
    if new_type is not None and new_type not in VALID_TYPES:
        raise PlanError(f"invalid db_patch.type: {new_type!r}")

    new_payload = db_patch.get("payload")
    if new_payload is not None and not isinstance(new_payload, dict):
        raise PlanError("db_patch.payload must be a full object when provided")

    if new_payload is not None:
        payload_type = new_payload.get("type")
        if payload_type is not None and payload_type not in VALID_TYPES:
            raise PlanError(f"invalid db_patch.payload.type: {payload_type!r}")
        if db_patch.get("answer") is not None and "answer" in new_payload:
            if not _json_equal(db_patch["answer"], new_payload["answer"]):
                raise PlanError("db_patch.answer and db_patch.payload.answer must match when both are provided")
        if new_type is not None and payload_type is not None and new_type != payload_type:
            raise PlanError("db_patch.type and db_patch.payload.type must match when both are provided")

    if verdict != "fix_needed" and any(db_patch.get(field) is not None for field in PATCH_FIELDS):
        raise PlanError("db_patch may only contain changes when verdict is fix_needed")

    return {
        "source": source,
        "bank_key": str(bank_key),
        "question_id": str(question_id),
        "feedback_id": feedback_id,
        "verdict": verdict,
        "confidence": confidence,
        "db_patch": db_patch,
    }


def _expected_after_patch(old_answer: Any, old_type: Any, old_payload: Any, db_patch: dict) -> tuple[Any, Any, Any]:
    new_payload = db_patch.get("payload")
    new_answer = db_patch.get("answer")
    new_type = db_patch.get("type")

    expected_payload = new_payload if new_payload is not None else old_payload
    expected_answer = old_answer
    expected_type = old_type

    if new_payload is not None:
        if new_answer is None and "answer" in new_payload:
            expected_answer = new_payload["answer"]
        if new_type is None and "type" in new_payload:
            expected_type = new_payload["type"]

    if new_answer is not None:
        expected_answer = new_answer
    if new_type is not None:
        expected_type = new_type

    return expected_answer, expected_type, expected_payload


def _assert_verified(verify: tuple[Any, Any, Any] | None, expected: tuple[Any, Any, Any]) -> None:
    if not verify:
        raise PlanError("verification query returned no row")

    actual_answer, actual_type, actual_payload = verify
    expected_answer, expected_type, expected_payload = expected

    mismatches = []
    if not _json_equal(actual_answer, expected_answer):
        mismatches.append("answer")
    if actual_type != expected_type:
        mismatches.append("type")
    if not _json_equal(actual_payload, expected_payload):
        mismatches.append("payload")

    if mismatches:
        raise PlanError(f"verification failed for fields: {', '.join(mismatches)}")


def _assert_feedback_matches_plan(cur, feedback_id: Any, bank_key: str, question_id: str) -> None:
    if feedback_id is None:
        return

    cur.execute(
        "SELECT question_bank, question_id, status FROM feedbacks WHERE feedback_id=%s",
        (feedback_id,),
    )
    row = cur.fetchone()
    if not row:
        raise PlanError(f"feedback {feedback_id} not found")

    feedback_bank, feedback_question_id, _status = row
    if str(feedback_bank) != str(bank_key) or str(feedback_question_id) != str(question_id):
        raise PlanError(
            "plan target does not match feedback target: "
            f"plan={bank_key}/{question_id}, feedback={feedback_bank}/{feedback_question_id}"
        )


def _build_resolution_note(plan: dict) -> str:
    resolution_note = plan.get("resolution_note", "")
    if resolution_note:
        return str(resolution_note)

    changed = plan.get("changed_fields", [])
    if changed:
        parts = [
            f"{c.get('path')}: {c.get('before')} -> {c.get('after')}"
            for c in changed
            if isinstance(c, dict)
        ]
        return "已修复: " + "; ".join(parts)
    return "DeepSeek审题: 无需修改"


def _resolve_feedback_in_transaction(cur, feedback_id: int, plan: dict, *, verdict: str, applied_patch: bool) -> None:
    if verdict == "needs_human_review":
        print(f"skip feedback {feedback_id}: needs_human_review cannot be auto-resolved")
        return
    if verdict == "fix_needed" and not applied_patch:
        raise PlanError("cannot resolve fix_needed feedback before a verified DB patch is applied")
    if verdict != "no_change" and not applied_patch:
        raise PlanError(f"cannot auto-resolve feedback for verdict={verdict!r}")

    cur.execute(
        "UPDATE feedbacks SET status='resolved', resolved_at=now(), resolution_note=%s WHERE feedback_id=%s",
        (_build_resolution_note(plan), feedback_id),
    )
    if cur.rowcount != 1:
        raise PlanError(f"feedback {feedback_id} update affected {cur.rowcount} rows")
    print(f"feedback {feedback_id} -> resolved")


def main():
    parser = argparse.ArgumentParser(description="Apply DeepSeek repair plan")
    parser.add_argument("--plan", required=True, help="Path to plan JSON")
    parser.add_argument("--env-file", default=DEFAULT_ENV_FILE)
    parser.add_argument("--set-feedback-status", action="store_true", help="Update feedback status")
    parser.add_argument("--yes", action="store_true", help="Actually write changes (default: dry-run)")
    args = parser.parse_args()

    try:
        plan = json.loads(Path(args.plan).read_text(encoding="utf-8"))
        parsed = _validate_plan(plan)
    except (OSError, json.JSONDecodeError, PlanError) as exc:
        _fail(str(exc))

    env = load_env_file(args.env_file)
    db_url = env.get("DATABASE_URL") or os.environ.get("DATABASE_URL")
    if not db_url:
        _fail("DATABASE_URL not set")

    bank_key = parsed["bank_key"]
    question_id = parsed["question_id"]
    feedback_id = parsed["feedback_id"]
    verdict = parsed["verdict"]
    confidence = parsed["confidence"]
    db_patch = parsed["db_patch"]

    print(f"target: {bank_key}/{question_id}")
    print(f"feedback_id: {feedback_id}")
    print(f"verdict: {verdict} confidence={confidence}")
    print("source_of_truth: db")

    new_answer = db_patch.get("answer")
    new_type = db_patch.get("type")
    new_payload = db_patch.get("payload")

    will_update_answer = new_answer is not None
    will_update_type = new_type is not None
    will_update_payload = new_payload is not None

    print(f"will_update: answer={will_update_answer} type={will_update_type} payload={will_update_payload}")

    if verdict == "needs_human_review":
        print("needs_human_review: no database changes will be applied")
        return

    if verdict == "fix_needed" and not (will_update_answer or will_update_type or will_update_payload):
        _fail("verdict is fix_needed but db_patch is empty")

    if not args.yes:
        if not (will_update_answer or will_update_type or will_update_payload):
            print("dry-run: no PostgreSQL changes written")
            if args.set_feedback_status and feedback_id:
                print(f"dry-run: would set feedback {feedback_id} to resolved")
            return
        print("dry-run: no PostgreSQL changes written; pass --yes to apply")
        return

    backup_dir = Path("/opt/quizcraft-cn/repair_backups/db")
    backup_path = _safe_backup_path(
        backup_dir,
        bank_key=bank_key,
        question_id=question_id,
        feedback_id=feedback_id,
    )

    try:
        with psycopg.connect(db_url) as conn:
            with conn.cursor() as cur:
                _assert_feedback_matches_plan(cur, feedback_id, bank_key, question_id)

                if not (will_update_answer or will_update_type or will_update_payload):
                    print("nothing to update")
                    if args.set_feedback_status and feedback_id:
                        _resolve_feedback_in_transaction(
                            cur,
                            feedback_id,
                            plan,
                            verdict=verdict,
                            applied_patch=False,
                        )
                    conn.commit()
                    return

                cur.execute(
                    "SELECT answer, type, payload FROM bank_questions WHERE bank_key=%s AND question_id=%s",
                    (bank_key, question_id),
                )
                row = cur.fetchone()
                if not row:
                    raise PlanError("question not found")

                old_answer, old_type, old_payload = row
                backup_data = {
                    "answer": old_answer,
                    "type": old_type,
                    "payload": old_payload,
                    "plan": plan,
                }
                backup_path.write_text(json.dumps(backup_data, ensure_ascii=False, indent=2, default=str), encoding="utf-8")
                print(f"backup: {backup_path}")

                if will_update_payload:
                    cur.execute(
                        "UPDATE bank_questions SET payload=%s WHERE bank_key=%s AND question_id=%s",
                        (Jsonb(new_payload), bank_key, question_id),
                    )
                    if not will_update_answer and "answer" in new_payload:
                        cur.execute(
                            "UPDATE bank_questions SET answer=%s::jsonb WHERE bank_key=%s AND question_id=%s",
                            (json.dumps(new_payload["answer"], ensure_ascii=False), bank_key, question_id),
                        )
                    if not will_update_type and "type" in new_payload:
                        cur.execute(
                            "UPDATE bank_questions SET type=%s WHERE bank_key=%s AND question_id=%s",
                            (new_payload["type"], bank_key, question_id),
                        )

                if will_update_answer:
                    cur.execute(
                        "UPDATE bank_questions SET answer=%s::jsonb WHERE bank_key=%s AND question_id=%s",
                        (json.dumps(new_answer, ensure_ascii=False), bank_key, question_id),
                    )

                if will_update_type:
                    cur.execute(
                        "UPDATE bank_questions SET type=%s WHERE bank_key=%s AND question_id=%s",
                        (new_type, bank_key, question_id),
                    )

                cur.execute(
                    "SELECT answer, type, payload FROM bank_questions WHERE bank_key=%s AND question_id=%s",
                    (bank_key, question_id),
                )
                verify = cur.fetchone()
                expected = _expected_after_patch(old_answer, old_type, old_payload, db_patch)
                _assert_verified(verify, expected)

                if args.set_feedback_status and feedback_id:
                    _resolve_feedback_in_transaction(
                        cur,
                        feedback_id,
                        plan,
                        verdict=verdict,
                        applied_patch=True,
                    )

                conn.commit()
                print("verified: bank_questions answer/type/payload match plan")
                print("applied")
    except PlanError as exc:
        _fail(str(exc))


if __name__ == "__main__":
    main()
