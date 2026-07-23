#!/usr/bin/env bash
set -euo pipefail

: "${CUTOVER_LOG_SINCE:?set an absolute journalctl --since timestamp}"
units=(quizcraft-go quizcraft-cn platform-core platform-core-mail-worker platform-core-smtp-provider console-gateway)
log_tmp="$(mktemp)"
error_tmp="$(mktemp)"
trap 'rm -f "$log_tmp" "$error_tmp"' EXIT
journalctl -q --no-pager --since "$CUTOVER_LOG_SINCE" "${units[@]/#/-u}" > "$log_tmp"
journalctl -q --no-pager --priority=err --since "$CUTOVER_LOG_SINCE" "${units[@]/#/-u}" > "$error_tmp"
test ! -s "$error_tmp"
for name in CUTOVER_EVIDENCE_SECRET QUIZCRAFT_OPERATOR_SESSION PLATFORM_CLIENT_SECRET; do
  value="${!name:-}"
  if [[ -n "$value" ]] && grep -Fq -- "$value" "$log_tmp"; then
    echo "secret value from $name appeared in service logs" >&2
    exit 1
  fi
done
if grep -Eiq 'authorization:[[:space:]]*bearer[[:space:]]+[A-Za-z0-9._~-]{20,}|[A-Za-z0-9._%+-]+@henu\.edu\.cn|验证码[^[:digit:]]*[[:digit:]]{6,10}' "$log_tmp"; then
  echo "service logs contain a token, student email, or verification code" >&2
  exit 1
fi
