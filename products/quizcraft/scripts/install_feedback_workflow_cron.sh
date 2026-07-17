#!/usr/bin/env bash
set -euo pipefail

SCRIPT_PATH="${SCRIPT_PATH:-/opt/quizcraft-cn/scripts/feedback_workflow.py}"
PYTHON_PATH="${PYTHON_PATH:-/opt/quizcraft-cn/.venv/bin/python}"
ENV_FILE="${ENV_FILE:-/etc/quizcraft-cn.env}"
CRON_TIME="${CRON_TIME:-0 10 * * *}"  # daily 10:00
LOG_FILE="${LOG_FILE:-/var/log/quizcraft-feedback-workflow.log}"
WEBHOOK_URL="${WEBHOOK_URL:-}"
TOP="${TOP:-10}"
TIMEZONE="${TIMEZONE:-Asia/Shanghai}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 1
fi

CRON_CMD="${CRON_TIME} ${PYTHON_PATH} ${SCRIPT_PATH} --env-file ${ENV_FILE} --top ${TOP} --timezone ${TIMEZONE} --output /var/log/quizcraft-feedback-workflow-latest.json"
if [ -n "${WEBHOOK_URL}" ]; then
  CRON_CMD="${CRON_CMD} --notify --webhook ${WEBHOOK_URL}"
fi
CRON_CMD="${CRON_CMD} >>${LOG_FILE} 2>&1"
CRON_MARKER="# QuizCraft feedback workflow"

TMP_CRON=$(mktemp)
(crontab -l 2>/dev/null || true) | grep -v "feedback_workflow.py" > "$TMP_CRON"
{
  echo "${CRON_MARKER}"
  echo "$CRON_CMD"
} >> "$TMP_CRON"
crontab "$TMP_CRON"
rm -f "$TMP_CRON"

echo "installed cron: ${CRON_TIME}"
echo "command: ${CRON_CMD}"