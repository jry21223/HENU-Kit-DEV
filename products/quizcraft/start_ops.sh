#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BACKEND_PORT="${BACKEND_PORT:-10086}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
WAIT_HOST="${WAIT_HOST:-127.0.0.1}"
PYTHON_BIN="${PYTHON_BIN:-${ROOT_DIR}/.venv/bin/python}"
SERVER_PID=""
CLIENT_PID=""

cleanup() {
  if [ -n "${SERVER_PID}" ]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  if [ -n "${CLIENT_PID}" ]; then
    kill "${CLIENT_PID}" >/dev/null 2>&1 || true
  fi
}

wait_for_url() {
  local name="$1"
  local url="$2"
  local retries="${3:-30}"
  local i
  for ((i=1; i<=retries; i++)); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      echo "ready: ${name} ${url}"
      return 0
    fi
    sleep 1
  done
  echo "failed to start: ${name} ${url}" >&2
  return 1
}

trap cleanup EXIT INT TERM

if [ ! -x "${PYTHON_BIN}" ]; then
  echo "Python runtime not found: ${PYTHON_BIN}" >&2
  echo "Run scripts/install_deps.sh first." >&2
  exit 1
fi

if [ ! -d "${ROOT_DIR}/web-app/node_modules" ]; then
  echo "Frontend dependencies not found. Run scripts/install_deps.sh first." >&2
  exit 1
fi

cd "${ROOT_DIR}"
scripts/build_ops.sh

APP_HOST="${APP_HOST:-127.0.0.1}" \
APP_PORT="${BACKEND_PORT}" \
CORS_ORIGINS="${CORS_ORIGINS:-http://${WAIT_HOST}:${FRONTEND_PORT},http://localhost:${FRONTEND_PORT}}" \
"${PYTHON_BIN}" server.py > server.log 2>&1 &
SERVER_PID=$!
wait_for_url "backend" "http://${WAIT_HOST}:${BACKEND_PORT}/api/banks"

cd "${ROOT_DIR}/web-app"
npm run preview:ops -- --host "${FRONTEND_HOST:-127.0.0.1}" --port "${FRONTEND_PORT}" > dev.log 2>&1 &
CLIENT_PID=$!
wait_for_url "frontend" "http://${WAIT_HOST}:${FRONTEND_PORT}/"

echo "ops preview running"
echo "frontend: http://${WAIT_HOST}:${FRONTEND_PORT}"
echo "backend:  http://${WAIT_HOST}:${BACKEND_PORT}"
wait
