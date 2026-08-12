#!/usr/bin/env bash
# Transfer one already-built, signed release directly from WSL2 to production,
# then invoke the existing exact-SHA production activation entry. This script
# never builds images and never routes artifacts through macOS.
set -Eeuo pipefail

program="deploy-henukit-release-from-wsl"
production_alias="henu-prod"
remote_incoming_root="/opt/henukit-incoming"

usage() {
  cat >&2 <<'EOF'
usage: deploy-henukit-release-from-wsl.sh \
  --sha <full-main-sha> \
  --artifact-dir <signed-flat-directory> \
  --allowed-signers <trusted-public-signers-file> \
  --remote-env-file <absolute-production-env-path> \
  --account-operator-role <role-code> \
  [--platform-migrations <comma-separated-reviewed-files>] \
  [--recover-degraded-baseline <full-current-sha>] \
  [--adopt-degraded-baseline-owner <historical-owner-uid>] \
  --preflight|--execute

Run this only from the WSL2 deployment identity that owns the henu-prod SSH
alias. The release must already have been built and signed by the separate
builder identity. --preflight is read-only. --execute transfers the immutable
flat bundle directly to production and invokes the existing activation entry.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

file_mode() {
  stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
}

trusted_local_file() {
  local file="$1"
  local label="$2"
  local mode
  [[ "$file" == /* ]] || die "$label must use an absolute path"
  [[ -f "$file" && -r "$file" && ! -L "$file" ]] ||
    die "$label must be a readable regular non-symlink file"
  mode="$(file_mode "$file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || die "could not determine $label mode"
  (( (8#$mode & 8#022) == 0 )) ||
    die "$label must not be group- or world-writable"
}

release_sha=""
artifact_dir=""
allowed_signers=""
remote_env_file=""
account_operator_role=""
platform_migrations=""
recovery_baseline_sha=""
adopt_degraded_baseline_owner=""
mode=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      release_sha="$2"
      shift 2
      ;;
    --artifact-dir)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      artifact_dir="$2"
      shift 2
      ;;
    --allowed-signers)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      allowed_signers="$2"
      shift 2
      ;;
    --remote-env-file)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      remote_env_file="$2"
      shift 2
      ;;
    --account-operator-role)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      account_operator_role="$2"
      shift 2
      ;;
    --platform-migrations)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      platform_migrations="$2"
      shift 2
      ;;
    --recover-degraded-baseline)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      recovery_baseline_sha="$2"
      shift 2
      ;;
    --adopt-degraded-baseline-owner)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      adopt_degraded_baseline_owner="$2"
      shift 2
      ;;
    --preflight|--execute)
      [[ -z "$mode" ]] || die "choose exactly one of --preflight or --execute"
      mode="$1"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
[[ -n "$artifact_dir" && -n "$allowed_signers" && -n "$remote_env_file" &&
   -n "$account_operator_role" && -n "$mode" ]] || { usage; exit 64; }
[[ "$remote_env_file" =~ ^/[A-Za-z0-9_./-]+$ && "$remote_env_file" != "/" ]] ||
  die "--remote-env-file must be a specific absolute path"
[[ "$account_operator_role" =~ ^[a-z0-9][a-z0-9-]{0,63}$ ]] ||
  die "--account-operator-role must use lowercase letters, digits, or hyphens"
if [[ -n "$platform_migrations" ]]; then
  [[ "$platform_migrations" =~ ^[0-9]{6}_[a-z0-9_]+\.up\.sql(,[0-9]{6}_[a-z0-9_]+\.up\.sql)*$ ]] ||
    die "--platform-migrations must be reviewed numbered .up.sql filenames"
fi
if [[ -n "$recovery_baseline_sha" ]]; then
  [[ "$recovery_baseline_sha" =~ ^[0-9a-f]{40}$ ]] ||
    die "--recover-degraded-baseline must be a full lowercase Git SHA"
  [[ "$recovery_baseline_sha" != "$release_sha" ]] ||
    die "recovery baseline and candidate SHA must differ"
  [[ "$mode" == "--execute" || "$mode" == "--preflight" ]] ||
    die "degraded-baseline recovery requires an explicit deployment mode"
fi
if [[ -n "$adopt_degraded_baseline_owner" ]]; then
  [[ -n "$recovery_baseline_sha" ]] ||
    die "--adopt-degraded-baseline-owner requires --recover-degraded-baseline"
  [[ "$adopt_degraded_baseline_owner" =~ ^[1-9][0-9]*$ ]] ||
    die "--adopt-degraded-baseline-owner must be a non-root numeric UID"
fi

[[ "$(uname -s)" == "Linux" && "$(uname -m)" == "x86_64" ]] ||
  die "deployment transport must run on WSL2 Linux x86_64"
[[ "$(uname -r)" =~ [Ww][Ss][Ll]2 ]] ||
  die "deployment transport must run on WSL2"

command -v git >/dev/null 2>&1 || die "git is required"
command -v ssh >/dev/null 2>&1 || die "ssh is required"
if [[ "$mode" == "--execute" ]]; then
  command -v rsync >/dev/null 2>&1 || die "rsync is required"
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
inventory="$repo_root/scripts/ops/henukit-release-images.sh"
local_verifier="${HENUKIT_LOCAL_ARTIFACT_VERIFIER:-$repo_root/scripts/ops/verify-henukit-local-release.sh}"
trusted_local_file "$inventory" "release inventory"
trusted_local_file "$local_verifier" "local artifact verifier"
[[ -x "$inventory" && -x "$local_verifier" ]] ||
  die "release inventory and local artifact verifier must be executable"
trusted_local_file "$allowed_signers" "allowed signers"

[[ -d "$artifact_dir" && ! -L "$artifact_dir" ]] ||
  die "--artifact-dir must be a non-symlink directory"
artifact_dir="$(cd "$artifact_dir" && pwd -P)"
[[ -r "$artifact_dir/RELEASE_SHA" ]] || die "artifact directory has no RELEASE_SHA"
[[ "$(tr -d '[:space:]' < "$artifact_dir/RELEASE_SHA")" == "$release_sha" ]] ||
  die "artifact RELEASE_SHA does not match --sha"

[[ "$(git -C "$repo_root" rev-parse HEAD)" == "$release_sha" ]] ||
  die "deployment checkout HEAD does not match --sha"
[[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
  die "deployment checkout must be clean, including untracked files"
remote_main_sha="$(git -C "$repo_root" ls-remote --exit-code origin refs/heads/main | awk 'NR == 1 { print $1 }')"
[[ "$remote_main_sha" == "$release_sha" ]] ||
  die "requested release is not the current origin/main head"

"$local_verifier" \
  --artifact-dir "$artifact_dir" \
  --sha "$release_sha" \
  --inventory "$inventory" \
  --allowed-signers "$allowed_signers" >/dev/null

ssh_options=(-o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10)
resolved_config="$(ssh -G "${ssh_options[@]}" "$production_alias" 2>/dev/null)" ||
  die "could not resolve the $production_alias SSH alias"
resolved_host="$(awk '$1 == "hostname" { print $2; exit }' <<< "$resolved_config")"
resolved_user="$(awk '$1 == "user" { print $2; exit }' <<< "$resolved_config")"
resolved_port="$(awk '$1 == "port" { print $2; exit }' <<< "$resolved_config")"
resolved_proxy_jump="$(awk '$1 == "proxyjump" { print $2; exit }' <<< "$resolved_config")"
resolved_proxy_command="$(awk '$1 == "proxycommand" { $1 = ""; sub(/^ /, ""); print; exit }' <<< "$resolved_config")"
[[ -n "$resolved_host" && "$resolved_host" != "$production_alias" ]] ||
  die "$production_alias is not a configured alias"
[[ "$resolved_user" == "root" ]] || die "$production_alias must resolve to the approved root deployment identity"
[[ "$resolved_port" =~ ^[1-9][0-9]{0,4}$ && "$resolved_port" -le 65535 ]] ||
  die "$production_alias resolves to an invalid SSH port"
[[ -z "$resolved_proxy_jump" || "$resolved_proxy_jump" == "none" ]] ||
  die "$production_alias must connect directly without ProxyJump"
[[ -z "$resolved_proxy_command" || "$resolved_proxy_command" == "none" ]] ||
  die "$production_alias must connect directly without ProxyCommand"
ssh_options+=(-o ProxyJump=none -o ProxyCommand=none)

remote_release_dir="$remote_incoming_root/henukit-release-$release_sha"
remote_state_file="$(mktemp "${TMPDIR:-/tmp}/henukit-remote-state.XXXXXX")"
chmod 0600 "$remote_state_file"
if ! ssh "${ssh_options[@]}" "$production_alias" sh -s -- \
  "$remote_env_file" "$remote_incoming_root" "$remote_release_dir" "$release_sha" \
  "${recovery_baseline_sha:--}" "${adopt_degraded_baseline_owner:--}" \
  >"$remote_state_file" <<'REMOTE_PREFLIGHT'
set -eu
remote_env_file="$1"
incoming_root="$2"
remote_release_dir="$3"
release_sha="$4"
case "$5" in
  -) recovery_baseline_sha="" ;;
  *) recovery_baseline_sha="$5" ;;
esac
case "$6" in
  -) adopt_degraded_baseline_owner="" ;;
  *) adopt_degraded_baseline_owner="$6" ;;
esac
trusted_root_file() {
  file="$1"
  case "$file" in /*) ;; *) return 1 ;; esac
  test -f "$file" && test -r "$file" && test ! -L "$file"
  test "$(stat -c '%u' "$file")" -eq 0
  mode="$(stat -c '%a' "$file")"
  test $((0$mode & 022)) -eq 0
  parent="$(dirname "$file")"
  while :; do
    test -d "$parent" && test ! -L "$parent"
    test "$(stat -c '%u' "$parent")" -eq 0
    mode="$(stat -c '%a' "$parent")"
    test $((0$mode & 022)) -eq 0
    test "$parent" = / && break
    next="$(dirname "$parent")"
    test "$next" != "$parent"
    parent="$next"
  done
}
trusted_root_directory() {
  directory="$1"
  case "$directory" in /*) ;; *) return 1 ;; esac
  while :; do
    test -d "$directory" && test ! -L "$directory"
    test "$(stat -c '%u' "$directory")" -eq 0
    mode="$(stat -c '%a' "$directory")"
    test $((0$mode & 022)) -eq 0
    test "$directory" = / && break
    next="$(dirname "$directory")"
    test "$next" != "$directory"
    directory="$next"
  done
}
test "$(id -u)" -eq 0
for helper in \
  /usr/local/sbin/watch-henukit-actions \
  /usr/local/sbin/activate-henukit-release \
  /usr/local/sbin/verify-henukit-local-release.sh \
  /usr/local/sbin/henukit-release-images.sh; do
  trusted_root_file "$helper"
  test -x "$helper"
done
if test -n "$recovery_baseline_sha"; then
  case "$recovery_baseline_sha" in
    *[!0-9a-f]*|'') exit 1 ;;
  esac
  test "${#recovery_baseline_sha}" -eq 40
  test -L /opt/henukit-current
  test "$(basename "$(readlink -f /opt/henukit-current)")" = "$recovery_baseline_sha"
  /usr/local/sbin/activate-henukit-release --help 2>&1 |
    grep -q -- '--recover-degraded-baseline'
fi
if test -n "$adopt_degraded_baseline_owner"; then
  case "$adopt_degraded_baseline_owner" in ''|0|*[!0-9]*) exit 1 ;; esac
  trusted_root_file /usr/local/sbin/adopt-henukit-degraded-baseline
  test -x /usr/local/sbin/adopt-henukit-degraded-baseline
  /usr/local/sbin/adopt-henukit-degraded-baseline \
    --sha "$recovery_baseline_sha" \
    --candidate-sha "$release_sha" \
    --expected-owner-uid "$adopt_degraded_baseline_owner" \
    --preflight >/dev/null
fi
trusted_root_file /etc/henukit/release-signers
trusted_root_file /etc/henukit/github-actions-read.token
trusted_root_file "$remote_env_file"
watcher_state_root=/var/lib/henukit-actions-watch
trusted_root_directory "$watcher_state_root"
if test -e "$watcher_state_root/quiesce.request"; then
  trusted_root_file "$watcher_state_root/quiesce.request"
  read -r quiesce_sha quiesce_instance quiesce_nonce quiesce_extra < "$watcher_state_root/quiesce.request"
  test "$quiesce_sha" = "$release_sha"
  case "$quiesce_instance" in ''|*[!0-9]*) exit 1 ;; esac
  case "$quiesce_nonce" in ''|*[!0-9a-f]*) exit 1 ;; esac
  test "${#quiesce_nonce}" -eq 32
  test -z "${quiesce_extra:-}"
fi
command -v systemctl >/dev/null 2>&1
command -v flock >/dev/null 2>&1
systemctl cat henukit-actions-watch.service >/dev/null
test -d "$incoming_root"
test ! -L "$incoming_root"
test "$(stat -c '%u' "$incoming_root")" -eq 0
mode="$(stat -c '%a' "$incoming_root")"
test $((0$mode & 022)) -eq 0
if test -e "$remote_release_dir"; then
  test -d "$remote_release_dir" && test ! -L "$remote_release_dir"
  /usr/local/sbin/verify-henukit-local-release.sh \
    --artifact-dir "$remote_release_dir" \
    --sha "$release_sha" \
    --inventory /usr/local/sbin/henukit-release-images.sh \
    --allowed-signers /etc/henukit/release-signers >/dev/null
  printf 'verified-existing\n'
else
  printf 'absent\n'
fi
REMOTE_PREFLIGHT
then
  rm -f -- "$remote_state_file"
  die "production preflight rejected the target trust roots or release state"
fi
remote_artifact_state="$(tr -d '\r\n' < "$remote_state_file")"
rm -f -- "$remote_state_file"
[[ "$remote_artifact_state" == "absent" || "$remote_artifact_state" == "verified-existing" ]] ||
  die "production preflight returned an invalid artifact state"

if [[ "$mode" == "--preflight" ]]; then
  printf '%s: preflight passed for release %s\n' "$program" "$release_sha"
  exit 0
fi

remote_stage=""

cleanup_remote_stage() {
  if [[ -n "${remote_stage:-}" ]]; then
    ssh "${ssh_options[@]}" "$production_alias" sh -s -- "$remote_stage" <<'REMOTE_CLEANUP' >/dev/null 2>&1 || true
set -eu
stage="$1"
case "$stage" in
  /opt/henukit-incoming/.incoming-[0-9a-f]*-[0-9]*-[0-9]*) rm -rf -- "$stage" ;;
  *) exit 64 ;;
esac
REMOTE_CLEANUP
  fi
}
trap cleanup_remote_stage EXIT

if [[ "$remote_artifact_state" == "absent" ]]; then
  remote_stage="$remote_incoming_root/.incoming-$release_sha-$$-$RANDOM"
  [[ "$remote_stage" =~ ^/opt/henukit-incoming/\.incoming-[0-9a-f]{40}-[0-9]+-[0-9]+$ ]] ||
    die "could not construct a safe remote staging path"

  ssh "${ssh_options[@]}" "$production_alias" sh -s -- \
    "$remote_incoming_root" "$remote_stage" <<'REMOTE_STAGE'
set -eu
incoming_root="$1"
stage="$2"
test -d "$incoming_root"
test ! -L "$incoming_root"
test "$(stat -c '%u' "$incoming_root")" -eq 0
mode="$(stat -c '%a' "$incoming_root")"
test $((0$mode & 022)) -eq 0
test ! -e "$stage"
install -d -o root -g root -m 0700 "$stage"
REMOTE_STAGE

  rsync_ssh="ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10 -o ProxyJump=none -o ProxyCommand=none"
  rsync \
    --archive \
    --chmod=D700,F400 \
    --protect-args \
    -e "$rsync_ssh" \
    "$artifact_dir/" \
    "$production_alias:$remote_stage/"

  remote_main_sha="$(git -C "$repo_root" ls-remote --exit-code origin refs/heads/main | awk 'NR == 1 { print $1 }')"
  [[ "$remote_main_sha" == "$release_sha" ]] ||
    die "origin/main changed during transfer; refusing to activate stale artifacts"

  ssh "${ssh_options[@]}" "$production_alias" sh -s -- \
    "$remote_stage" "$remote_release_dir" "$release_sha" <<'REMOTE_VERIFY'
set -eu
stage="$1"
release_dir="$2"
release_sha="$3"
trusted_root_file() {
  file="$1"
  case "$file" in /*) ;; *) return 1 ;; esac
  test -f "$file" && test -r "$file" && test ! -L "$file"
  test "$(stat -c '%u' "$file")" -eq 0
  mode="$(stat -c '%a' "$file")"
  test $((0$mode & 022)) -eq 0
  parent="$(dirname "$file")"
  while :; do
    test -d "$parent" && test ! -L "$parent"
    test "$(stat -c '%u' "$parent")" -eq 0
    mode="$(stat -c '%a' "$parent")"
    test $((0$mode & 022)) -eq 0
    test "$parent" = / && break
    next="$(dirname "$parent")"
    test "$next" != "$parent"
    parent="$next"
  done
}
test ! -e "$release_dir"
for helper in \
  /usr/local/sbin/verify-henukit-local-release.sh \
  /usr/local/sbin/henukit-release-images.sh; do
  trusted_root_file "$helper"
  test -x "$helper"
done
trusted_root_file /etc/henukit/release-signers
/usr/local/sbin/verify-henukit-local-release.sh \
  --artifact-dir "$stage" \
  --sha "$release_sha" \
  --inventory /usr/local/sbin/henukit-release-images.sh \
  --allowed-signers /etc/henukit/release-signers >/dev/null
mv -- "$stage" "$release_dir"
REMOTE_VERIFY
  remote_stage=""
fi

remote_platform_migrations="${platform_migrations:--}"
ssh "${ssh_options[@]}" "$production_alias" sh -s -- \
  "$release_sha" "$remote_release_dir" "$remote_env_file" \
  "$account_operator_role" "$remote_platform_migrations" \
  "${recovery_baseline_sha:--}" "${adopt_degraded_baseline_owner:--}" <<'REMOTE_ACTIVATE'
set -eu
release_sha="$1"
release_dir="$2"
remote_env_file="$3"
account_operator_role="$4"
case "$5" in
  -) platform_migrations="" ;;
  *) platform_migrations="$5" ;;
esac
case "${6-}" in
  -) recovery_baseline_sha="" ;;
  *) recovery_baseline_sha="${6-}" ;;
esac
case "${7-}" in
  -) adopt_degraded_baseline_owner="" ;;
  *) adopt_degraded_baseline_owner="${7-}" ;;
esac
trusted_root_file() {
  file="$1"
  case "$file" in /*) ;; *) return 1 ;; esac
  test -f "$file" && test -r "$file" && test ! -L "$file"
  test "$(stat -c '%u' "$file")" -eq 0
  mode="$(stat -c '%a' "$file")"
  test $((0$mode & 022)) -eq 0
  parent="$(dirname "$file")"
  while :; do
    test -d "$parent" && test ! -L "$parent"
    test "$(stat -c '%u' "$parent")" -eq 0
    mode="$(stat -c '%a' "$parent")"
    test $((0$mode & 022)) -eq 0
    test "$parent" = / && break
    next="$(dirname "$parent")"
    test "$next" != "$parent"
    parent="$next"
  done
}
trusted_root_directory() {
  directory="$1"
  case "$directory" in /*) ;; *) return 1 ;; esac
  while :; do
    test -d "$directory" && test ! -L "$directory"
    test "$(stat -c '%u' "$directory")" -eq 0
    mode="$(stat -c '%a' "$directory")"
    test $((0$mode & 022)) -eq 0
    test "$directory" = / && break
    next="$(dirname "$directory")"
    test "$next" != "$directory"
    directory="$next"
  done
}
for helper in \
  /usr/local/sbin/watch-henukit-actions \
  /usr/local/sbin/activate-henukit-release \
  /usr/local/sbin/verify-henukit-local-release.sh \
  /usr/local/sbin/henukit-release-images.sh; do
  trusted_root_file "$helper"
  test -x "$helper"
done
trusted_root_file /etc/henukit/release-signers
trusted_root_file /etc/henukit/github-actions-read.token
trusted_root_file "$remote_env_file"
watcher_state_root=/var/lib/henukit-actions-watch
trusted_root_directory "$watcher_state_root"
exec 8>"$watcher_state_root/direct-deploy.lock"
flock -n 8 || exit 76
watcher_service=henukit-actions-watch.service
watcher_was_active=0
quiesce_owned=0
quiesce_file="$watcher_state_root/quiesce.request"
quiesced_file="$watcher_state_root/quiesced"
restore_watcher() {
  result=$?
  trap - EXIT
  if test "$quiesce_owned" -eq 1; then
    rm -f -- "$quiesce_file" "$quiesced_file"
  fi
  if test "$watcher_was_active" -eq 1; then
    systemctl start "$watcher_service" >/dev/null 2>&1 || exit 74
    systemctl is-active --quiet "$watcher_service" || exit 74
  fi
  exit "$result"
}
trap restore_watcher EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
if systemctl is-active --quiet "$watcher_service"; then
  watcher_was_active=1
  quiesce_owned=1
  watcher_instance_file="$watcher_state_root/watcher.instance"
  trusted_root_file "$watcher_instance_file"
  watcher_instance="$(tr -d '\r\n' < "$watcher_instance_file")"
  case "$watcher_instance" in ''|*[!0-9]*) exit 75 ;; esac
  main_pid="$(systemctl show --property MainPID --value "$watcher_service")"
  test "$main_pid" = "$watcher_instance" || exit 75
  transport_nonce="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
  case "$transport_nonce" in ''|*[!0-9a-f]*) exit 75 ;; esac
  test "${#transport_nonce}" -eq 32 || exit 75
  request_tmp="$watcher_state_root/.quiesce.request.$$"
  rm -f -- "$quiesced_file"
  umask 077
  printf '%s %s %s\n' "$release_sha" "$watcher_instance" "$transport_nonce" > "$request_tmp"
  chmod 0600 "$request_tmp"
  mv "$request_tmp" "$quiesce_file"
elif test -e "$quiesce_file"; then
  trusted_root_file "$quiesce_file"
  read -r quiesce_sha watcher_instance transport_nonce quiesce_extra < "$quiesce_file"
  test "$quiesce_sha" = "$release_sha"
  case "$watcher_instance" in ''|*[!0-9]*) exit 75 ;; esac
  case "$transport_nonce" in ''|*[!0-9a-f]*) exit 75 ;; esac
  test "${#transport_nonce}" -eq 32 || exit 75
  test -z "${quiesce_extra:-}"
  watcher_was_active=1
  quiesce_owned=1
fi
if test "$watcher_was_active" -eq 1; then
  attempts=0
  while systemctl is-active --quiet "$watcher_service"; do
    current_pid="$(systemctl show --property MainPID --value "$watcher_service")"
    test "$current_pid" = "$watcher_instance" || exit 77
    attempts=$((attempts + 1))
    test "$attempts" -le 600 || exit 75
    sleep 1
  done
  trusted_root_file "$quiesced_file"
  test "$(tr -d '\r\n' < "$quiesced_file")" = "$release_sha $watcher_instance $transport_nonce" || exit 75
fi
if test -n "$adopt_degraded_baseline_owner"; then
  trusted_root_file /usr/local/sbin/adopt-henukit-degraded-baseline
  test -x /usr/local/sbin/adopt-henukit-degraded-baseline
  /usr/local/sbin/adopt-henukit-degraded-baseline \
    --sha "$recovery_baseline_sha" \
    --candidate-sha "$release_sha" \
    --expected-owner-uid "$adopt_degraded_baseline_owner" \
    --execute
fi
export GH_TOKEN_FILE=/etc/henukit/github-actions-read.token
export HENUKIT_ENV_FILE="$remote_env_file"
export HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE="$account_operator_role"
export HENUKIT_PLATFORM_MIGRATIONS="$platform_migrations"
if test -n "$recovery_baseline_sha"; then
  /usr/local/sbin/activate-henukit-release \
    "$release_sha" --local-artifacts "$release_dir" \
    --recover-degraded-baseline "$recovery_baseline_sha" --execute
else
  /usr/local/sbin/activate-henukit-release \
    "$release_sha" --local-artifacts "$release_dir" --execute
fi
REMOTE_ACTIVATE

printf '%s: release %s activated through %s\n' "$program" "$release_sha" "$production_alias"
