#!/usr/bin/env python3
"""GitHub webhook receiver for QuizCraft CN production deploys."""

from __future__ import annotations

import hashlib
import hmac
import json
import os
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any


HOST = os.getenv("WEBHOOK_HOST", "127.0.0.1")
PORT = int(os.getenv("WEBHOOK_PORT", "10087"))
PATH = os.getenv("WEBHOOK_PATH", "/webhook/github")
MAX_BODY_BYTES = int(os.getenv("WEBHOOK_MAX_BODY_BYTES", str(1024 * 1024)))

EXPECTED_REPOSITORY = os.getenv("EXPECTED_REPOSITORY", "jry21223/quizcraft-cn")
BRANCH = os.getenv("BRANCH", "master")
REPO_DIR = os.getenv("REPO_DIR", "/opt/quizcraft-cn")
STATIC_DEPLOY_DIR = os.getenv("STATIC_DEPLOY_DIR", "/var/www/quizcraft-cn")
SERVICE_NAME = os.getenv("SERVICE_NAME", "quizcraft-cn.service")
LOCK_PATH = os.getenv("DEPLOY_LOCK_PATH", "/run/quizcraft-deploy.lock")
BUILD_FRONTEND_ON_WEBHOOK = os.getenv("BUILD_FRONTEND_ON_WEBHOOK", "0").strip().lower() in {
    "1",
    "true",
    "yes",
    "on",
}


def log(message: str) -> None:
    print(message, flush=True)


def required_env(name: str) -> str:
    value = os.getenv(name)
    if not value:
        raise RuntimeError(f"missing required environment variable: {name}")
    return value


def verify_signature(secret: str, body: bytes, signature_header: str | None) -> bool:
    if not signature_header or not signature_header.startswith("sha256="):
        return False
    expected = hmac.new(secret.encode("utf-8"), body, hashlib.sha256).hexdigest()
    received = signature_header.removeprefix("sha256=")
    return hmac.compare_digest(expected, received)


def run_deploy(delivery: str, pushed_ref: str, before: str, after: str) -> None:
    deploy_script = f"""
set -euo pipefail
cd {shell_quote(REPO_DIR)}

echo "[deploy] delivery={delivery} ref={pushed_ref} before={before} after={after}"
start_head="$(git rev-parse HEAD)"
echo "[deploy] start_head=${{start_head}}"

timestamp="$(date -u +\"%Y%m%d%H%M%S\")"
backup_dir="/root/quizcraft-pre-webhook-${{timestamp}}"
dirty_state="$(git status --short || true)"
if [ -n "${{dirty_state}}" ]; then
  echo "[deploy] dirty working tree detected, backup_dir=${{backup_dir}}"
  mkdir -p "${{backup_dir}}"
  git status --short > "${{backup_dir}}/git_status.txt"
  git diff > "${{backup_dir}}/git_diff.txt"
  git diff --cached > "${{backup_dir}}/git_diff_cached.txt"
  git stash push -u -m "pre-webhook server dirty state ${{timestamp}}"
  echo "[deploy] stashed dirty changes into git stash"
fi

git fetch --all --prune
git checkout {shell_quote(BRANCH)}
pull_before="$(git rev-parse HEAD)"
git pull --ff-only origin {shell_quote(BRANCH)}
pull_after="$(git rev-parse HEAD)"
echo "[deploy] pull_before=${{pull_before}} pull_after=${{pull_after}}"

changed_backend_deps="$(git diff --name-only "${{pull_before}}" "${{pull_after}}" -- requirements.txt || true)"
if [ -n "${{changed_backend_deps}}" ] || [ ! -d ".venv" ]; then
  echo "[deploy] backend dependencies changed or venv missing; installing Python deps"
  PYTHON_BIN="${{PYTHON_BIN:-}}"
  if [ -z "${{PYTHON_BIN}}" ]; then
    if command -v python3.11 >/dev/null 2>&1; then
      PYTHON_BIN="$(command -v python3.11)"
    else
      PYTHON_BIN="$(command -v python3)"
    fi
  fi
  if [ ! -d ".venv" ]; then
    "${{PYTHON_BIN}}" -m venv .venv
  fi
  .venv/bin/python -m pip install --upgrade pip
  .venv/bin/python -m pip install -r requirements.txt
else
  echo "[deploy] backend dependencies unchanged; skipping Python install"
fi

if {str(BUILD_FRONTEND_ON_WEBHOOK).lower()}; then
  changed_frontend_deps="$(git diff --name-only "${{pull_before}}" "${{pull_after}}" -- web-app/package.json web-app/package-lock.json || true)"
  if [ -n "${{changed_frontend_deps}}" ] || [ ! -d "web-app/node_modules" ] || [ ! -f "web-app/node_modules/.bin/vite" ]; then
    echo "[deploy] frontend dependencies changed or missing; installing npm deps"
    cd web-app
    if [ -f package-lock.json ]; then
      npm ci --include=dev
    else
      npm install --include=dev
    fi
    cd ..
  else
    echo "[deploy] frontend dependencies unchanged; skipping npm install"
  fi
  STATIC_DEPLOY_DIR={shell_quote(STATIC_DEPLOY_DIR)} ./scripts/build_ops.sh
else
  echo "[deploy] skipping frontend build on webhook; deploy web-app/dist from CI or local machine"
fi

systemctl restart {shell_quote(SERVICE_NAME)}
systemctl status {shell_quote(SERVICE_NAME)} --no-pager -n 30
echo "[deploy] success head=$(git rev-parse --short HEAD)"
"""
    command = ["flock", "-n", LOCK_PATH, "bash", "-lc", deploy_script]
    env = os.environ.copy()
    log(f"[deploy] queued delivery={delivery}")
    result = subprocess.run(command, env=env, text=True)
    if result.returncode == 0:
        log(f"[deploy] completed delivery={delivery}")
    else:
        log(f"[deploy] failed delivery={delivery} exit={result.returncode}")


