#!/usr/bin/env bash
# Explicit one-command activation for one reviewed, fixed-SHA HENU Kit release.
# The watcher remains the owner of artifact verification, backup/restore,
# Account mock gates, activation, smoke checks, and rollback.
set -Eeuo pipefail

program="activate-henukit-release"

usage() {
  cat >&2 <<'EOF'
usage: activate-henukit-release.sh <full-main-sha> --execute
       activate-henukit-release.sh <full-main-sha> --local-artifacts <artifact-dir> --execute
       activate-henukit-release.sh <full-main-sha> --local-artifacts <artifact-dir> \
         --recover-degraded-baseline <full-current-sha> --execute

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

release_source="actions"
local_artifact_dir=""
recovery_baseline_sha=""
if [[ $# -eq 2 && "$2" == "--execute" ]]; then
  release_sha="$1"
elif [[ $# -eq 4 && "$2" == "--local-artifacts" && "$4" == "--execute" ]]; then
  release_sha="$1"
  release_source="local"
  local_artifact_dir="$3"
elif [[ $# -eq 6 && "$2" == "--local-artifacts" &&
        "$4" == "--recover-degraded-baseline" && "$6" == "--execute" ]]; then
  release_sha="$1"
  release_source="local"
  local_artifact_dir="$3"
  recovery_baseline_sha="$5"
else
  usage
  exit 64
fi

repo="${HENUKIT_REPO:-jry21223/HENU-Kit-DEV}"
blocker_issue="${HENUKIT_BLOCKER_ISSUE:-166}"
branch="${HENUKIT_BRANCH:-main}"
workflow="${HENUKIT_WORKFLOW:-deploy-henukit.yml}"
state_root="${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}"
watcher="${HENUKIT_WATCHER:-/usr/local/sbin/watch-henukit-actions}"
env_file="${HENUKIT_ENV_FILE:-}"
token_file="${GH_TOKEN_FILE:-/etc/henukit/github-actions-read.token}"
release_root="${HENUKIT_RELEASE_ROOT:-/opt/henukit-releases}"
trust_anchor="${HENUKIT_TRUST_ANCHOR:-/}"
epay_ssh_target="${HENUKIT_EPAY_GATEWAY_SSH_TARGET:-root@metaview.top}"
epay_gateway_dir="${HENUKIT_EPAY_GATEWAY_DIR:-/root/epay-gateway}"

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release SHA must be 40 lowercase hexadecimal characters"
if [[ "$release_source" == "local" ]]; then
  [[ -d "$local_artifact_dir" && ! -L "$local_artifact_dir" ]] ||
    die "--local-artifacts must name a non-symlink artifact directory"
  if [[ -n "$recovery_baseline_sha" ]]; then
    [[ "$recovery_baseline_sha" =~ ^[0-9a-f]{40}$ ]] ||
      die "--recover-degraded-baseline must be a full lowercase Git SHA"
    [[ "$recovery_baseline_sha" != "$release_sha" ]] ||
      die "recovery baseline and candidate SHA must differ"
  fi
fi
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

trusted_private_directory() {
  local path="$1" parent mode owner
  [[ "$path" == /* && "$trust_anchor" == /* ]] || return 1
  [[ -d "$path" && ! -L "$path" ]] || return 1
  mode="$(stat -c '%a' "$path" 2>/dev/null || stat -f '%Lp' "$path")"
  owner="$(stat -c '%u' "$path" 2>/dev/null || stat -f '%u' "$path")"
  [[ "$owner" == "$(id -u)" ]] || return 1
  (( (8#$mode & 8#077) == 0 )) || return 1
  [[ "$path" == "$trust_anchor" ]] && return 0
  parent="$(dirname "$path")"
  while :; do
    [[ -d "$parent" && ! -L "$parent" ]] || return 1
    mode="$(stat -c '%a' "$parent" 2>/dev/null || stat -f '%Lp' "$parent")"
    owner="$(stat -c '%u' "$parent" 2>/dev/null || stat -f '%u' "$parent")"
    [[ "$owner" == "$(id -u)" ]] || return 1
    (( (8#$mode & 8#022) == 0 )) || return 1
    [[ "$parent" == "$trust_anchor" ]] && return 0
    [[ "$parent" != "/" ]] || return 1
    parent="$(dirname "$parent")"
  done
}

trusted_nonwritable_directory_chain() {
  local parent="$1" mode owner
  while :; do
    [[ -d "$parent" && ! -L "$parent" ]] || return 1
    mode="$(stat -c '%a' "$parent" 2>/dev/null || stat -f '%Lp' "$parent")"
    owner="$(stat -c '%u' "$parent" 2>/dev/null || stat -f '%u' "$parent")"
    [[ "$owner" == "$(id -u)" ]] || return 1
    (( (8#$mode & 8#022) == 0 )) || return 1
    [[ "$parent" == "$trust_anchor" ]] && return 0
    [[ "$parent" != "/" ]] || return 1
    parent="$(dirname "$parent")"
  done
}

for directory in "$state_root" "$state_root/approvals" "$state_root/prepared"; do
  if [[ ! -e "$directory" ]]; then
    trusted_nonwritable_directory_chain "$(dirname "$directory")" ||
      die "cannot create release state below an untrusted parent chain: $directory"
    install -d -m 0700 "$directory"
  fi
  trusted_private_directory "$directory" ||
    die "release state directory is not owned by the release user with a trusted private parent chain: $directory"
done
approval="$state_root/approvals/$release_sha"
prepared="$state_root/prepared/$release_sha"
recovery_binding="$state_root/prepared/${release_sha}.recovery-baseline"
active="$state_root/last-activated-sha"
resume_existing_approval=0

validate_private_file() {
  local path="$1" label="$2" mode owner
  [[ -f "$path" && -r "$path" && ! -L "$path" ]] || die "$label is missing or untrusted"
  mode="$(stat -c '%a' "$path" 2>/dev/null || stat -f '%Lp' "$path")"
  owner="$(stat -c '%u' "$path" 2>/dev/null || stat -f '%u' "$path")"
  [[ "$mode" == "600" || "$mode" == "400" ]] || die "$label must use mode 0600 or 0400"
  [[ "$owner" == "$(id -u)" ]] || die "$label must be owned by the release user"
}

validate_prepared_evidence() {
  local prepared_backup prepared_metadata
  validate_private_file "$prepared" "prepared backup evidence"
  [[ -s "$prepared" ]] || die "prepared backup evidence is empty"
  prepared_backup="$(tr -d '\r\n' < "$prepared")"
  [[ "$prepared_backup" =~ ^/[A-Za-z0-9_./-]+$ && "$prepared_backup" != "/" ]] ||
    die "prepared backup evidence contains an invalid backup path"
  prepared_metadata="${prepared_backup}.meta"
  validate_private_file "$prepared_backup" "prepared backup"
  validate_private_file "$prepared_metadata" "prepared backup metadata"
  [[ "$(grep -c '^release_sha=' "$prepared_metadata")" == "1" ]] ||
    die "prepared backup metadata must contain exactly one release SHA"
  [[ "$(sed -n 's/^release_sha=//p' "$prepared_metadata")" == "$release_sha" ]] ||
    die "prepared backup evidence is not bound to release $release_sha"
}

validate_existing_approval() {
  validate_private_file "$approval" "existing approval"
  [[ "$(tr -d '\r\n' < "$approval")" == "$release_sha" ]] ||
    die "existing approval is not bound to release $release_sha"
}

validate_recovery_binding() {
  validate_private_file "$recovery_binding" "recovery approval binding"
  [[ "$(tr -d '\r\n' < "$recovery_binding")" == "$recovery_baseline_sha" ]] ||
    die "existing approval is not bound to recovery baseline $recovery_baseline_sha"
}

blocker_state="$(gh api "repos/$repo/issues/$blocker_issue" --jq '.state')"
[[ "$blocker_state" == "closed" ]] || die "blocker issue #$blocker_issue must be closed before Account Portfolio deployment"

if [[ -e "$approval" ]]; then
  [[ "$release_source" == "local" && -n "$recovery_baseline_sha" ]] ||
    die "an approval already exists for release $release_sha"
  validate_existing_approval
  validate_prepared_evidence
  validate_recovery_binding
  resume_existing_approval=1
  printf '%s: resuming valid unconsumed approval for release %s\n' "$program" "$release_sha"
fi

verify_release_current() {
  local branch_head run_row run_sha run_status run_conclusion
  branch_head="$(gh api "repos/$repo/branches/$branch" --jq '.commit.sha')"
  [[ "$branch_head" == "$release_sha" ]] || die "requested release is not the current $branch head"
  if [[ "$release_source" == "local" ]]; then
    return
  fi
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

watcher_args=(--once)
if [[ "$release_source" == "local" ]]; then
  watcher_args=(--local-artifacts "$local_artifact_dir" --sha "$release_sha")
  if [[ -n "$recovery_baseline_sha" ]]; then
    watcher_args+=(--recover-degraded-baseline "$recovery_baseline_sha")
  fi
fi

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

approval_incoming=""
recovery_binding_incoming=""
remote_stage=""
environment_backup="$(dirname "$env_file")/.henukit-env.backup.$$"
cp -p "$env_file" "$environment_backup"
chmod 0600 "$environment_backup"
export HENUKIT_ROLLBACK_ENV_FILE="$environment_backup"
cleanup() {
  if [[ -n "${approval_incoming:-}" && -f "$approval_incoming" ]]; then
    rm -f -- "$approval_incoming"
  fi
  if [[ -n "${recovery_binding_incoming:-}" && -f "$recovery_binding_incoming" ]]; then
    rm -f -- "$recovery_binding_incoming"
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

# With no approval, the first pass can only verify artifacts and restore-test
# backups. An exact local recovery may resume an approval left unconsumed by a
# fail-closed pre-load rejection, but only after independently validating the
# approval and its SHA-bound prepared evidence above.
if [[ "$resume_existing_approval" -eq 0 ]]; then
  "$watcher" "${watcher_args[@]}"
  [[ -s "$prepared" ]] || die "watcher did not prepare verified backup evidence for release $release_sha"
  validate_prepared_evidence
fi

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

if [[ "$resume_existing_approval" -eq 0 ]]; then
  umask 077
  if [[ -n "$recovery_baseline_sha" ]]; then
    recovery_binding_incoming="$(mktemp "$state_root/prepared/.${release_sha}.recovery-baseline.XXXXXX")"
    printf '%s\n' "$recovery_baseline_sha" > "$recovery_binding_incoming"
    chmod 0600 "$recovery_binding_incoming"
    mv "$recovery_binding_incoming" "$recovery_binding"
    recovery_binding_incoming=""
  elif [[ -e "$recovery_binding" ]]; then
    die "routine activation found a stale recovery approval binding"
  fi
  approval_incoming="$(mktemp "$state_root/approvals/.${release_sha}.XXXXXX")"
  printf '%s\n' "$release_sha" > "$approval_incoming"
  chmod 0600 "$approval_incoming"
  mv "$approval_incoming" "$approval"
  approval_incoming=""
fi

# The approval is single-use. The watcher consumes it before loading an image,
# refreshes both verified backups, activates, verifies, and rolls back on error.
"$watcher" "${watcher_args[@]}"
[[ -s "$active" && "$(tr -d '\r\n' < "$active")" == "$release_sha" ]] ||
  die "watcher returned without activating release $release_sha"
rm -f -- "$environment_backup"
environment_backup=""

printf '%s: release %s activated\n' "$program" "$release_sha"
