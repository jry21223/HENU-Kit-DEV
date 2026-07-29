#!/usr/bin/env bash
set -Eeuo pipefail

: "${CONFIRM_CUTOVER_SWITCH:?set CONFIRM_CUTOVER_SWITCH=yes}"
[[ "$CONFIRM_CUTOVER_SWITCH" == "yes" ]]

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
action="${1:-}"
release_sha="${2:-}"
static_release_id="${3:-${2:-}-writes}"
go_release_root="${GO_RELEASE_ROOT:-/opt/quizcraft-go/releases}"
go_current_link="${GO_CURRENT_LINK:-/opt/quizcraft-go/releases/current}"
static_release_root="${STATIC_RELEASE_ROOT:-/var/www/quizcraft-releases}"
static_current_link="${STATIC_CURRENT_LINK:-/var/www/quizcraft-current}"
state_dir="${CUTOVER_STATE_DIR:-/var/lib/quizcraft-cutover}"
maintenance_marker="${QUIZCRAFT_MAINTENANCE_MARKER:-$state_dir/maintenance-enabled}"
service_name="${QUIZCRAFT_GO_SERVICE:-quizcraft-go.service}"
nginx_config="${QUIZCRAFT_NGINX_CONFIG:-/etc/nginx/sites-enabled/superhuazai.me}"
go_health_url="${QUIZCRAFT_GO_HEALTH_URL:-http://127.0.0.1:10089/healthz}"
writes_env_file="${QUIZCRAFT_GO_ENV_FILE:-/etc/quizcraft-go.env}"
cutover_verify_script="${CUTOVER_VERIFY_SCRIPT:-$script_dir/verify-cutover.sh}"

portable_replace() {
  local source="$1" target="$2"
  if mv --help 2>&1 | grep -q -- ' --no-target-directory'; then
    mv -Tf "$source" "$target"
  else
    mv -fh "$source" "$target"
  fi
}

atomic_link() {
  local target="$1" link="$2" next_link="${2}.next.$$"
  ln -s "$target" "$next_link"
  portable_replace "$next_link" "$link"
}

atomic_state() {
  local value="$1" path="$2" next_path="${2}.next.$$"
  printf '%s\n' "$value" > "$next_path"
  portable_replace "$next_path" "$path"
}

enable_maintenance() {
  install -d -m 700 "$(dirname "$maintenance_marker")"
  atomic_state enabled "$maintenance_marker"
  nginx -t
  systemctl reload nginx
}

disable_maintenance() {
  nginx -t
  systemctl reload nginx
  if [[ -e "$maintenance_marker" ]]; then unlink "$maintenance_marker"; fi
}