def shell_quote(value: str) -> str:
    return "'" + value.replace("'", "'\"'\"'") + "'"


def json_response(handler: BaseHTTPRequestHandler, status: int, payload: dict[str, Any]) -> None:
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json; charset=utf-8")
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


class GitHubWebhookHandler(BaseHTTPRequestHandler):
    server_version = "QuizCraftDeployWebhook/1.0"

    def log_message(self, fmt: str, *args: Any) -> None:
        log(f"[http] {self.address_string()} - {fmt % args}")

    def do_GET(self) -> None:
        if self.path == "/healthz":
            json_response(self, 200, {"ok": True})
            return
        json_response(self, 404, {"error": "not found"})

    def do_POST(self) -> None:
        if self.path != PATH:
            json_response(self, 404, {"error": "not found"})
            return

        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            json_response(self, 400, {"error": "invalid content length"})
            return
        if length <= 0 or length > MAX_BODY_BYTES:
            json_response(self, 413, {"error": "payload too large"})
            return

        body = self.rfile.read(length)
        secret = required_env("WEBHOOK_SECRET")
        signature = self.headers.get("X-Hub-Signature-256")
        if not verify_signature(secret, body, signature):
            json_response(self, 401, {"error": "invalid signature"})
            return

        event = self.headers.get("X-GitHub-Event", "")
        delivery = self.headers.get("X-GitHub-Delivery", "unknown")
        if event == "ping":
            log(f"[webhook] accepted ping delivery={delivery}")
            json_response(self, 200, {"ok": True, "event": "ping"})
            return
        if event != "push":
            log(f"[webhook] ignored event={event} delivery={delivery}")
            json_response(self, 202, {"ok": True, "ignored": "unsupported event"})
            return

        try:
            payload = json.loads(body.decode("utf-8"))
        except json.JSONDecodeError:
            json_response(self, 400, {"error": "invalid json"})
            return

        repository = payload.get("repository", {}).get("full_name")
        pushed_ref = payload.get("ref")
        expected_ref = f"refs/heads/{BRANCH}"
        if repository != EXPECTED_REPOSITORY or pushed_ref != expected_ref:
            log(
                "[webhook] ignored push "
                f"delivery={delivery} repository={repository} ref={pushed_ref}"
            )
            json_response(self, 202, {"ok": True, "ignored": "repository or branch mismatch"})
            return

        before = str(payload.get("before", ""))
        after = str(payload.get("after", ""))
        thread = threading.Thread(
            target=run_deploy,
            args=(delivery, pushed_ref, before, after),
            daemon=True,
        )
        thread.start()
        json_response(self, 202, {"ok": True, "deployment": "queued"})


def main() -> int:
    try:
        required_env("WEBHOOK_SECRET")
    except RuntimeError as exc:
        log(f"[fatal] {exc}")
        return 2

    httpd = ThreadingHTTPServer((HOST, PORT), GitHubWebhookHandler)
    log(f"[server] listening on http://{HOST}:{PORT}{PATH}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        log("[server] shutting down")
    finally:
        httpd.server_close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
