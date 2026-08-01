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
prepare and restore-test backups without approval, then atomically approves
that exact SHA, installs the tested EasyPay gateway patch set on MetaView, and
asks the watcher to activate HENU Kit. It never approves while the QuizCraft
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
state_root="${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}"
watcher="${HENUKIT_WATCHER:-/usr/local/sbin/watch-henukit-actions}"
release_root="${HENUKIT_RELEASE_ROOT:-/opt/henukit-releases}"
epay_ssh_target="${HENUKIT_EPAY_GATEWAY_SSH_TARGET:-root@metaview.top}"
epay_gateway_dir="${HENUKIT_EPAY_GATEWAY_DIR:-/root/epay-gateway}"

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release SHA must be 40 lowercase hexadecimal characters"
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_REPO must be an owner/name pair"
[[ "$blocker_issue" =~ ^[1-9][0-9]*$ ]] || die "HENUKIT_BLOCKER_ISSUE must be a positive issue number"
[[ "$epay_ssh_target" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_EPAY_GATEWAY_SSH_TARGET contains unsupported characters"
[[ "$epay_gateway_dir" =~ ^/[A-Za-z0-9_./-]+$ && "$epay_gateway_dir" != "/" ]] ||
  die "HENUKIT_EPAY_GATEWAY_DIR must be a specific absolute path"
[[ -x "$watcher" ]] || die "watcher is not executable: $watcher"
command -v gh >/dev/null 2>&1 || die "gh CLI is required"
command -v ssh >/dev/null 2>&1 || die "ssh is required"
command -v tar >/dev/null 2>&1 || die "tar is required"

install -d -m 0700 "$state_root" "$state_root/approvals" "$state_root/prepared"
approval="$state_root/approvals/$release_sha"
prepared="$state_root/prepared/$release_sha"
active="$state_root/last-activated-sha"

blocker_state="$(gh api "repos/$repo/issues/$blocker_issue" --jq '.state')"
[[ "$blocker_state" == "closed" ]] || die "blocker issue #$blocker_issue must be closed before Account Portfolio deployment"

if [[ -s "$active" && "$(tr -d '\r\n' < "$active")" == "$release_sha" ]]; then
  printf '%s: release %s is already active\n' "$program" "$release_sha"
  exit 0
fi

[[ ! -e "$approval" ]] || die "an approval already exists for release $release_sha"

# No approval exists during this first pass, so the watcher can only download,
# validate Account's mock-free manifest and production env, then backup and
# restore-test both databases. Any failure exits before an approval is written.
"$watcher" --once
[[ -s "$prepared" ]] || die "watcher did not prepare verified backup evidence for release $release_sha"

release_dir="$release_root/$release_sha"
epay_installer="$release_dir/bin/deploy-epay-gateway-patches.sh"
epay_patches="$release_dir/infra/epay-gateway/patches"
[[ -x "$epay_installer" ]] || die "release has no executable EasyPay gateway installer"
[[ -d "$epay_patches" ]] || die "release has no EasyPay gateway patch set"

approval_incoming="$state_root/approvals/.${release_sha}.$$"
remote_stage=""
cleanup() {
  if [[ -n "${approval_incoming:-}" && -f "$approval_incoming" ]]; then
    rm -f -- "$approval_incoming"
  fi
  if [[ "$remote_stage" =~ ^/tmp/henukit-epay-release\.[A-Za-z0-9]+$ ]]; then
    ssh "$epay_ssh_target" "rm -rf -- '$remote_stage'" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

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

printf '%s: release %s activated\n' "$program" "$release_sha"
