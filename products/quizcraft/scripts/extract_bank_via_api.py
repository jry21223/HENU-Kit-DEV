#!/usr/bin/env python3
from __future__ import annotations

import argparse
import collections
import json
import mimetypes
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
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


def post_multipart_file(url: str, token: str, field_name: str, path: Path, timeout: int) -> dict[str, Any]:
    boundary = f"----QuizCraftBoundary{uuid.uuid4().hex}"
    content_type = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
    file_bytes = path.read_bytes()

    body = b"".join(
        [
            f"--{boundary}\r\n".encode("utf-8"),
            (
                f'Content-Disposition: form-data; name="{field_name}"; '
                f'filename="{path.name}"\r\n'
            ).encode("utf-8"),
            f"Content-Type: {content_type}\r\n\r\n".encode("utf-8"),
            file_bytes,
            b"\r\n",
            f"--{boundary}--\r\n".encode("utf-8"),
        ]
    )

    request = urllib.request.Request(
        url,
        data=body,
        headers={
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "Content-Length": str(len(body)),
            "X-Admin-Token": token,
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"Extract failed: HTTP {exc.code}\n{detail}") from exc


def post_json(url: str, token: str, payload: dict[str, Any], timeout: int, label: str = "Request") -> dict[str, Any]:
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
        raise SystemExit(f"{label} failed: HTTP {exc.code}\n{detail}") from exc


def download_json(url: str, timeout: int) -> dict[str, Any]:
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"Download failed: HTTP {exc.code}\n{detail}") from exc


def absolute_download_url(api_base_url: str, download_url: str) -> str:
    if download_url.startswith(("http://", "https://")):
        parsed_download = urllib.parse.urlsplit(download_url)
        encoded_path = urllib.parse.quote(parsed_download.path, safe="/%")
        return urllib.parse.urlunsplit((
            parsed_download.scheme,
            parsed_download.netloc,
            encoded_path,
            parsed_download.query,
            parsed_download.fragment,
        ))

    parsed = urllib.parse.urlparse(api_base_url)
    origin = f"{parsed.scheme}://{parsed.netloc}"
    encoded_download = urllib.parse.quote(download_url, safe="/%?=&")
    if download_url.startswith("/"):
        return origin + encoded_download
    return api_base_url.rstrip("/") + "/" + encoded_download


def export_standard_bank(api_base_url: str, token: str, name: str, questions: list[dict[str, Any]], timeout: int) -> dict[str, Any]:
    export_endpoint = api_base_url.rstrip("/") + "/extract/export"
    export_result = post_json(
        export_endpoint,
        token,
        {"questions": questions, "name": name},
        timeout,
        label="Export",
    )
    download_url = export_result.get("download_url")
    if not isinstance(download_url, str) or not download_url:
        raise SystemExit("Export API did not return download_url.")
    return download_json(absolute_download_url(api_base_url, download_url), timeout)


def validate_questions(questions: list[dict[str, Any]]) -> list[str]:
    issues: list[str] = []
    ids = [question.get("id") for question in questions]
    duplicates = [item for item, count in collections.Counter(ids).items() if item and count > 1]
    if duplicates:
        issues.append(f"duplicate question ids: {duplicates[:10]}")

    for index, question in enumerate(questions, start=1):
        qid = question.get("id") or f"#{index}"
        q_type = question.get("type")
        options = question.get("options")
        answer = question.get("answer")
        if q_type in {"single", "multi"}:
            if not isinstance(options, list) or len(options) != 4:
                issues.append(f"{qid}: expected 4 options, got {len(options or [])}")
            if q_type == "single" and not isinstance(answer, int):
                issues.append(f"{qid}: single answer should be integer")
            if q_type == "multi" and not isinstance(answer, list):
                issues.append(f"{qid}: multi answer should be list")
        elif q_type == "judge":
            if not isinstance(answer, bool):
                issues.append(f"{qid}: judge answer should be boolean")
        else:
            issues.append(f"{qid}: unknown type {q_type!r}")
    return issues


