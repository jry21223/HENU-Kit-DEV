#!/usr/bin/env bash
# Opt-in scheduler. All production gates remain owned by the existing activator.
set -Eeuo pipefail
umask 077

program=auto-deploy-henukit-actions
log() { printf '%s: %s\n' "$program" "$*"; }
die() { log "$*" >&2; exit 1; }
[[ $# == 1 && ( "$1" == --once || "$1" == --watch ) ]] ||
  die 'usage: auto-deploy-henukit-actions.sh --once|--watch'
mode="$1"
wakeup=0
stopping=0
if [[ "$mode" == --watch ]]; then
  trap 'wakeup=1' USR1
  # Register before startup checks: Type=simple permits immediate notifications.
  trap 'stopping=1' TERM INT
fi
[[ "$(id -u)" == 0 ]] || die 'the scheduler must run as root'

repo=jry21223/HENU-Kit-DEV
workflow=deploy-henukit.yml
branch=main
export HENUKIT_REPO="$repo" HENUKIT_WORKFLOW="$workflow" HENUKIT_BRANCH="$branch"
policy="${HENUKIT_AUTO_DEPLOY_POLICY:-}"
state="${HENUKIT_AUTO_DEPLOY_STATE_ROOT:-/var/lib/henukit-actions-auto-deploy}"
watcher_state="${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}"
activator="${HENUKIT_ACTIVATE_COMMAND:-/usr/local/sbin/activate-henukit-release}"
token="${GH_TOKEN_FILE:-/etc/henukit/github-actions-read.token}"
poll_seconds="${HENUKIT_POLL_SECONDS:-60}"
[[ -n "$policy" ]] || die 'an explicit root-owned HENUKIT_AUTO_DEPLOY_POLICY is required'
[[ "$poll_seconds" =~ ^[1-9][0-9]*$ ]] || die 'poll interval must be a positive integer'
for command in gh jq flock sha256sum systemctl sync; do
  command -v "$command" >/dev/null || die "$command is required"
done

file_mode() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }
file_owner() { stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"; }
trusted() {
  local path="$1" permissions
  [[ "$path" == /* && "$path" != *'/../'* && "$path" != *'/./'* ]] || die 'trusted paths must be absolute and canonical'
  while :; do
    [[ -e "$path" && ! -L "$path" && ( -f "$path" || -d "$path" ) ]] || die "untrusted path: $path"
    [[ "$(file_owner "$path")" == 0 ]] || die "path must be root-owned: $path"
    permissions="$(file_mode "$path")"
    [[ "$permissions" =~ ^[0-7]{3,4}$ ]] || die "invalid permissions: $path"
    (( (8#$permissions & 8#022) == 0 )) || die "path must not be group/world writable: $path"
    [[ "$path" != / ]] || break
    path="$(dirname "$path")"
  done
}
private_file() {
  [[ -f "$1" && ! -L "$1" ]] || die "private file must be regular: $1"
  trusted "$1"
  [[ "$(file_mode "$1")" == 600 || "$(file_mode "$1")" == 400 ]] || die "private file must use mode 0600 or 0400: $1"
}
load_policy() {
  private_file "$policy"
  jq -e --arg repo "$repo" --arg workflow "$workflow" '
    type == "object" and keys == ["branch","format","repository","workflow"] and
    .format == "henukit-actions-auto-deploy-v1" and .repository == $repo and
    .workflow == $workflow and .branch == "main"
  ' "$policy" >/dev/null || die 'auto-deploy policy does not match the fixed main release contract'
  policy_digest="$(sha256sum "$policy" | awk '{print $1}')"
}

load_policy
trusted "$activator"
[[ -f "$activator" && -x "$activator" ]] || die 'release activator must be an executable regular file'
[[ -z "${HENUKIT_RETAINED_RELEASE_OWNER_UID:-}" ]] || die 'historical ownership adoption requires an explicit operator release'
private_file "$token"
GH_TOKEN="$(tr -d '\r\n' < "$token")"
[[ -n "$GH_TOKEN" ]] || die 'GitHub token is empty'
export GH_TOKEN
for directory in "$state" "$watcher_state"; do
  [[ "$directory" == /* && ! -L "$directory" ]] || die 'state paths must be absolute non-symlink directories'
  trusted "$(dirname "$directory")"
  if [[ -e "$directory" ]]; then
    [[ -d "$directory" ]] || die 'state path must be a directory'
    trusted "$directory"
    [[ "$(file_mode "$directory")" == 700 ]] || die 'state directories must use mode 0700'
  else
    install -d -m 0700 "$directory"
  fi
  trusted "$directory"
done
[[ ! -L "$state/controller.lock" ]] || die 'controller lock must not be a symlink'
[[ ! -e "$state/controller.lock" ]] || private_file "$state/controller.lock"
exec 9>"$state/controller.lock"
flock -n 9 || die 'another automatic release controller is running'

latest_run() {
  gh api "repos/$repo/actions/workflows/$workflow/runs?branch=$branch&event=push&per_page=1" \
    --jq '.workflow_runs[0] // empty | [.id,.run_attempt,.head_sha,.status,(.conclusion // "pending"),.path,.event,.head_branch] | @tsv'
}
branch_head() { gh api "repos/$repo/git/ref/heads/$branch" --jq '.object.sha'; }
write_audit() {
  local destination="$1" outcome="$2" result="${3:-0}" incoming
  [[ ! -e "$destination" && ! -L "$destination" ]] || die 'automatic attempt audit already exists'
  incoming="$(mktemp "$state/.audit.XXXXXX")" || die 'cannot create automatic attempt audit'
  jq -n --arg sha "$sha" --arg run "$run_id" --arg attempt "$run_attempt" \
    --arg policy "$policy_digest" --arg helper "$activator_digest" --arg outcome "$outcome" \
    --arg time "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson result "$result" \
    '{format:"henukit-actions-auto-attempt-v1",evidence_type:"scheduling_intent",sha:$sha,selected_run_id:$run,selected_run_attempt:$attempt,
      policy_sha256:$policy,activator_sha256:$helper,outcome:$outcome,recorded_at:$time,exit_code:$result}' \
    > "$incoming" || die 'cannot encode automatic attempt audit'
  chmod 0400 "$incoming"
  # Link publication refuses overwrite, including another process's audit.
  ln "$incoming" "$destination" || die 'cannot publish automatic attempt audit'
  rm -f "$incoming"
  sync -f "$state" || die 'cannot durably record automatic attempt audit'
}

activate_locked() (
  [[ ! -L "$watcher_state/direct-deploy.lock" ]] || die 'direct deployment lock is a symlink'
  [[ ! -e "$watcher_state/direct-deploy.lock" ]] || trusted "$watcher_state/direct-deploy.lock"
  exec 8>"$watcher_state/direct-deploy.lock"
  flock -n 8 || { log 'another direct deployment holds the lock; waiting'; return 0; }
  [[ "$(latest_run)" == "$row" && "$(branch_head)" == "$sha" ]] || {
    log 'release identity changed before authorization; waiting for current main'; return 0;
  }
  load_policy
  trusted "$activator"
  activator_digest="$(sha256sum "$activator" | awk '{print $1}')"
  write_audit "$state/$sha.attempted.json" attempted
  local status=0
  HENUKIT_EXPECTED_ACTIONS_IDENTITY="actions:$run_id:$run_attempt" "$activator" "$sha" --execute || status=$?
  if [[ "$status" == 0 ]]; then
    private_file "$watcher_state/last-activated-sha"
    [[ "$(tr -d '\r\n' < "$watcher_state/last-activated-sha")" == "$sha" ]] || status=1
  fi
  if [[ "$status" != 0 ]]; then
    write_audit "$state/$sha.failed.json" failed "$status"
    log "release $sha failed; automatic retries for this SHA are disabled"
    return "$status"
  fi
  write_audit "$state/$sha.activated.json" activated
  log "release $sha activated through the existing full release gates"
)

check_once() {
  local row run_id run_attempt sha status conclusion path event ref old_state
  load_policy
  if [[ -e "$watcher_state/scoped-food-release" || -L "$watcher_state/scoped-food-release" ]]; then
    log 'scoped release requires reviewed reconciliation before full automatic activation'; return 0
  fi
  for pending in "$watcher_state/rollback-contracts/pending/"*; do
    if [[ -e "$pending" || -L "$pending" ]]; then
      log 'pending rollback requires operator reconciliation before automatic scheduling'; return 0
    fi
  done
  old_state="$(systemctl show henukit-actions-watch.service -p LoadState --value)" || die 'cannot inspect the legacy watcher'
  if [[ "$old_state" != not-found ]]; then
    [[ "$old_state" == loaded ]] || die 'legacy watcher load state is ambiguous'
    old_state="$(systemctl show henukit-actions-watch.service -p ActiveState --value)" || die 'cannot inspect legacy watcher activity'
    [[ "$old_state" == inactive ]] || { log 'legacy watcher must drain and stop before automatic scheduling'; return 0; }
  fi
  row="$(latest_run)" || die 'GitHub workflow lookup failed'
  [[ -n "$row" ]] || { log 'no main build is available'; return 0; }
  IFS=$'\t' read -r run_id run_attempt sha status conclusion path event ref <<<"$row"
  [[ "$run_id" =~ ^[1-9][0-9]*$ && "$run_attempt" =~ ^[1-9][0-9]*$ && "$sha" =~ ^[0-9a-f]{40}$ ]] || die 'GitHub returned an invalid release identity'
  [[ "$path" == ".github/workflows/$workflow" && "$event" == push && "$ref" == main ]] || die 'GitHub returned an unexpected release source'
  [[ "$status" == completed && "$conclusion" == success ]] || { log 'latest main build is not successful; waiting'; return 0; }
  [[ "$(branch_head)" == "$sha" ]] || { log 'successful build is stale; waiting for current main'; return 0; }
  if [[ -e "$watcher_state/last-activated-sha" || -L "$watcher_state/last-activated-sha" ]]; then
    private_file "$watcher_state/last-activated-sha"
    if [[ "$(tr -d '\r\n' < "$watcher_state/last-activated-sha")" == "$sha" ]]; then
      log "full release $sha is already recorded; no new activation"; return 0
    fi
  fi
  if [[ -e "$state/$sha.attempted.json" || -L "$state/$sha.attempted.json" ]]; then
    private_file "$state/$sha.attempted.json"
    log "release $sha already has an automatic attempt; no automatic redeployment"; return 0
  fi
  activate_locked
}

if [[ "$mode" == --once ]]; then check_once; exit $?; fi
while [[ "$stopping" == 0 ]]; do
  check_once || log 'automatic activation stopped; retaining its attempt audit'
  [[ "$stopping" == 0 ]] || break
  if [[ "$wakeup" == 1 ]]; then wakeup=0; continue; fi
  sleep "$poll_seconds" &
  sleeper=$!
  wait "$sleeper" || true
  kill "$sleeper" 2>/dev/null || true
  wait "$sleeper" 2>/dev/null || true
  wakeup=0
done
