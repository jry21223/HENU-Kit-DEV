#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

: "${GO_BASE_URL:?set the loopback Go origin, for example http://127.0.0.1:10089}"
: "${LEGACY_BASE_URL:?set LEGACY_BASE_URL, for example http://127.0.0.1:10086}"
: "${EXPECTED_RELEASE_SHA:?set EXPECTED_RELEASE_SHA}"
: "${EXPECTED_WRITES_ENABLED:?set EXPECTED_WRITES_ENABLED to true or false}"
: "${EXPECTED_LEGACY_READ_ONLY:?set EXPECTED_LEGACY_READ_ONLY to true or false}"
: "${CUTOVER_EVIDENCE_SECRET:?set CUTOVER_EVIDENCE_SECRET}"
: "${EXPECTED_MIGRATION_RUN_ID:?set EXPECTED_MIGRATION_RUN_ID}"
: "${EXPECTED_SOURCE_HEAD:?set EXPECTED_SOURCE_HEAD}"
: "${LEGACY_SERVER_PATH:?set LEGACY_SERVER_PATH to the deployed server.py}"
: "${EXPECTED_LEGACY_SHA256:?set EXPECTED_LEGACY_SHA256}"
: "${LEGACY_DATABASE_URL:?export LEGACY_DATABASE_URL without printing it}"
: "${CUTOVER_GATE_EVIDENCE_FILE:?set root-owned cutover gate evidence JSON path}"
: "${PUBLIC_BASE_URL:?set the public QuizCraft origin currently serving maintenance}"
: "${CUTOVER_E2E_BASE_URL:?set a maintenance-bypass origin serving the staged browser bundle and Go API}"
: "${CUTOVER_WEB_APP_DIR:?set the immutable web-app release directory with Playwright installed}"
: "${PLATFORM_ACCOUNT_ORIGIN:?set the Platform Core account origin}"
: "${CONSOLE_ORIGIN:?set the Console origin}"
: "${CUTOVER_TEST_EMAIL:?set the real mailbox address used for cutover verification}"
: "${CUTOVER_RESTORE_ADMIN_URL:?set the isolated PostgreSQL admin used for temporary restore databases}"
: "${LEGACY_PYTHON:=/opt/quizcraft-cn/.venv/bin/python}"

