#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

try:
    from scripts.admin_api_security import resolve_admin_api_base_url
except ModuleNotFoundError:
    from admin_api_security import resolve_admin_api_base_url


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

    questions = data.get("questions")
    if not isinstance(questions, list) or not questions:
        raise SystemExit("Bank JSON must contain a non-empty questions array.")

    return data


def post_json(url: str, token: str, payload: dict[str, Any]) -> dict[str, Any]:
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
        with urllib.request.urlopen(request, timeout=60) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"Upload failed: HTTP {exc.code}\n{detail}") from exc


def main() -> int:
    script_dir = Path(__file__).resolve().parent
    default_env = script_dir / ".env"

    parser = argparse.ArgumentParser(description="Upload a QuizCraft bank JSON through the admin API.")
    parser.add_argument("bank_json", type=Path, help="Path to the bank JSON file.")
    parser.add_argument("--key", help="Bank key, for example: mayuan.")
    parser.add_argument("--name", help="Display name. Defaults to meta.name.")
    parser.add_argument("--color", help="Theme color, for example: #c62828. Defaults to meta.color.")
    parser.add_argument("--api-base-url", help="Admin API base URL. Defaults to QUIZCRAFT_API_BASE_URL.")
    parser.add_argument("--env", type=Path, default=default_env, help="Env file containing ADMIN_TOKEN.")
    parser.add_argument("--no-overwrite", action="store_true", help="Fail if the bank key already exists.")
    args = parser.parse_args()

    load_env_file(args.env.expanduser())
    api_base_url = resolve_admin_api_base_url(args.api_base_url)

    token = os.getenv("ADMIN_TOKEN") or os.getenv("QUIZCRAFT_ADMIN_TOKEN")
    if not token:
        raise SystemExit("Missing ADMIN_TOKEN. Put it in .env or export it before running.")

    bank_path = args.bank_json.expanduser()
    if not bank_path.exists():
        raise SystemExit(f"File not found: {bank_path}")

    data = read_bank(bank_path)
    meta = data.get("meta") if isinstance(data.get("meta"), dict) else {}
    questions = data["questions"]
    key = args.key or bank_path.stem
    name = args.name or meta.get("name") or key
    color = args.color or meta.get("color")

    payload: dict[str, Any] = {
        "key": key,
        "name": name,
        "color": color,
        "questions": questions,
        "overwrite": not args.no_overwrite,
    }

    endpoint = api_base_url + "/banks/save"
    result = post_json(endpoint, token, payload)
    bank = result.get("bank", {})
    print(
        json.dumps(
            {
                "uploaded": True,
                "key": bank.get("key", key),
                "name": bank.get("name", name),
                "total": bank.get("total", len(questions)),
                "file": result.get("file"),
            },
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
