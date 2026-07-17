#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

BACKEND_HOST="${BACKEND_HOST:-0.0.0.0}"
BACKEND_PORT="${BACKEND_PORT:-10086}"
FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
WAIT_HOST="${WAIT_HOST:-127.0.0.1}"
PYTHON_BIN="${PYTHON_BIN:-}"
SERVER_PID=""
CLIENT_PID=""

stop_port() {
  local port="$1"
  local pid
  pid=$(lsof -tiTCP:"${port}" -sTCP:LISTEN | head -n1 || true)
  if [ -n "${pid}" ]; then
    echo "⚠️ 端口 ${port} 已被占用，停止进程 ${pid}"
    kill "${pid}" || true
    sleep 1
  fi
}

wait_for_url() {
  local name="$1"
  local url="$2"
  local retries="${3:-30}"
  local i
  for ((i=1; i<=retries; i++)); do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      echo "✓ ${name}已就绪: ${url}"
      return 0
    fi
    sleep 1
  done
  echo "❌ ${name}启动失败: ${url}"
  return 1
}

resolve_python_bin() {
  if [ -n "${PYTHON_BIN}" ]; then
    return 0
  fi

  if [ -x "${SCRIPT_DIR}/.venv/bin/python3" ]; then
    PYTHON_BIN="${SCRIPT_DIR}/.venv/bin/python3"
    return 0
  fi

  if [ -x "${SCRIPT_DIR}/.venv/bin/python" ]; then
    PYTHON_BIN="${SCRIPT_DIR}/.venv/bin/python"
    return 0
  fi

  if command -v python3 >/dev/null 2>&1; then
    PYTHON_BIN="$(command -v python3)"
    return 0
  fi

  return 1
}

cleanup() {
  if [ -n "${SERVER_PID}" ]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  if [ -n "${CLIENT_PID}" ]; then
    kill "${CLIENT_PID}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT INT TERM

echo "🚀 启动刷题系统..."

if ! resolve_python_bin; then
  echo "❌ 需要安装 Python 3"
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "❌ 需要安装 Node.js 和 npm"
  exit 1
fi

if ! command -v lsof >/dev/null 2>&1; then
  echo "❌ 需要安装 lsof"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "❌ 需要安装 curl"
  exit 1
fi

if ! "${PYTHON_BIN}" -m pip --version >/dev/null 2>&1; then
  echo "❌ 当前 Python 环境缺少 pip: ${PYTHON_BIN}"
  exit 1
fi

echo "📦 检查后端依赖..."
if ! "${PYTHON_BIN}" -m pip install -q -r requirements.txt; then
  echo "❌ Python 依赖安装失败，请优先使用项目内 .venv，或通过 PYTHON_BIN 指定可写 Python 环境"
  exit 1
fi

stop_port "${BACKEND_PORT}"
stop_port "${FRONTEND_PORT}"

echo "🔧 启动后端服务..."
APP_HOST="${BACKEND_HOST}" APP_PORT="${BACKEND_PORT}" "${PYTHON_BIN}" server.py > server.log 2>&1 &
SERVER_PID=$!
wait_for_url "后端" "http://${WAIT_HOST}:${BACKEND_PORT}/api/banks"

echo "🌐 启动前端..."
cd web-app
echo "📦 同步前端依赖..."
npm install --include=dev

npm run dev -- --host "${FRONTEND_HOST}" --port "${FRONTEND_PORT}" > dev.log 2>&1 &
CLIENT_PID=$!
wait_for_url "前端" "http://${WAIT_HOST}:${FRONTEND_PORT}/"

echo ""
echo "✅ 刷题系统已启动！"
echo "   前端: http://${WAIT_HOST}:${FRONTEND_PORT}"
echo "   后端: http://${WAIT_HOST}:${BACKEND_PORT}"
echo "   API文档: http://${WAIT_HOST}:${BACKEND_PORT}/docs"
echo ""
echo "按 Ctrl+C 停止服务"
wait
