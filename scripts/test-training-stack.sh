#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export TRAINING_STACK_DIRECTORY="$(mktemp -d "${TMPDIR:-/tmp}/training-stack.XXXXXX")"
export TRAINING_CORE_PORT="${TRAINING_CORE_PORT:-13642}"
export TRAINING_GATEWAY_PORT="${TRAINING_GATEWAY_PORT:-13643}"
export PLAYWRIGHT_TRAINING_PORT="${PLAYWRIGHT_TRAINING_PORT:-3165}"
export PLAYWRIGHT_TRAINING_STACK=1
export PLAYWRIGHT_PORTAL_GATEWAY_URL="http://127.0.0.1:$TRAINING_GATEWAY_PORT"
core_pid="";gateway_pid=""
cleanup(){
 touch "$TRAINING_STACK_DIRECTORY/stop"
 if [[ -n "$core_pid" ]];then wait "$core_pid" || true;fi
 if [[ -n "$gateway_pid" ]];then wait "$gateway_pid" || true;fi
}
trap cleanup EXIT
(cd "$repo_root/products/quizcraft/go-service" && go test -c -o "$TRAINING_STACK_DIRECTORY/core.test" ./tests)
(cd "$repo_root/services/portal-gateway" && go test -c -o "$TRAINING_STACK_DIRECTORY/gateway.test" ./internal/httpapi)
(cd "$repo_root/products/quizcraft/go-service/tests" && "$TRAINING_STACK_DIRECTORY/core.test" -test.run '^TestTrainingBrowserCoreHarness$' -test.v) > "$TRAINING_STACK_DIRECTORY/core.log" 2>&1 &
core_pid=$!
(cd "$repo_root/services/portal-gateway/internal/httpapi" && "$TRAINING_STACK_DIRECTORY/gateway.test" -test.run '^TestTrainingBrowserGatewayHarness$' -test.v) > "$TRAINING_STACK_DIRECTORY/gateway.log" 2>&1 &
gateway_pid=$!
for attempt in $(seq 1 120);do
 if [[ -f "$TRAINING_STACK_DIRECTORY/core-ready" && -f "$TRAINING_STACK_DIRECTORY/gateway-ready" ]];then break;fi
 if ! kill -0 "$core_pid" 2>/dev/null || ! kill -0 "$gateway_pid" 2>/dev/null;then cat "$TRAINING_STACK_DIRECTORY/core.log" "$TRAINING_STACK_DIRECTORY/gateway.log";exit 1;fi
 if [[ "$attempt" == 120 ]];then cat "$TRAINING_STACK_DIRECTORY/core.log" "$TRAINING_STACK_DIRECTORY/gateway.log";exit 1;fi
 sleep 0.5
done
printf 'Training stack evidence: %s\n' "$TRAINING_STACK_DIRECTORY"
cd "$repo_root/apps/portal"
corepack pnpm --config.verify-deps-before-run=false exec playwright test --config playwright.training.config.ts "$@"
