#!/usr/bin/env bash
# 求职雷达（Career）smoke 证据脚本（#404）。
#
# 启动 testcontainers（PostgreSQL + Redis）→ 走完整 Lifetime 闭环：
# 配置画像 → 创建 search → 驱动 worker → 断言 completed → 结果落库 →
# digest 邮件入队 → 状态/历史可读。输出 tee 到 docs/career-smoke-evidence.txt。
#
# 用法：
#   scripts/dev/career-smoke.sh            # 单条 Smoke 闭环（默认）
#   scripts/dev/career-smoke.sh --all      # 整个 career 服务测试套件
#
# 依赖：docker（testcontainers 自动起库）、go。

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVIDENCE_FILE="$REPO_ROOT/docs/career-smoke-evidence.txt"

if ! docker info >/dev/null 2>&1; then
  echo "error: docker 未运行，无法启动 testcontainers" >&2
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "error: go 未安装" >&2
  exit 1
fi

cd "$REPO_ROOT/services/career-opportunities"

RUN_FILTER="-run=TestSmoke"
if [[ "${1:-}" == "--all" ]]; then
  RUN_FILTER=""
fi

echo "==> career smoke 开始 $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "==> 目录：$PWD"
{
  echo "# Career smoke evidence — #404 端到端验收"
  echo
  echo "运行时间：$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "运行方式：go test -count=1 -v ${RUN_FILTER:-（全部用例）} ./tests/"
  echo "覆盖闭环：画像 PUT → search 创建(queued) → worker Step → completed → 结果落库 → digest 入队 → 状态/历史读取"
  echo
} | tee "$EVIDENCE_FILE"

if ! go test -count=1 -v $RUN_FILTER ./tests/ 2>&1 | tee -a "$EVIDENCE_FILE"; then
  echo "==> smoke 失败：见 $EVIDENCE_FILE" >&2
  exit 1
fi

echo "==> smoke 通过，证据已写入 $EVIDENCE_FILE"
