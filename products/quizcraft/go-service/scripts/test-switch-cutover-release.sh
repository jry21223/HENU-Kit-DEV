#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
subject="$script_dir/switch-cutover-release.sh"
test_root="$(mktemp -d)"
test_root="$(cd "$test_root" && pwd -P)"
trap 'rm -rf "$test_root"' EXIT
old_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
new_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
write_release="$new_sha-writes"

mkdir -p "$test_root/bin" "$test_root/go/$old_sha" "$test_root/go/$new_sha" \
  "$test_root/static/$old_sha" "$test_root/static/$write_release" "$test_root/state"
touch "$test_root/go/$old_sha/quizcraft-server" "$test_root/go/$new_sha/quizcraft-server"
chmod +x "$test_root/go/$old_sha/quizcraft-server" "$test_root/go/$new_sha/quizcraft-server"
touch "$test_root/static/$old_sha/index.html" "$test_root/static/$write_release/index.html" "$test_root/nginx.conf"
printf 'QUIZCRAFT_WRITES_ENABLED=0\n' > "$test_root/quizcraft-go.env"
ln -s "$test_root/go/$old_sha" "$test_root/go/current"
ln -s "$test_root/static/$old_sha" "$test_root/static/current"

cat > "$test_root/bin/nginx" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$test_root/bin/curl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$test_root/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
if [[ "${CRASH_AFTER_WRITE_FLAG:-0}" == "1" && "${1:-}" == "restart" ]] && grep -qx 'QUIZCRAFT_WRITES_ENABLED=1' "$QUIZCRAFT_GO_ENV_FILE"; then
  kill -KILL "$PPID"
  exit 137
fi
exit 0
EOF
cat > "$test_root/bin/mv" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "--help" ]]; then exec /bin/mv "$@"; fi
arguments=("$@")
source_path="${arguments[${#arguments[@]}-2]}"
target_path="${arguments[${#arguments[@]}-1]}"
if [[ "${FAIL_PHASE_COMMIT:-0}" == "1" && "$target_path" == */active-phase && "$(cat "$source_path")" == "writes" ]]; then exit 1; fi
exec /bin/mv "$@"
EOF
cat > "$test_root/bin/verify-cutover" <<'EOF'
#!/usr/bin/env bash
flag="$(sed -n 's/^QUIZCRAFT_WRITES_ENABLED=//p' "$QUIZCRAFT_GO_ENV_FILE")"
if [[ "$EXPECTED_WRITES_ENABLED" == "false" ]]; then
  [[ "$flag" == "0" ]]
  printf 'preflight\n' >> "$VERIFY_LOG"
  [[ "${FAIL_PREFLIGHT:-0}" != "1" ]]
else
  [[ "$flag" == "1" ]]
  printf 'post-activation\n' >> "$VERIFY_LOG"
  [[ "${FAIL_POST_VERIFY:-0}" != "1" ]]
fi
EOF
chmod +x "$test_root/bin/"*

run_switch() {
  PATH="$test_root/bin:$PATH" \
  VERIFY_LOG="$test_root/verify.log" \
  FAIL_PREFLIGHT="${FAIL_PREFLIGHT:-0}" \
  FAIL_POST_VERIFY="${FAIL_POST_VERIFY:-0}" \
  FAIL_PHASE_COMMIT="${FAIL_PHASE_COMMIT:-0}" \
  CRASH_AFTER_WRITE_FLAG="${CRASH_AFTER_WRITE_FLAG:-0}" \
  CONFIRM_CUTOVER_SWITCH=yes \
  GO_RELEASE_ROOT="$test_root/go" GO_CURRENT_LINK="$test_root/go/current" \
  STATIC_RELEASE_ROOT="$test_root/static" STATIC_CURRENT_LINK="$test_root/static/current" \
  CUTOVER_STATE_DIR="$test_root/state" QUIZCRAFT_NGINX_CONFIG="$test_root/nginx.conf" \
  QUIZCRAFT_GO_ENV_FILE="$test_root/quizcraft-go.env" \
  CUTOVER_VERIFY_SCRIPT="$test_root/bin/verify-cutover" \
    "$subject" "$@"
}

if run_switch activate-writes invalid-sha; then
  echo "expected invalid SHA preflight failure" >&2
  exit 1
fi

if FAIL_PREFLIGHT=1 run_switch activate-writes "$new_sha" "$write_release"; then
  echo "expected stop-write preflight failure" >&2
  exit 1
fi
grep -qx 'QUIZCRAFT_WRITES_ENABLED=0' "$test_root/quizcraft-go.env"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$old_sha"

: > "$test_root/verify.log"
run_switch activate-writes "$new_sha" "$write_release"
test "$(cat "$test_root/verify.log")" = $'preflight\npost-activation'
test "$(readlink -f "$test_root/go/current")" = "$test_root/go/$new_sha"
test "$(readlink -f "$test_root/static/current")" = "$test_root/static/$write_release"
grep -qx 'QUIZCRAFT_WRITES_ENABLED=1' "$test_root/quizcraft-go.env"
grep -qx 'writes' "$test_root/state/active-phase"

if run_switch rollback; then
  echo "expected post-write rollback refusal" >&2
  exit 1
fi

echo "switch-cutover-release tests passed"