[[ "$EXPECTED_WRITES_ENABLED" =~ ^(true|false)$ ]]
[[ "$EXPECTED_LEGACY_READ_ONLY" =~ ^(true|false)$ ]]
[[ "$CUTOVER_EVIDENCE_SECRET" =~ ^[A-Za-z0-9_-]{32,}$ ]]
[[ "$EXPECTED_SOURCE_HEAD" =~ ^[0-9]+$ ]]
[[ "$EXPECTED_RELEASE_SHA" =~ ^[0-9a-f]{40}$ ]]
[[ -f "$CUTOVER_GATE_EVIDENCE_FILE" ]]
test "$(stat -c '%u' "$CUTOVER_GATE_EVIDENCE_FILE")" = 0
gate_permissions="$(stat -c '%a' "$CUTOVER_GATE_EVIDENCE_FILE")"
test $(( 8#$gate_permissions & 8#077 )) -eq 0
require_root_executable() {
  local path="$1" permissions
  [[ -f "$path" && -x "$path" ]]
  test "$(stat -c '%u' "$path")" = 0
  permissions="$(stat -c '%a' "$path")"
  test $(( 8#$permissions & 8#022 )) -eq 0
}

browser_verifier="$script_dir/verify-browser-cutover.mjs"
platform_core_verifier="$script_dir/verify-platform-core-cutover.py"
log_verifier="$script_dir/verify-cutover-logs.sh"
restore_verifier="$script_dir/verify-backup-restores.py"
require_root_executable "$browser_verifier"
require_root_executable "$platform_core_verifier"
require_root_executable "$log_verifier"
require_root_executable "$restore_verifier"
: "${QUIZCRAFT_OPERATOR_SESSION:?set a dedicated cutover operator session with Workshop read permission}"
[[ "$QUIZCRAFT_OPERATOR_SESSION" =~ ^[A-Za-z0-9._~-]{32,}$ ]]

cutover_tmp="$(mktemp -d)"
trap 'rm -rf "$cutover_tmp"' EXIT
chmod 700 "$cutover_tmp"
printf 'header = "X-QuizCraft-Cutover-Secret: %s"\n' "$CUTOVER_EVIDENCE_SECRET" > "$cutover_tmp/evidence.curl"
chmod 600 "$cutover_tmp/evidence.curl"
if [[ "$EXPECTED_WRITES_ENABLED" == "true" ]]; then
  printf 'cookie = "__Host-quizcraft_session=%s"\n' "$QUIZCRAFT_OPERATOR_SESSION" > "$cutover_tmp/operator.curl"
  chmod 600 "$cutover_tmp/operator.curl"
fi

curl --fail --silent --show-error --max-time 10 "$GO_BASE_URL/readyz" > "$cutover_tmp/readiness.json"
curl --fail --silent --show-error --max-time 10 "$LEGACY_BASE_URL/api/healthz" > "$cutover_tmp/legacy-health.json"
curl --fail --silent --show-error --max-time 10 "$GO_BASE_URL/api/v1/banks" > "$cutover_tmp/banks.json"
curl --fail --silent --show-error --max-time 10 --config "$cutover_tmp/evidence.curl" \
  "$GO_BASE_URL/api/v1/cutover-evidence?run_id=$EXPECTED_MIGRATION_RUN_ID&source_head=$EXPECTED_SOURCE_HEAD" \
  > "$cutover_tmp/evidence.json"
test "$(sha256sum "$LEGACY_SERVER_PATH" | awk '{print $1}')" = "$EXPECTED_LEGACY_SHA256"
actual_source_head="$("$LEGACY_PYTHON" - <<'PY'
import os
import psycopg
with psycopg.connect(os.environ["LEGACY_DATABASE_URL"]) as connection:
    with connection.cursor() as cursor:
        cursor.execute("SELECT COALESCE(max(event_id),0) FROM quizcraft_migration_events")
        print(cursor.fetchone()[0])
PY
)"
test "$actual_source_head" = "$EXPECTED_SOURCE_HEAD"
maintenance_status="$(curl --silent --show-error --max-time 10 -o "$cutover_tmp/maintenance.html" -w '%{http_code}' "$PUBLIC_BASE_URL/")"
test "$maintenance_status" = 503
grep -q 'QuizCraft 正在维护' "$cutover_tmp/maintenance.html"

assert_public_maintenance() {
  local method="$1" path="$2" output="$cutover_tmp/public-${3}.html" status
  status="$(curl --silent --show-error --max-time 10 -o "$output" -w '%{http_code}' -X "$method" -H 'Content-Type: application/json' --data '{}' "$PUBLIC_BASE_URL$path")"
  test "$status" = 503
  grep -q 'QuizCraft 正在维护' "$output"
}

assert_public_maintenance POST /api/v1/practice/sessions practice
assert_public_maintenance POST /api/v1/feedback feedback
assert_public_maintenance PUT /api/v1/banks/maintenance-probe/favorites/maintenance-probe favorite

if [[ "$EXPECTED_WRITES_ENABLED" == "true" ]]; then
  if ! CUTOVER_VIEWPORT=desktop CUTOVER_E2E_BASE_URL="$CUTOVER_E2E_BASE_URL" CUTOVER_WEB_APP_DIR="$CUTOVER_WEB_APP_DIR" QUIZCRAFT_OPERATOR_SESSION="$QUIZCRAFT_OPERATOR_SESSION" node "$browser_verifier" >"$cutover_tmp/browser-desktop.log" 2>&1; then
    echo "desktop browser cutover verification failed" >&2
    exit 1
  fi
  if ! CUTOVER_VIEWPORT=mobile_390 CUTOVER_E2E_BASE_URL="$CUTOVER_E2E_BASE_URL" CUTOVER_WEB_APP_DIR="$CUTOVER_WEB_APP_DIR" QUIZCRAFT_OPERATOR_SESSION="$QUIZCRAFT_OPERATOR_SESSION" node "$browser_verifier" >"$cutover_tmp/browser-mobile-390.log" 2>&1; then
    echo "390px browser cutover verification failed" >&2
    exit 1
  fi
fi
if ! "$log_verifier" >"$cutover_tmp/log-audit.log" 2>&1; then
  echo "cutover log audit failed" >&2
  exit 1
fi

python3 - "$cutover_tmp" "$EXPECTED_RELEASE_SHA" "$EXPECTED_WRITES_ENABLED" "$EXPECTED_LEGACY_READ_ONLY" "$EXPECTED_MIGRATION_RUN_ID" "$EXPECTED_SOURCE_HEAD" "$CUTOVER_GATE_EVIDENCE_FILE" <<'PY'
import hashlib
import json, pathlib, sys
from datetime import datetime, timedelta, timezone

root = pathlib.Path(sys.argv[1])
expected_sha = sys.argv[2]
expected_writes = sys.argv[3] == "true"
expected_legacy_read_only = sys.argv[4] == "true"
expected_run_id = sys.argv[5]
expected_source_head = int(sys.argv[6])
readiness = json.loads((root / "readiness.json").read_text())
legacy = json.loads((root / "legacy-health.json").read_text())
banks = json.loads((root / "banks.json").read_text())
evidence = json.loads((root / "evidence.json").read_text()).get("data", {})
gate_path = pathlib.Path(sys.argv[7])
gate = json.loads(gate_path.read_text())
assert gate.get("release_sha") == expected_sha, gate
assert gate.get("migration_run_id") == expected_run_id, gate
assert gate.get("source_head") == expected_source_head, gate
created_at = datetime.fromisoformat(gate["created_at"].replace("Z", "+00:00"))
assert datetime.now(timezone.utc) - timedelta(hours=24) <= created_at <= datetime.now(timezone.utc) + timedelta(minutes=5), gate
reconciliation = gate.get("reconciliation", {})
assert reconciliation.get("cursor_equals_head") is True and reconciliation.get("exceptions") == 0, reconciliation
for key in ("banks", "questions", "question_types", "answers", "chapters", "feedback", "legacy_rankings", "content_hashes"):
    assert reconciliation.get(key) is True, (key, reconciliation)
for owner in ("legacy", "go"):
    backup = gate.get("backups", {}).get(owner, {})
    path = pathlib.Path(backup.get("path", ""))
    assert path.is_file(), backup
    stat = path.stat()
    assert stat.st_uid == 0 and stat.st_mode & 0o077 == 0, backup
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    assert digest.hexdigest() == backup.get("sha256"), backup
assert readiness.get("data", {}).get("status") == "ok", readiness
assert evidence.get("database") == "ready", evidence
assert evidence.get("release_sha") == expected_sha, evidence
assert evidence.get("writes_enabled") is expected_writes, evidence
assert evidence.get("migration_run_id") == expected_run_id, evidence
assert evidence.get("migration_cursor") == expected_source_head, evidence
assert legacy.get("read_only") is expected_legacy_read_only, legacy
assert banks.get("data"), "Go bank list is empty"
PY

if ! CUTOVER_GATE_EVIDENCE_FILE="$CUTOVER_GATE_EVIDENCE_FILE" CUTOVER_RESTORE_ADMIN_URL="$CUTOVER_RESTORE_ADMIN_URL" EXPECTED_MIGRATION_RUN_ID="$EXPECTED_MIGRATION_RUN_ID" EXPECTED_SOURCE_HEAD="$EXPECTED_SOURCE_HEAD" "$LEGACY_PYTHON" "$restore_verifier" >"$cutover_tmp/restore.log" 2>&1; then
  echo "final backup restore verification failed" >&2
  exit 1
fi

if [[ "$EXPECTED_WRITES_ENABLED" == "false" ]]; then
  go_write_status="$(curl --silent --show-error --max-time 10 -o "$cutover_tmp/go-write-block.json" -w '%{http_code}' -H 'Content-Type: application/json' -H 'Idempotency-Key: cutover-disabled-probe' --data '{}' "$GO_BASE_URL/api/v1/practice/sessions")"
  test "$go_write_status" = "503"
  python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["error"]["code"] == "writes_disabled"' "$cutover_tmp/go-write-block.json"
fi

if [[ "$EXPECTED_LEGACY_READ_ONLY" == "true" ]]; then
  legacy_write_status="$(curl --silent --show-error --max-time 10 -o "$cutover_tmp/legacy-write-block.json" -w '%{http_code}' -H 'Content-Type: application/json' --data '{}' "$LEGACY_BASE_URL/api/feedback")"
  test "$legacy_write_status" = "503"
  python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["detail"] == "旧 QuizCraft 正处于只读观察期"' "$cutover_tmp/legacy-write-block.json"
fi

if [[ "$EXPECTED_WRITES_ENABLED" == "true" ]]; then
  python3 - "$cutover_tmp/banks.json" > "$cutover_tmp/session-request.json" <<'PY'
import json, sys
bank = json.load(open(sys.argv[1]))["data"][0]
json.dump({"bank_id": bank["bank_id"], "bank_version_id": bank["bank_version_id"], "mode": "random", "question_count": 1}, sys.stdout)
PY
  curl --fail --silent --show-error --max-time 10 \
    --cookie-jar "$cutover_tmp/cookies" \
    -H 'Content-Type: application/json' \
    -H "Idempotency-Key: cutover-session-$EXPECTED_RELEASE_SHA" \
    --data-binary @"$cutover_tmp/session-request.json" \
    "$GO_BASE_URL/api/v1/practice/sessions" > "$cutover_tmp/session.json"
  python3 - "$cutover_tmp/session.json" > "$cutover_tmp/answer-request.json" <<'PY'
import json, sys
question = json.load(open(sys.argv[1]))["data"]["questions"][0]
json.dump({"question_id": question["question_id"], "question_version_id": question["question_version_id"], "answer": 0}, sys.stdout)
PY
  session_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["data"]["session_id"])' "$cutover_tmp/session.json")"
  curl --fail --silent --show-error --max-time 10 \
    --cookie "$cutover_tmp/cookies" \
    -H 'Content-Type: application/json' \
    -H "Idempotency-Key: cutover-answer-$EXPECTED_RELEASE_SHA" \
    --data-binary @"$cutover_tmp/answer-request.json" \
    "$GO_BASE_URL/api/v1/practice/sessions/$session_id/answers" > "$cutover_tmp/answer.json"
  python3 - "$cutover_tmp/answer.json" <<'PY'
import json, sys
data = json.load(open(sys.argv[1])).get("data", {})
assert data.get("question_id") and isinstance(data.get("correct"), bool), data
PY

  python3 - "$cutover_tmp/session.json" "$EXPECTED_RELEASE_SHA" > "$cutover_tmp/feedback-request.json" <<'PY'
import json, sys
session = json.load(open(sys.argv[1]))["data"]
question = session["questions"][0]
json.dump({
    "bank_id": session["bank_id"],
    "question_id": question["question_id"],
    "question_version_id": question["question_version_id"],
    "category": "other",
    "detail": f"HC-22 live cutover verification {sys.argv[2]}",
}, sys.stdout)
PY
  curl --fail --silent --show-error --max-time 10 \
    --config "$cutover_tmp/operator.curl" \
    -H 'Content-Type: application/json' \
    -H "Idempotency-Key: cutover-feedback-$EXPECTED_RELEASE_SHA" \
    --data-binary @"$cutover_tmp/feedback-request.json" \
    "$GO_BASE_URL/api/v1/feedback" > "$cutover_tmp/feedback.json"
  python3 -c 'import json,sys; data=json.load(open(sys.argv[1]))["data"]; assert data["state"] == "succeeded" and data["resource_id"]' "$cutover_tmp/feedback.json"

  curl --fail --silent --show-error --max-time 10 \
    "$GO_BASE_URL/api/v1/rankings/overall?period=weekly" > "$cutover_tmp/ranking.json"
  python3 -c 'import json,sys; data=json.load(open(sys.argv[1]))["data"]; assert data["scope"] == "overall" and data["period"] == "weekly" and isinstance(data["entries"], list)' "$cutover_tmp/ranking.json"

  read -r bank_id question_id < <(python3 -c 'import json,sys; data=json.load(open(sys.argv[1]))["data"]; print(data["bank_id"], data["questions"][0]["question_id"])' "$cutover_tmp/session.json")
  probe_nonce="$(date +%s)-$$"
  curl --fail --silent --show-error --max-time 10 \
    --config "$cutover_tmp/operator.curl" \
    --request PUT \
    -H "Idempotency-Key: cutover-favorite-add-$probe_nonce" \
    "$GO_BASE_URL/api/v1/banks/$bank_id/favorites/$question_id" > "$cutover_tmp/favorite-add.json"
  curl --fail --silent --show-error --max-time 10 \
    --config "$cutover_tmp/operator.curl" \
    "$GO_BASE_URL/api/v1/banks/$bank_id/favorites" > "$cutover_tmp/favorites.json"
  python3 - "$cutover_tmp/favorites.json" "$question_id" <<'PY'
import json, sys
items = json.load(open(sys.argv[1]))["data"]
assert any(item["question_id"] == sys.argv[2] and item["available"] for item in items), items
PY
  curl --fail --silent --show-error --max-time 10 \
    --config "$cutover_tmp/operator.curl" \
    --request DELETE \
    -H "Idempotency-Key: cutover-favorite-remove-$probe_nonce" \
    "$GO_BASE_URL/api/v1/banks/$bank_id/favorites/$question_id" > "$cutover_tmp/favorite-remove.json"

  curl --fail --silent --show-error --max-time 10 \
    --config "$cutover_tmp/operator.curl" \
    "$GO_BASE_URL/api/v1/workshop/catalog" > "$cutover_tmp/workshop.json"
  python3 -c 'import json,sys; data=json.load(open(sys.argv[1]))["data"]; assert data and all(item.get("bank_id") and isinstance(item.get("versions"), list) for item in data)' "$cutover_tmp/workshop.json"
fi

if [[ "$EXPECTED_WRITES_ENABLED" == "false" ]]; then
  if ! PLATFORM_ACCOUNT_ORIGIN="$PLATFORM_ACCOUNT_ORIGIN" \
    CONSOLE_ORIGIN="$CONSOLE_ORIGIN" \
    CUTOVER_TEST_EMAIL="$CUTOVER_TEST_EMAIL" \
    python3 "$platform_core_verifier" >"$cutover_tmp/platform-core.log" 2>&1; then
    echo "real-mail Platform Core login/OAuth/revocation verification failed" >&2
    exit 1
  fi
fi

if ! "$log_verifier" >"$cutover_tmp/final-log-audit.log" 2>&1; then
  echo "final cutover log audit failed" >&2
  exit 1
fi

echo "QuizCraft cutover verification passed for $EXPECTED_RELEASE_SHA (health, evidence, practice, feedback, favorites, ranking, workshop)"
