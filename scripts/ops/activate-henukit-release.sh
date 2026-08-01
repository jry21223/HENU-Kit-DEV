#!/usr/bin/env bash
# Explicit one-command activation for one reviewed, fixed-SHA HENU Kit release.
# The watcher remains the owner of artifact verification, backup/restore,
# Account mock gates, activation, smoke checks, and rollback.
set -Eeuo pipefail

program="activate-henukit-release"

usage() {
  cat >&2 <<'EOF'
usage: activate-henukit-release.sh <full-main-sha> --execute

The command is the production approval action. It first asks the watcher to
prepare and restore-test backups without approval, installs the tested EasyPay
gateway patch set on MetaView, then atomically approves that exact SHA and asks
the watcher to activate HENU Kit. It never approves while the QuizCraft
cutover blocker is open or when preparation, mock, or gateway gates fail.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

if [[ $# -ne 2 || "$2" != "--execute" ]]; then
  usage
  exit 64
fi

release_sha="$1"
repo="${HENUKIT_REPO:-jry21223/HENU-Kit-DEV}"
blocker_issue="${HENUKIT_BLOCKER_ISSUE:-166}"
branch="${HENUKIT_BRANCH:-main}"
workflow="${HENUKIT_WORKFLOW:-deploy-henukit.yml}"
state_root="${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}"
watcher="${HENUKIT_WATCHER:-/usr/local/sbin/watch-henukit-actions}"
env_file="${HENUKIT_ENV_FILE:-}"
token_file="${GH_TOKEN_FILE:-/etc/henukit/github-actions-read.token}"
release_root="${HENUKIT_RELEASE_ROOT:-/opt/henukit-releases}"
epay_ssh_target="${HENUKIT_EPAY_GATEWAY_SSH_TARGET:-root@metaview.top}"
epay_gateway_dir="${HENUKIT_EPAY_GATEWAY_DIR:-/root/epay-gateway}"

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release SHA must be 40 lowercase hexadecimal characters"
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_REPO must be an owner/name pair"
[[ "$blocker_issue" =~ ^[1-9][0-9]*$ ]] || die "HENUKIT_BLOCKER_ISSUE must be a positive issue number"
[[ "$branch" =~ ^[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_BRANCH contains unsupported characters"
[[ "$epay_ssh_target" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_EPAY_GATEWAY_SSH_TARGET contains unsupported characters"
[[ "$epay_gateway_dir" =~ ^/[A-Za-z0-9_./-]+$ && "$epay_gateway_dir" != "/" ]] ||
  die "HENUKIT_EPAY_GATEWAY_DIR must be a specific absolute path"
[[ -x "$watcher" ]] || die "watcher is not executable: $watcher"
command -v gh >/dev/null 2>&1 || die "gh CLI is required"
command -v ssh >/dev/null 2>&1 || die "ssh is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
[[ -r "$token_file" && -f "$token_file" ]] || die "GH_TOKEN_FILE must point to a readable regular file"
token_mode="$(stat -c '%a' "$token_file" 2>/dev/null || stat -f '%Lp' "$token_file")"
token_owner="$(stat -c '%u' "$token_file" 2>/dev/null || stat -f '%u' "$token_file")"
[[ "$token_mode" == "600" || "$token_mode" == "400" ]] || die "GitHub token file mode must be 0600 or 0400"
[[ "$token_owner" == "$(id -u)" ]] || die "GitHub token file must be owned by the release user"
GH_TOKEN="$(tr -d '\r\n' < "$token_file")"
[[ -n "$GH_TOKEN" ]] || die "GitHub token file is empty"
export GH_TOKEN

install -d -m 0700 "$state_root" "$state_root/approvals" "$state_root/prepared"
approval="$state_root/approvals/$release_sha"
prepared="$state_root/prepared/$release_sha"
active="$state_root/last-activated-sha"

blocker_state="$(gh api "repos/$repo/issues/$blocker_issue" --jq '.state')"
[[ "$blocker_state" == "closed" ]] || die "blocker issue #$blocker_issue must be closed before Account Portfolio deployment"

[[ ! -e "$approval" ]] || die "an approval already exists for release $release_sha"

verify_release_current() {
  local branch_head run_row run_sha run_status run_conclusion
  branch_head="$(gh api "repos/$repo/branches/$branch" --jq '.commit.sha')"
  [[ "$branch_head" == "$release_sha" ]] || die "requested release is not the current $branch head"
  run_row="$(gh run list --repo "$repo" --workflow "$workflow" --branch "$branch" --event push --limit 1 --json headSha,status,conclusion --jq 'first(.[]) | [.headSha,.status,.conclusion] | @tsv')"
  IFS=$'\t' read -r run_sha run_status run_conclusion <<< "$run_row"
  [[ "$run_sha" == "$release_sha" && "$run_status" == "completed" && "$run_conclusion" == "success" ]] ||
    die "requested release is not the newest completed successful $workflow run"
}

verify_release_current
if [[ -s "$active" && "$(tr -d '\r\n' < "$active")" == "$release_sha" ]]; then
  printf '%s: release %s is already active\n' "$program" "$release_sha"
  exit 0
fi

[[ -n "$env_file" && -r "$env_file" && -w "$env_file" && ! -L "$env_file" ]] ||
  die "HENUKIT_ENV_FILE must be a writable, non-symlink production environment file"

tenant_credentials="$(ssh "$epay_ssh_target" bash -s -- "$epay_gateway_dir" <<'REMOTE'
set -Eeuo pipefail
env_file="$1/.env"
value() {
  local key="$1" line
  line="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$env_file" | tail -n 1 || true)"
  printf '%s' "${line#*=}"
}
printf '%s\n%s\n' "$(value HENUKIT_EPAY_PID)" "$(value HENUKIT_EPAY_KEY)"
REMOTE
)"
[[ "$tenant_credentials" == *$'\n'* ]] || die "MetaView did not return the HENU tenant credential pair"
tenant_pid="${tenant_credentials%%$'\n'*}"
tenant_key="${tenant_credentials#*$'\n'}"
[[ "$tenant_pid" =~ ^[A-Za-z0-9_-]{1,64}$ ]] || die "MetaView HENU tenant PID is missing or invalid"
[[ ${#tenant_key} -ge 16 && "$tenant_key" != *[[:space:]]* ]] || die "MetaView HENU tenant key is missing or invalid"

set_account_env_value() {
  local key="$1"
  local value="$2"
  local incoming="$(dirname "$env_file")/.henukit-env.$$"
  awk -v key="$key" -v value="$value" '
    BEGIN { changed=0 }
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" { if (changed > 0) exit 42; print key "=" value; changed++; next }
    { print }
    END { if (changed == 0) print key "=" value }
  ' "$env_file" > "$incoming" || {
    rm -f -- "$incoming"
    return 1
  }
  chmod 0600 "$incoming"
  mv "$incoming" "$env_file"
}

approval_incoming="$state_root/approvals/.${release_sha}.$$"
remote_stage=""
environment_backup="$(dirname "$env_file")/.henukit-env.backup.$$"
cp -p "$env_file" "$environment_backup"
chmod 0600 "$environment_backup"
export HENUKIT_ROLLBACK_ENV_FILE="$environment_backup"
cleanup() {
  if [[ -n "${approval_incoming:-}" && -f "$approval_incoming" ]]; then
    rm -f -- "$approval_incoming"
  fi
  if [[ "$remote_stage" =~ ^/tmp/henukit-epay-release\.[A-Za-z0-9]+$ ]]; then
    ssh "$epay_ssh_target" "rm -rf -- '$remote_stage'" >/dev/null 2>&1 || true
  fi
  if [[ -n "${environment_backup:-}" && -f "$environment_backup" ]]; then
    mv "$environment_backup" "$env_file" || true
  fi
}
trap cleanup EXIT
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_PID "$tenant_pid" || die "could not install the HENU tenant PID"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_KEY "$tenant_key" || die "could not install the HENU tenant key"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL "https://metaview.top/epay" || die "could not configure the EasyPay base URL"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL "https://henukit.cn/api/v1/payment-providers/easypay/notifications" || die "could not configure the EasyPay callback"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL "https://henukit.cn/account/membership" || die "could not configure the EasyPay return URL"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_ENABLED 1 || die "could not atomically enable Account Portfolio EasyPay"

# No approval exists during this first pass, so the watcher can only download,
# validate Account's mock-free manifest and production env, then backup and
# restore-test both databases. Any failure exits before an approval is written.
"$watcher" --once
[[ -s "$prepared" ]] || die "watcher did not prepare verified backup evidence for release $release_sha"
[[ "$(basename "$(tr -d '\r\n' < "$prepared")")" == *"$release_sha"* ]] ||
  die "prepared backup evidence is not bound to release $release_sha"

release_dir="$release_root/$release_sha"
epay_installer="$release_dir/bin/deploy-epay-gateway-patches.sh"
epay_patches="$release_dir/infra/epay-gateway/patches"
[[ -x "$epay_installer" ]] || die "release has no executable EasyPay gateway installer"
[[ -d "$epay_patches" ]] || die "release has no EasyPay gateway patch set"

verify_release_current
remote_stage="$(ssh "$epay_ssh_target" 'mktemp -d /tmp/henukit-epay-release.XXXXXX')"
[[ "$remote_stage" =~ ^/tmp/henukit-epay-release\.[A-Za-z0-9]+$ ]] ||
  die "EasyPay gateway host returned an invalid staging path"
tar -C "$release_dir" -czf - \
  bin/deploy-epay-gateway-patches.sh \
  infra/epay-gateway/patches | \
  ssh "$epay_ssh_target" "tar -xzf - -C '$remote_stage'"
ssh "$epay_ssh_target" \
  "$remote_stage/bin/deploy-epay-gateway-patches.sh" \
  "$epay_gateway_dir" \
  "$remote_stage/infra/epay-gateway/patches" \
  --execute
verify_release_current

umask 077
printf '%s\n' "$release_sha" > "$approval_incoming"
chmod 0600 "$approval_incoming"
mv "$approval_incoming" "$approval"
approval_incoming=""

# The approval is single-use. The watcher consumes it before loading an image,
# refreshes both verified backups, activates, verifies, and rolls back on error.
"$watcher" --once
[[ -s "$active" && "$(tr -d '\r\n' < "$active")" == "$release_sha" ]] ||
  die "watcher returned without activating release $release_sha"
rm -f -- "$environment_backup"
environment_backup=""

printf '%s: release %s activated\n' "$program" "$release_sha"
