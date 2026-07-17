#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Audit QuizCraft bank questions with DeepSeek in parallel.

The script is intentionally read-only: it never rewrites bank JSON files.
It emits one JSON object per question so long runs can be resumed safely.

Examples:
  DEEPSEEK_API_KEYS="sk-xxx,sk-yyy" python3 scripts/audit_bank_via_deepseek.py
  python3 scripts/audit_bank_via_deepseek.py tiku/sixiu.json --key-file /secure/deepseek_keys.txt
  python3 scripts/audit_bank_via_deepseek.py --limit 20 --workers-per-key 1 --verbose
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import re
import sys
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Iterable


PROJECT_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MODEL = "deepseek-chat"
DEFAULT_API_URL = "https://api.deepseek.com/chat/completions"
LETTERS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"


SYSTEM_PROMPT = """你是一个严谨的中文高校考试题库质检员。
请只根据题干、选项、题型、当前答案和解析判断这道题是否存在问题。
重点检查：
1. 题型是否正确：single 单选、multi 多选、judge 判断。
2. 当前答案是否与题干和选项一致。
3. 解析是否与当前答案一致。
4. 选项是否重复、缺失、错别字严重、或题干残留广告/无关文本。
5. 如果没有充分证据证明有错，不要臆造问题。

必须只返回一个 JSON 对象，不要 Markdown，不要解释 JSON 之外的文字。
JSON schema:
{
  "status": "ok | suspect | invalid",
  "severity": "none | low | medium | high",
  "issue_types": ["answer_mismatch | type_mismatch | analysis_mismatch | option_error | stem_error | typo | duplicate_option | format_error | stale_or_ambiguous"],
  "recommended_type": "single | multi | judge | null",
  "recommended_answer": "A/B/C/ABCD/对/错/null",
  "confidence": 0.0,
  "reason": "不超过120字，说明判断依据",
  "suggested_fix": "不超过160字，若无需修改则为空字符串"
}
"""


@dataclass(frozen=True)
class AuditTask:
    bank: str
    bank_path: str
    question_index: int
    question: dict[str, Any]
    key_index: int


def load_env_file(path: Path | None) -> None:
    if path is None or not path.exists():
        return
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[len("export ") :].strip()
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip("\"'")
        if key and key not in os.environ:
            os.environ[key] = value


def split_keys(value: str) -> list[str]:
    keys: list[str] = []
    for item in re.split(r"[\s,;]+", value):
        item = item.strip()
        if item:
            keys.append(item)
    return keys


def load_api_keys(args: argparse.Namespace) -> list[str]:
    keys: list[str] = []
    for value in args.api_key or []:
        keys.extend(split_keys(value))

    if args.key_file:
        key_file = args.key_file.expanduser()
        if not key_file.exists():
            raise SystemExit(f"Key file not found: {key_file}")
        for raw_line in key_file.read_text(encoding="utf-8").splitlines():
            line = raw_line.strip()
            if not line or line.startswith("#"):
                continue
            keys.extend(split_keys(line))

    for env_name in (args.keys_env, "DEEPSEEK_API_KEYS", "DEEPSEEK_API_KEY", "LLM_API_KEY"):
        value = os.getenv(env_name)
        if value:
            keys.extend(split_keys(value))

    unique: list[str] = []
    seen: set[str] = set()
    for key in keys:
        if key not in seen:
            unique.append(key)
            seen.add(key)
    if not unique:
        raise SystemExit(
            "Missing DeepSeek keys. Set DEEPSEEK_API_KEYS, pass --api-key, or pass --key-file."
        )
    return unique