def main() -> int:
    script_dir = Path(__file__).resolve().parent
    default_env = script_dir / ".env"

    parser = argparse.ArgumentParser(
        description="Extract a source file through QuizCraft's existing /extract/parse admin API."
    )
    parser.add_argument("source", type=Path, help="PDF/DOCX/TXT/JSON source file.")
    parser.add_argument("--key", required=True, help="Bank key, for example: mayuan.")
    parser.add_argument("--name", required=True, help="Question bank display name.")
    parser.add_argument("--color", help="Theme color, for example: #c62828.")
    parser.add_argument("--output", type=Path, help="Output parsed bank JSON. Defaults to /tmp/<key>.parsed.json.")
    parser.add_argument("--api-base-url", help="Admin API base URL. Defaults to QUIZCRAFT_API_BASE_URL.")
    parser.add_argument("--env", type=Path, default=default_env, help="Env file containing ADMIN_TOKEN.")
    parser.add_argument("--timeout", type=int, default=180)
    parser.add_argument("--save", action="store_true", help="Save parsed questions to the production bank immediately.")
    parser.add_argument("--no-overwrite", action="store_true", help="Fail if saving and the bank key already exists.")
    parser.add_argument("--allow-issues", action="store_true", help="Continue when local shape validation reports issues.")
    args = parser.parse_args()

    load_env_file(args.env.expanduser())
    api_base_url = resolve_admin_api_base_url(args.api_base_url)

    admin_token = os.getenv("ADMIN_TOKEN") or os.getenv("QUIZCRAFT_ADMIN_TOKEN")
    if not admin_token:
        raise SystemExit("Missing ADMIN_TOKEN. Put it in .env or export it before running.")

    source_path = args.source.expanduser().resolve()
    if not source_path.exists():
        raise SystemExit(f"Source file not found: {source_path}")

    endpoint = api_base_url + "/extract/parse"
    result = post_multipart_file(endpoint, admin_token, "file", source_path, args.timeout)
    questions = result.get("questions")
    if not isinstance(questions, list) or not questions:
        raise SystemExit("Extract API returned no questions.")

    output_path = (
        args.output.expanduser().resolve()
        if args.output
        else Path("/tmp") / f"{args.key}.parsed.json"
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)
    bank = export_standard_bank(api_base_url, admin_token, args.name, questions, args.timeout)
    meta = bank.get("meta")
    if isinstance(meta, dict):
        meta["key"] = args.key
        if args.color:
            meta["color"] = args.color

    standard_questions = bank.get("questions")
    if not isinstance(standard_questions, list) or not standard_questions:
        raise SystemExit("Export API returned no standard questions.")

    issues = validate_questions(standard_questions)
    output_path.write_text(json.dumps(bank, ensure_ascii=False, indent=2), encoding="utf-8")

    print(f"source={source_path}")
    print(f"output={output_path}")
    print(f"total={len(standard_questions)}")
    print(f"types={dict(collections.Counter(question.get('type') for question in standard_questions))}")
    print(f"issues={len(issues)}")
    for issue in issues[:20]:
        print(f"[issue] {issue}")

    if issues and not args.allow_issues:
        raise SystemExit("Validation failed. Re-run with --allow-issues only after manual review.")

    if args.save:
        save_endpoint = api_base_url + "/banks/save"
        save_result = post_json(
            save_endpoint,
            admin_token,
            {
                "key": args.key,
                "name": args.name,
                "color": args.color,
                "questions": standard_questions,
                "overwrite": not args.no_overwrite,
            },
            timeout=120,
            label="Save",
        )
        print(json.dumps({"saved": True, "bank": save_result.get("bank"), "file": save_result.get("file")}, ensure_ascii=False))

    return 0


if __name__ == "__main__":
    sys.exit(main())
