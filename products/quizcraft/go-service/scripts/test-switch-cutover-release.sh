#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/switch-cutover-release.sh"
test_root="$(mktemp -d)"
test_root="$(cd "$test_root" && pwd -P)"
trap 'rm -rf "$test_root"' EXIT
old_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
new_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

read_release="$new_sha-read5"
write_release="$new_sha-writes"
mkdir -p "$test_root/bin" "$test_root/go/$old_sha" "$test_root/go/$new_sha" \
  "$test_root/static/$old_sha" "$test_root/static/$read_release" "$test_root/static/$write_release" "$test_root/state"
touch "$test_root/go/$old_sha/quizcraft-server" "$test_root/go/$new_sha/quizcraft-server"
chmod +x "$test_root/go/$old_sha/quizcraft-server" "$test_root/go/$new_sha/quizcraft-server"
touch "$test_root/static/$old_sha/index.html" "$test_root/static/$read_release/index.html" \
  "$test_root/static/$write_release/index.html" "$test_root/nginx.conf"
printf 'QUIZCRAFT_WRITES_ENABLED=0\n' > "$test_root/quizcraft-go.env"
ln -s "$test_root/go/$old_sha" "$test_root/go/current"
ln -s "$test_root/static/$old_sha" "$test_root/static/current"

cat > "$test_root/bin/nginx" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$test_root/bin/curl" <<'EOF'
#!/usr/bin/env bash
count="$(cat "$HEALTH_COUNTER_FILE" 2>/dev/null || echo 0)"
count=$((count + 1))
printf '%s\n' "$count" > "$HEALTH_COUNTER_FILE"
if (( count <= ${FAIL_HEALTH_ATTEMPTS:-0} )); then
  exit 1
fi
[[ "${FAIL_HEALTH:-0}" != "1" ]]
EOF
cat > "$test_root/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
if [[ "${CRASH_AFTER_WRITE_FLAG:-0}" == "1" && "${1:-}" == "restart" ]]; then
  kill -KILL "$PPID"
  exit 137
fi
exit 0
EOF
cat > "$test_root/bin/mv" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "--help" ]]; then
  exec /bin/mv "$@"
fi
arguments=("$@")
source_path="${arguments[${#arguments[@]}-2]}"
target_path="${arguments[${#arguments[@]}-1]}"
if [[ "${FAIL_PHASE_COMMIT:-0}" == "1" && "$target_path" == */active-phase && "$(cat "$source_path")" == "writes" ]]; then
  exit 1
fi
exec /bin/mv "$@"
EOF
cat > "$test_root/bin/verify-cutover" <<'EOF'
#!/usr/bin/env bash
[[ "${FAIL_VERIFY:-0}" != "1" ]]
EOF
chmod +x "$test_root/bin/nginx" "$test_root/bin/curl" "$test_root/bin/systemctl" "$test_root/bin/mv" "$test_root/bin/verify-cutover"

run_switch() {
  PATH="$test_root/bin:$PATH" \
  FAIL_HEALTH="${FAIL_HEALTH:-0}" \
  FAIL_HEALTH_ATTEMPTS="${FAIL_HEALTH_ATTEMPTS:-0}" \
  HEALTH_COUNTER_FILE="$test_root/health-attempts" \
  FAIL_VERIFY="${FAIL_VERIFY:-0}" \
  FAIL_PHASE_COMMIT="${FAIL_PHASE_COMMIT:-0}" \
  CRASH_AFTER_WRITE_FLAG="${CRASH_AFTER_WRITE_FLAG:-0}" \
  CONFIRM_CUTOVER_SWITCH=yes \
  GO_RELEASE_ROOT="$test_root/go" \
  GO_CURRENT_LINK="$test_root/go/current" \
  STATIC_RELEASE_ROOT="$test_root/static" \
  STATIC_CURRENT_LINK="$test_root/static/current" \
  CUTOVER_STATE_DIR="$test_root/state" \
  QUIZCRAFT_NGINX_CONFIG="$test_root/nginx.conf" \
  QUIZCRAFT_GO_ENV_FILE="$test_root/quizcraft-go.env" \
  CUTOVER_VERIFY_SCRIPT="$test_root/bin/verify-cutover" \
    "$subject" "$@"
}

run_switch activate "$new_sha" "$read_release"
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$new_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$read_release"

run_switch rollback
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$old_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$old_sha"

if run_switch activate invalid-sha; then
  echo "expected invalid SHA preflight failure" >&2
  exit 1
fi
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$old_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$old_sha"

missing_sha=cccccccccccccccccccccccccccccccccccccccc
if run_switch activate "$missing_sha"; then
  echo "expected missing artifact preflight failure" >&2
  exit 1
fi
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$old_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$old_sha"

printf '0\n' > "$test_root/health-attempts"
FAIL_HEALTH_ATTEMPTS=2 run_switch activate "$new_sha" "$read_release"
test "$(cat "$test_root/health-attempts")" = 3
run_switch rollback

if FAIL_HEALTH=1 run_switch activate "$new_sha" "$read_release"; then
  echo "expected injected health failure" >&2
  exit 1
fi
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$old_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$old_sha"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=0' "$test_root/quizcraft-go.env"

run_switch activate "$new_sha" "$read_release"
if FAIL_VERIFY=1 run_switch activate-writes "$new_sha" "$write_release"; then
  echo "expected write verification failure" >&2
  exit 1
fi
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$new_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$read_release"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=0' "$test_root/quizcraft-go.env"

if CRASH_AFTER_WRITE_FLAG=1 run_switch activate-writes "$new_sha" "$write_release"; then
  echo "expected crash after enabling the write flag" >&2
  exit 1
fi
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$read_release"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=1' "$test_root/quizcraft-go.env"
grep -qx "activating-writes:$test_root/static/$write_release" "$test_root/state/active-phase"
run_switch rollback
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$read_release"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=0' "$test_root/quizcraft-go.env"

if FAIL_PHASE_COMMIT=1 run_switch activate-writes "$new_sha" "$write_release"; then
  echo "expected post-exposure phase commit failure" >&2
  exit 1
fi
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$write_release"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=1' "$test_root/quizcraft-go.env"
grep -qx "activating-writes:$test_root/static/$write_release" "$test_root/state/active-phase"
if run_switch rollback; then
  echo "expected post-write rollback refusal" >&2
  exit 1
fi
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$new_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$write_release"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=1' "$test_root/quizcraft-go.env"
grep -qx 'writes' "$test_root/state/active-phase"

echo "switch-cutover-release tests passed"