def load_bank(path: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(data, list):
        return {"questions": data}, data
    if isinstance(data, dict) and isinstance(data.get("questions"), list):
        return data, data["questions"]
    raise SystemExit(f"Unsupported bank JSON shape: {path}")


def discover_bank_paths(paths: list[Path]) -> list[Path]:
    if paths:
        return [path.expanduser() for path in paths]
    tiku_dir = PROJECT_ROOT / "tiku"
    if not tiku_dir.exists():
        raise SystemExit("No bank path provided and tiku/ does not exist.")
    return sorted(path for path in tiku_dir.glob("*.json") if path.is_file())


def get_question_text(question: dict[str, Any]) -> str:
    for key in ("content", "question", "title", "stem", "text", "question_text"):
        value = question.get(key)
        if value:
            return str(value)
    return ""


def get_options(question: dict[str, Any]) -> list[Any]:
    options = question.get("options") or question.get("choices") or []
    if isinstance(options, dict):
        return [options[key] for key in sorted(options)]
    if isinstance(options, list):
        return options
    return []


def normalize_answer(question: dict[str, Any]) -> str:
    answer = question.get("answer")
    if answer is None:
        answer = question.get("correct_answer")

    qtype = str(question.get("type") or question.get("question_type") or "").lower()
    if qtype in {"judge", "true_false", "boolean"}:
        if isinstance(answer, bool):
            return "对" if answer else "错"
        text = str(answer).strip().lower()
        if text in {"true", "t", "1", "yes", "对", "正确", "是"}:
            return "对"
        if text in {"false", "f", "0", "no", "错", "错误", "否"}:
            return "错"
        return str(answer)

    values = answer if isinstance(answer, list) else [answer]
    normalized: list[str] = []
    options = get_options(question)
    for value in values:
        if isinstance(value, int):
            normalized.append(LETTERS[value] if 0 <= value < len(LETTERS) else str(value))
            continue
        text = str(value).strip()
        if re.fullmatch(r"[A-Za-z]", text):
            normalized.append(text.upper())
        elif text.isdigit():
            index = int(text)
            if 0 <= index < len(LETTERS):
                normalized.append(LETTERS[index])
            elif 1 <= index <= len(LETTERS):
                normalized.append(LETTERS[index - 1])
            else:
                normalized.append(text)
        elif text in options:
            normalized.append(LETTERS[options.index(text)])
        else:
            normalized.append(text)

    if str(question.get("type") or "").lower() == "multi":
        return "".join(sorted(set(normalized), key=lambda item: LETTERS.find(item) if item in LETTERS else 99))
    return normalized[0] if normalized else ""


def question_identity(bank: str, index: int, question: dict[str, Any]) -> str:
    qid = str(question.get("id") or question.get("question_id") or question.get("number") or "")
    return f"{bank}:{index}:{qid}"


def load_completed(output_path: Path, retry_errors: bool) -> set[str]:
    completed: set[str] = set()
    if not output_path.exists():
        return completed
    for line in output_path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            item = json.loads(line)
        except json.JSONDecodeError:
            continue
        identity = str(item.get("identity") or "")
        if not identity:
            continue
        if retry_errors and item.get("status") == "error":
            continue
        completed.add(identity)
    return completed


def build_tasks(paths: list[Path], completed: set[str], limit: int | None, keys_count: int) -> list[AuditTask]:
    tasks: list[AuditTask] = []
    for path in paths:
        _, questions = load_bank(path)
        bank = path.stem
        for offset, question in enumerate(questions, start=1):
            if not isinstance(question, dict):
                continue
            identity = question_identity(bank, offset, question)
            if identity in completed:
                continue
            tasks.append(
                AuditTask(
                    bank=bank,
                    bank_path=str(path),
                    question_index=offset,
                    question=question,
                    key_index=len(tasks) % keys_count,
                )
            )
            if limit is not None and len(tasks) >= limit:
                return tasks
    return tasks


def build_user_prompt(task: AuditTask) -> str:
    question = task.question
    options = get_options(question)
    option_lines = []
    for index, option in enumerate(options):
        label = LETTERS[index] if index < len(LETTERS) else str(index + 1)
        option_lines.append(f"{label}. {option}")

    payload = {
        "bank": task.bank,
        "question_index": task.question_index,
        "id": question.get("id") or question.get("question_id"),
        "number": question.get("number"),
        "type": question.get("type") or question.get("question_type"),
        "stem": get_question_text(question),
        "options": option_lines,
        "current_answer": normalize_answer(question),
        "analysis": str(question.get("analysis") or ""),
    }
    return json.dumps(payload, ensure_ascii=False, indent=2)


def extract_json_object(text: str) -> dict[str, Any]:
    stripped = text.strip()
    if stripped.startswith("```"):
        stripped = re.sub(r"^```(?:json)?\s*", "", stripped)
        stripped = re.sub(r"\s*```$", "", stripped)
    try:
        value = json.loads(stripped)
    except json.JSONDecodeError:
        start = stripped.find("{")
        end = stripped.rfind("}")
        if start < 0 or end <= start:
            raise
        value = json.loads(stripped[start : end + 1])
    if not isinstance(value, dict):
        raise ValueError("model did not return a JSON object")
    return value


def normalize_audit_result(value: dict[str, Any]) -> dict[str, Any]:
    status = str(value.get("status") or "suspect").strip().lower()
    if status not in {"ok", "suspect", "invalid"}:
        status = "suspect"

    severity = str(value.get("severity") or ("none" if status == "ok" else "medium")).strip().lower()
    if severity not in {"none", "low", "medium", "high"}:
        severity = "medium"

    issue_types = value.get("issue_types")
    if not isinstance(issue_types, list):
        issue_types = []
    issue_types = [str(item).strip() for item in issue_types if str(item).strip()]

    confidence = value.get("confidence", 0)
    try:
        confidence_float = float(confidence)
    except (TypeError, ValueError):
        confidence_float = 0.0
    confidence_float = max(0.0, min(1.0, confidence_float))

    return {
        "audit_status": status,
        "severity": severity,
        "issue_types": issue_types,
        "recommended_type": value.get("recommended_type"),
        "recommended_answer": value.get("recommended_answer"),
        "confidence": confidence_float,
        "reason": str(value.get("reason") or "").strip(),
        "suggested_fix": str(value.get("suggested_fix") or "").strip(),
    }


def post_deepseek(
    api_url: str,
    api_key: str,
    model: str,
    messages: list[dict[str, str]],
    timeout: int,
    temperature: float,
) -> tuple[dict[str, Any], str]:
    body = json.dumps(
        {
            "model": model,
            "messages": messages,
            "temperature": temperature,
            "response_format": {"type": "json_object"},
        },
        ensure_ascii=False,
    ).encode("utf-8")
    request = urllib.request.Request(
        api_url,
        data=body,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json; charset=utf-8",
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        raw = response.read().decode("utf-8")
    data = json.loads(raw)
    content = data["choices"][0]["message"]["content"]
    return data, str(content)


def audit_one(
    task: AuditTask,
    keys: list[str],
    semaphores: list[threading.BoundedSemaphore],
    args: argparse.Namespace,
) -> dict[str, Any]:
    identity = question_identity(task.bank, task.question_index, task.question)
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": build_user_prompt(task)},
    ]
    api_key = keys[task.key_index]
    started = time.time()
    last_error = ""

    for attempt in range(1, args.retries + 2):
        try:
            with semaphores[task.key_index]:
                if args.per_key_delay > 0:
                    time.sleep(args.per_key_delay)
                response, content = post_deepseek(
                    args.api_url,
                    api_key,
                    args.model,
                    messages,
                    timeout=args.timeout,
                    temperature=args.temperature,
                )
            parsed = extract_json_object(content)
            audit = normalize_audit_result(parsed)
            usage = response.get("usage") if isinstance(response, dict) else None
            return {
                "identity": identity,
                "status": "done",
                "created_at": datetime.now().astimezone().isoformat(),
                "bank": task.bank,
                "bank_path": task.bank_path,
                "question_index": task.question_index,
                "question_id": task.question.get("id") or task.question.get("question_id"),
                "number": task.question.get("number"),
                "type": task.question.get("type") or task.question.get("question_type"),
                "answer": normalize_answer(task.question),
                "stem_preview": get_question_text(task.question)[:120],
                "model": args.model,
                "key_index": task.key_index,
                "latency_sec": round(time.time() - started, 3),
                "usage": usage,
                **audit,
            }
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode("utf-8", errors="replace")
            last_error = f"HTTP {exc.code}: {detail[:500]}"
            if exc.code in {400, 401, 403}:
                break
        except Exception as exc:  # noqa: BLE001 - long batch scripts should record per-item failures
            last_error = f"{type(exc).__name__}: {exc}"

        if attempt <= args.retries:
            time.sleep(min(args.retry_backoff * attempt, 30))

    return {
        "identity": identity,
        "status": "error",
        "created_at": datetime.now().astimezone().isoformat(),
        "bank": task.bank,
        "bank_path": task.bank_path,
        "question_index": task.question_index,
        "question_id": task.question.get("id") or task.question.get("question_id"),
        "number": task.question.get("number"),
        "type": task.question.get("type") or task.question.get("question_type"),
        "answer": normalize_answer(task.question),
        "stem_preview": get_question_text(task.question)[:120],
        "model": args.model,
        "key_index": task.key_index,
        "latency_sec": round(time.time() - started, 3),
        "error": last_error,
    }


def append_jsonl(path: Path, lock: threading.Lock, item: dict[str, Any]) -> None:
    line = json.dumps(item, ensure_ascii=False, sort_keys=True)
    with lock:
        with path.open("a", encoding="utf-8") as output:
            output.write(line + "\n")
            output.flush()


def summarize_output(path: Path) -> dict[str, Any]:
    summary: dict[str, Any] = {
        "total": 0,
        "done": 0,
        "error": 0,
        "ok": 0,
        "suspect": 0,
        "invalid": 0,
        "high": 0,
        "medium": 0,
        "low": 0,
    }
    if not path.exists():
        return summary
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        try:
            item = json.loads(line)
        except json.JSONDecodeError:
            continue
        summary["total"] += 1
        status = item.get("status")
        if status == "error":
            summary["error"] += 1
            continue
        summary["done"] += 1
        audit_status = str(item.get("audit_status") or "")
        if audit_status in {"ok", "suspect", "invalid"}:
            summary[audit_status] += 1
        severity = str(item.get("severity") or "")
        if severity in {"high", "medium", "low"}:
            summary[severity] += 1
    return summary


def parse_args(argv: Iterable[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Audit QuizCraft question banks with parallel DeepSeek calls.")
    parser.add_argument("bank_json", nargs="*", type=Path, help="Bank JSON files. Defaults to tiku/*.json.")
    parser.add_argument("--env", type=Path, default=PROJECT_ROOT / "scripts" / ".env", help="Optional env file.")
    parser.add_argument("--api-key", action="append", help="DeepSeek API key. Can be repeated; env is preferred.")
    parser.add_argument("--key-file", type=Path, help="Text file containing one or more DeepSeek API keys.")
    parser.add_argument("--keys-env", default="DEEPSEEK_API_KEYS", help="Env var containing comma/newline-separated keys.")
    parser.add_argument("--api-url", default=os.getenv("DEEPSEEK_API_URL") or DEFAULT_API_URL)
    parser.add_argument("--model", default=os.getenv("DEEPSEEK_MODEL") or DEFAULT_MODEL)
    parser.add_argument("--output", type=Path, help="JSONL output path.")
    parser.add_argument("--workers-per-key", type=int, default=1, help="Concurrent workers for each key.")
    parser.add_argument("--limit", type=int, help="Audit only the first N pending questions.")
    parser.add_argument("--resume", action="store_true", default=True, help="Skip identities already present in output.")
    parser.add_argument("--no-resume", dest="resume", action="store_false", help="Do not skip existing output entries.")
    parser.add_argument("--retry-errors", action="store_true", help="When resuming, retry prior status=error rows.")
    parser.add_argument("--retries", type=int, default=2)
    parser.add_argument("--retry-backoff", type=float, default=2.0)
    parser.add_argument("--timeout", type=int, default=90)
    parser.add_argument("--temperature", type=float, default=0.0)
    parser.add_argument("--per-key-delay", type=float, default=0.0, help="Sleep before each request for a key.")
    parser.add_argument("--verbose", action="store_true")
    return parser.parse_args(list(argv))


def main(argv: Iterable[str] = sys.argv[1:]) -> int:
    args = parse_args(argv)
    load_env_file(args.env.expanduser() if args.env else None)

    if args.workers_per_key <= 0:
        raise SystemExit("--workers-per-key must be positive.")
    if args.retries < 0:
        raise SystemExit("--retries must be >= 0.")

    keys = load_api_keys(args)
    bank_paths = discover_bank_paths(args.bank_json)
    missing = [str(path) for path in bank_paths if not path.exists()]
    if missing:
        raise SystemExit("Bank file not found: " + ", ".join(missing))

    output_path = args.output
    if output_path is None:
        stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
        output_path = PROJECT_ROOT / "audit_reports" / f"deepseek_bank_audit_{stamp}.jsonl"
    output_path = output_path.expanduser()
    output_path.parent.mkdir(parents=True, exist_ok=True)

    completed = load_completed(output_path, args.retry_errors) if args.resume else set()
    tasks = build_tasks(bank_paths, completed, args.limit, len(keys))

    print(
        json.dumps(
            {
                "banks": [str(path) for path in bank_paths],
                "keys": len(keys),
                "workers": len(keys) * args.workers_per_key,
                "resume_skipped": len(completed),
                "pending": len(tasks),
                "output": str(output_path),
                "model": args.model,
            },
            ensure_ascii=False,
        )
    )
    if not tasks:
        print(json.dumps({"summary": summarize_output(output_path)}, ensure_ascii=False))
        return 0

    semaphores = [threading.BoundedSemaphore(args.workers_per_key) for _ in keys]
    output_lock = threading.Lock()
    completed_count = 0
    started = time.time()

    with concurrent.futures.ThreadPoolExecutor(max_workers=len(keys) * args.workers_per_key) as executor:
        futures = [
            executor.submit(audit_one, task, keys, semaphores, args)
            for task in tasks
        ]
        for future in concurrent.futures.as_completed(futures):
            item = future.result()
            append_jsonl(output_path, output_lock, item)
            completed_count += 1
            if args.verbose or completed_count % 20 == 0 or completed_count == len(tasks):
                elapsed = max(time.time() - started, 0.001)
                print(
                    json.dumps(
                        {
                            "progress": f"{completed_count}/{len(tasks)}",
                            "rate_per_min": round(completed_count / elapsed * 60, 2),
                            "last": {
                                "bank": item.get("bank"),
                                "index": item.get("question_index"),
                                "status": item.get("status"),
                                "audit_status": item.get("audit_status"),
                                "severity": item.get("severity"),
                            },
                        },
                        ensure_ascii=False,
                    )
                )

    print(json.dumps({"summary": summarize_output(output_path), "output": str(output_path)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
