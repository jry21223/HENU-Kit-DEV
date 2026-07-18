#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WORKSPACE_ROOT="$(cd "${ROOT_DIR}/../.." && pwd)"
PYTHON_BIN="${PYTHON_BIN:-}"

if [ -z "${PYTHON_BIN}" ]; then
  if command -v python3.11 >/dev/null 2>&1; then
    PYTHON_BIN="$(command -v python3.11)"
  else
    PYTHON_BIN="$(command -v python3)"
  fi
fi

cd "${ROOT_DIR}"

if [ ! -d ".venv" ]; then
  "${PYTHON_BIN}" -m venv .venv
fi

.venv/bin/python -m pip install --upgrade pip
.venv/bin/python -m pip install -r requirements.txt

pnpm --dir "${WORKSPACE_ROOT}" install --frozen-lockfile
