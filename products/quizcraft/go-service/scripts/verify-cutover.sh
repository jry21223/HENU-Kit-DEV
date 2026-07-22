#!/usr/bin/env bash
set -euo pipefail

: "${GO_BASE_URL:?set GO_BASE_URL, for example https://quiz.example.com}"
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
: "${LEGACY_PYTHON:=/opt/quizcraft-cn/.venv/bin/python}"

[[ "$EXPECTED_WRITES_ENABLED" =~ ^(true|false)$ ]]
[[ "$EXPECTED_LEGACY_READ_ONLY" =~ ^(true|false)$ ]]
[[ "$CUTOVER_EVIDENCE_SECRET" =~ ^[A-Za-z0-9_-]{32,}$ ]]
[[ "$EXPECTED_SOURCE_HEAD" =~ ^[0-9]+$ ]]
[[ "$EXPECTED_RELEASE_SHA" =~ ^[0-9a-f]{40}$ ]]
if [[ "$EXPECTED_WRITES_ENABLED" == "true" ]]; then
  : "${QUIZCRAFT_OPERATOR_SESSION:?set a dedicated cutover operator session with Workshop read permission}"
  [[ "$QUIZCRAFT_OPERATOR_SESSION" =~ ^[A-Za-z0-9._~-]{32,}$ ]]
fi

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

python3 - "$cutover_tmp" "$EXPECTED_RELEASE_SHA" "$EXPECTED_WRITES_ENABLED" "$EXPECTED_LEGACY_READ_ONLY" "$EXPECTED_MIGRATION_RUN_ID" "$EXPECTED_SOURCE_HEAD" <<'PY'
import json, pathlib, sys

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
assert readiness.get("data", {}).get("status") == "ok", readiness
assert evidence.get("database") == "ready", evidence
assert evidence.get("release_sha") == expected_sha, evidence
assert evidence.get("writes_enabled") is expected_writes, evidence
assert evidence.get("migration_run_id") == expected_run_id, evidence
assert evidence.get("migration_cursor") == expected_source_head, evidence
assert legacy.get("read_only") is expected_legacy_read_only, legacy
assert banks.get("data"), "Go bank list is empty"
PY

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

echo "QuizCraft cutover verification passed for $EXPECTED_RELEASE_SHA (health, evidence, practice, feedback, favorites, ranking, workshop)"
