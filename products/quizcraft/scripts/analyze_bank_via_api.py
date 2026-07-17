#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

try:
    from scripts.admin_api_security import resolve_admin_api_base_url
except ModuleNotFoundError:
    from admin_api_security import resolve_admin_api_base_url

FALLBACK_PREFIXES = (
    "生成解析失败:",
    "本题考查相关知识点的理解。正确答案是",
    "这是一道",
    "解析：根据教材内容，本题正确答案为",
)


def load_env_file(path: Path) -> None:
    if not path.exists():
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


def read_bank(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise SystemExit(f"Invalid JSON: {exc}") from exc

    if not isinstance(data, dict):
        raise SystemExit("Bank JSON must be an object.")
    if not isinstance(data.get("questions"), list) or not data["questions"]:
        raise SystemExit("Bank JSON must contain a non-empty questions array.")
    return data


def post_json(url: str, token: str, payload: dict[str, Any], timeout: int) -> dict[str, Any]:
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=body,
        headers={
            "Content-Type": "application/json; charset=utf-8",
            "X-Admin-Token": token,
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"Request failed: HTTP {exc.code}\n{detail}") from exc


def output_path_for(input_path: Path) -> Path:
    return input_path.with_name(f"{input_path.stem}.analyzed{input_path.suffix}")


def fallback_like(analysis: Any) -> bool:
    text = str(analysis or "").strip()
    if not text:
        return True
    if text.startswith(FALLBACK_PREFIXES):
        return True
    return text.startswith("这是一道") and "需要掌握基础概念" in text


def build_llm_config(args: argparse.Namespace) -> dict[str, Any]:
    provider = args.provider or os.getenv("LLM_PROVIDER") or "deepseek"
    api_key = (
        args.api_key
        or os.getenv("LLM_API_KEY")
        or os.getenv("DEEPSEEK_API_KEY")
        or os.getenv("OPENAI_API_KEY")
        or os.getenv("SILICONFLOW_API_KEY")
    )
    api_url = args.api_url or os.getenv("LLM_API_URL") or None
    model = args.model or os.getenv("LLM_MODEL") or None

    if not api_key:
        raise SystemExit("Missing LLM_API_KEY. Put it in .env or export it before running.")

    return {
        "provider": provider,
        "apiKey": api_key,
        "apiUrl": api_url,
        "model": model,
    }


def save_output(path: Path, data: dict[str, Any]) -> None:
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")


def strip_transient_context(question: dict[str, Any]) -> dict[str, Any]:
    cleaned = dict(question)
    for key in ("global_context", "full_context", "lecture_context"):
        cleaned.pop(key, None)
    return cleaned


def main() -> int:
    script_dir = Path(__file__).resolve().parent
    default_env = script_dir / ".env"

    parser = argparse.ArgumentParser(description="Generate QuizCraft question analyses through the admin API.")
    parser.add_argument("bank_json", type=Path, help="Path to the bank JSON file.")
    parser.add_argument("--key", help="Bank key used when saving, for example: mayuan.")
    parser.add_argument("--name", help="Display name used when saving. Defaults to meta.name.")
    parser.add_argument("--color", help="Theme color used when saving. Defaults to meta.color.")
    parser.add_argument("--provider", choices=["deepseek", "openai", "siliconflow"])
    parser.add_argument("--api-key", help="LLM API key. Prefer .env instead of CLI history.")
    parser.add_argument("--api-url", help="Custom OpenAI-compatible base URL.")
    parser.add_argument("--model", help="Model name.")
    parser.add_argument("--api-base-url", help="Admin API base URL. Defaults to QUIZCRAFT_API_BASE_URL.")
    parser.add_argument("--env", type=Path, default=default_env, help="Env file containing ADMIN_TOKEN and LLM_API_KEY.")
    parser.add_argument("--output", type=Path, help="Output analyzed JSON path.")
    parser.add_argument("--global-context-file", type=Path, help="Text file prepended to every analysis prompt, not saved back.")
    parser.add_argument("--chunk-size", type=int, default=20)
    parser.add_argument("--limit", type=int, help="Only analyze the first N missing-analysis questions.")
    parser.add_argument("--force", action="store_true", help="Regenerate analyses even when questions already have analysis.")
    parser.add_argument("--save", action="store_true", help="Save the analyzed bank back to QuizCraft after generation.")
    parser.add_argument("--no-overwrite", action="store_true", help="Fail if saving and the bank key already exists.")
    parser.add_argument("--allow-fallback", action="store_true", help="Allow fallback/mock-looking analyses.")
    parser.add_argument("--timeout", type=int, default=900)
    args = parser.parse_args()

    load_env_file(args.env.expanduser())
    api_base_url = resolve_admin_api_base_url(args.api_base_url)

    admin_token = os.getenv("ADMIN_TOKEN") or os.getenv("QUIZCRAFT_ADMIN_TOKEN")
    if not admin_token:
        raise SystemExit("Missing ADMIN_TOKEN. Put it in .env or export it before running.")

    if args.chunk_size <= 0:
        raise SystemExit("--chunk-size must be positive.")

    bank_path = args.bank_json.expanduser()
    data = read_bank(bank_path)
    output_path = (args.output.expanduser() if args.output else output_path_for(bank_path))
    config = build_llm_config(args)
    global_context = ""
    if args.global_context_file:
        context_path = args.global_context_file.expanduser()
        if not context_path.exists():
            raise SystemExit(f"Global context file not found: {context_path}")
        global_context = context_path.read_text(encoding="utf-8").strip()

    questions = data["questions"]
    if args.force:
        pending_indexes = list(range(len(questions)))
        for question in questions:
            question["analysis"] = ""
    else:
        pending_indexes = [
            index
            for index, question in enumerate(questions)
            if not str(question.get("analysis") or "").strip()
        ]
    if args.limit is not None:
        pending_indexes = pending_indexes[: max(0, args.limit)]

    if not pending_indexes:
        print("No missing analyses found.")
        save_output(output_path, data)
        return 0

    endpoint = api_base_url + "/extract/analyze"
    total = len(pending_indexes)
    started = time.time()

    print(f"pending={total} output={output_path}")
    for start in range(0, total, args.chunk_size):
        batch_indexes = pending_indexes[start : start + args.chunk_size]
        batch = []
        for index in batch_indexes:
            item = dict(questions[index])
            if global_context:
                item["global_context"] = global_context
            batch.append(item)
        result = post_json(
            endpoint,
            admin_token,
            {"questions": batch, "config": config},
            timeout=args.timeout,
        )
        analyzed = result.get("questions")
        if not isinstance(analyzed, list) or len(analyzed) != len(batch):
            raise SystemExit("Analyze API returned an unexpected questions payload.")

        bad = [
            item.get("id") or f"batch-{idx}"
            for idx, item in enumerate(analyzed)
            if fallback_like(item.get("analysis"))
        ]
        if bad and not args.allow_fallback:
            save_output(output_path, data)
            sample = ", ".join(map(str, bad[:5]))
            raise SystemExit(
                "Analyze API returned fallback/mock-looking analyses. "
                f"Stopped before saving. Sample question ids: {sample}"
            )

        for index, analyzed_question in zip(batch_indexes, analyzed):
            questions[index] = strip_transient_context(analyzed_question)

        save_output(output_path, data)
        done = min(start + len(batch_indexes), total)
        elapsed = max(0.1, time.time() - started)
        print(f"analyzed={done}/{total} rate={done / elapsed:.2f}/s")

    if args.save:
        meta = data.get("meta") if isinstance(data.get("meta"), dict) else {}
        save_endpoint = api_base_url + "/banks/save"
        save_payload = {
            "key": args.key or bank_path.stem,
            "name": args.name or meta.get("name") or args.key or bank_path.stem,
            "color": args.color or meta.get("color"),
            "questions": questions,
            "overwrite": not args.no_overwrite,
        }
        save_result = post_json(save_endpoint, admin_token, save_payload, timeout=120)
        print(json.dumps({"saved": True, "bank": save_result.get("bank"), "file": save_result.get("file")}, ensure_ascii=False))

    print(json.dumps({"analyzed": total, "output": str(output_path)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