validate_target() {
  local target="$1" root="$2"
  [[ "$target" == "$root"/* && -d "$target" ]]
}

read_writes_flag() {
  local values
  [[ -f "$writes_env_file" ]]
  values="$(sed -n 's/^QUIZCRAFT_WRITES_ENABLED=\([01]\)$/\1/p' "$writes_env_file")"
  [[ "$values" =~ ^[01]$ ]]
  printf '%s\n' "$values"
}

wait_for_go_health() {
  local attempt
  for attempt in {1..20}; do
    if curl --fail --silent --show-error --max-time 1 "$go_health_url" >/dev/null; then
      return 0
    fi
    sleep 0.25
  done
  echo "QuizCraft Go health check did not become ready after restart" >&2
  return 1
}

set_writes_flag() {
  python3 - "$writes_env_file" "$1" <<'PY'
import os
import pathlib
import re
import sys
import tempfile

path = pathlib.Path(sys.argv[1])
value = sys.argv[2]
stat = path.stat()
updated, count = re.subn(
    r"(?m)^QUIZCRAFT_WRITES_ENABLED=[01]$",
    f"QUIZCRAFT_WRITES_ENABLED={value}",
    path.read_text(),
)
if count != 1:
    raise SystemExit("Go environment must contain exactly one QUIZCRAFT_WRITES_ENABLED=0|1 line")
fd, temporary = tempfile.mkstemp(prefix=".quizcraft-go-env.", dir=path.parent)
try:
    os.fchmod(fd, stat.st_mode & 0o7777)
    os.fchown(fd, stat.st_uid, stat.st_gid)
    with os.fdopen(fd, "w") as stream:
        stream.write(updated)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)
finally:
    if os.path.exists(temporary):
        os.unlink(temporary)
PY
}

preflight_activation() {
  [[ "$release_sha" =~ ^[0-9a-f]{40}$ ]]
  [[ "$static_release_id" == "$release_sha-writes" ]]
  next_go="$go_release_root/$release_sha"
  next_static="$static_release_root/$static_release_id"
  validate_target "$next_go" "$go_release_root"
  validate_target "$next_static" "$static_release_root"
  [[ -x "$next_go/quizcraft-server" && -f "$next_static/index.html" ]]
  current_writes="$(read_writes_flag)"
  [[ "$current_writes" == "0" ]]
  next_phase=writes
  [[ "$action" == "activate-writes" && -x "$cutover_verify_script" ]]
}

record_previous_release() {
  install -d -m 700 "$state_dir"
  if [[ -L "$go_current_link" ]]; then readlink -f "$go_current_link" > "$state_dir/previous-go"; else echo ABSENT > "$state_dir/previous-go"; fi
  if [[ -L "$static_current_link" ]]; then readlink -f "$static_current_link" > "$state_dir/previous-static"; else echo ABSENT > "$state_dir/previous-static"; fi
  if [[ -f "$state_dir/active-phase" ]]; then cp "$state_dir/active-phase" "$state_dir/previous-phase"; else echo legacy > "$state_dir/previous-phase"; fi
  printf '%s\n' "$current_writes" > "$state_dir/previous-writes"
  if [[ -e "$maintenance_marker" ]]; then echo enabled > "$state_dir/previous-maintenance"; else echo disabled > "$state_dir/previous-maintenance"; fi
}

activate_release() {
  atomic_link "$next_go" "$go_current_link"
  systemctl restart "$service_name"
  systemctl is-active --quiet "$service_name"
  wait_for_go_health

  # The complete stop-write/reconciliation/restore gate runs while Go still
  # rejects every mutation. A failure here must never cross the write promise.
  QUIZCRAFT_GO_ENV_FILE="$writes_env_file" EXPECTED_WRITES_ENABLED=false "$cutover_verify_script"

  atomic_state "activating-writes:$next_static" "$state_dir/active-phase"
  set_writes_flag 1
  systemctl restart "$service_name"
  systemctl is-active --quiet "$service_name"
  wait_for_go_health
  QUIZCRAFT_GO_ENV_FILE="$writes_env_file" EXPECTED_WRITES_ENABLED=true "$cutover_verify_script"
  nginx -t
  systemctl reload nginx
  atomic_link "$next_static" "$static_current_link"
  atomic_state "$next_phase" "$state_dir/active-phase"
  disable_maintenance
}

rollback_release() {
  local active_phase previous_go previous_static previous_writes
  active_phase="$(cat "$state_dir/active-phase" 2>/dev/null || echo legacy)"
  if [[ "$active_phase" == "writes" ]]; then
    echo "post-write rollback to a read or legacy release is forbidden; activate a verified write-capable release or enter maintenance" >&2
    return 3
  fi
  previous_go="$(cat "$state_dir/previous-go")"
  previous_static="$(cat "$state_dir/previous-static")"
  previous_writes="$(cat "$state_dir/previous-writes")"
  set_writes_flag "$previous_writes"
  if [[ "$previous_go" == "ABSENT" ]]; then [[ ! -e "$go_current_link" && ! -L "$go_current_link" ]] || unlink "$go_current_link"; else validate_target "$previous_go" "$go_release_root"; atomic_link "$previous_go" "$go_current_link"; fi
  if [[ "$previous_static" == "ABSENT" ]]; then [[ ! -e "$static_current_link" && ! -L "$static_current_link" ]] || unlink "$static_current_link"; else validate_target "$previous_static" "$static_release_root"; atomic_link "$previous_static" "$static_current_link"; fi
  if [[ -n "${NGINX_ROLLBACK_CONFIG:-}" ]]; then
    [[ "$NGINX_ROLLBACK_CONFIG" == /var/backups/quizcraft/* && -f "$NGINX_ROLLBACK_CONFIG" ]]
    install -m 644 "$NGINX_ROLLBACK_CONFIG" "$nginx_config"
  fi
  cp "$state_dir/previous-phase" "$state_dir/active-phase"
  if [[ "$(cat "$state_dir/previous-maintenance")" == "enabled" ]]; then enable_maintenance; else disable_maintenance; fi
}

restore_runtime() {
  nginx -t
  if [[ -L "$go_current_link" ]]; then
    systemctl restart "$service_name"
    systemctl is-active --quiet "$service_name"
  else
    systemctl stop "$service_name"
  fi
  systemctl reload nginx
}

activate_with_rollback() {
  preflight_activation
  record_previous_release
  enable_maintenance
  nginx -t
  trap 'handle_activation_error $?' ERR
  activate_release
  trap - ERR
}

handle_activation_error() {
  local error_status="$1" active_phase
  trap - ERR
  set +e
  active_phase="$(cat "$state_dir/active-phase" 2>/dev/null || echo legacy)"
  if [[ "$active_phase" == "writes" ]] || [[ "$active_phase" == activating-writes:* && "$(read_writes_flag 2>/dev/null)" == "1" ]]; then
    echo "Go writes were enabled, so the write promise may have been crossed; preserving maintenance and the write-capable state for forward repair" >&2
    exit "$error_status"
  fi
  rollback_release
  restore_runtime
  exit "$error_status"
}

recover_incomplete_write_activation() {
  local active_phase pending_static current_static
  active_phase="$(cat "$state_dir/active-phase" 2>/dev/null || echo legacy)"
  [[ "$active_phase" == activating-writes:* ]] || return 0
  pending_static="${active_phase#activating-writes:}"
  current_static="$(readlink -f "$static_current_link" 2>/dev/null || true)"
  if validate_target "$pending_static" "$static_release_root" && [[ "$current_static" == "$pending_static" && "$(read_writes_flag)" == "1" ]]; then
    atomic_state writes "$state_dir/active-phase"
    disable_maintenance
    echo "Recovered an exposed QuizCraft write activation"
    return 0
  fi
  if [[ "$(read_writes_flag)" == "1" ]]; then
    if [[ "$action" == "resume-writes" ]]; then
      return 0
    fi
    echo "Go writes were enabled before the interrupted activation; automatic rollback is forbidden, keep maintenance active and forward-fix" >&2
    return 4
  fi
  rollback_release
  restore_runtime
  echo "Recovered an unexposed QuizCraft write activation by rolling it back"
}

resume_write_activation() {
  [[ "$release_sha" =~ ^[0-9a-f]{40}$ ]]
  [[ "$static_release_id" == "$release_sha-writes" ]]
  next_go="$go_release_root/$release_sha"
  next_static="$static_release_root/$static_release_id"
  validate_target "$next_go" "$go_release_root"
  validate_target "$next_static" "$static_release_root"
  [[ "$(readlink -f "$go_current_link")" == "$next_go" ]]
  [[ "$(read_writes_flag)" == "1" ]]
  active_phase="$(cat "$state_dir/active-phase" 2>/dev/null || echo legacy)"
  if [[ "$active_phase" == "writes" ]]; then
    [[ "$(readlink -f "$static_current_link")" == "$next_static" ]]
    QUIZCRAFT_GO_ENV_FILE="$writes_env_file" EXPECTED_WRITES_ENABLED=true "$cutover_verify_script"
    disable_maintenance
    return 0
  fi
  [[ "$active_phase" == "activating-writes:$next_static" ]]
  [[ -e "$maintenance_marker" ]]
  QUIZCRAFT_GO_ENV_FILE="$writes_env_file" EXPECTED_WRITES_ENABLED=true "$cutover_verify_script"
  nginx -t
  systemctl reload nginx
  atomic_link "$next_static" "$static_current_link"
  atomic_state writes "$state_dir/active-phase"
  disable_maintenance
}

recover_incomplete_write_activation

case "$action" in
  activate-writes) activate_with_rollback ;;
  resume-writes) resume_write_activation ;;
  rollback) rollback_release; restore_runtime ;;
  *) echo "usage: $0 activate-writes|resume-writes <40-char-release-sha> <sha-writes-static-release-id> | rollback" >&2; exit 2 ;;
esac

echo "QuizCraft $action completed"
