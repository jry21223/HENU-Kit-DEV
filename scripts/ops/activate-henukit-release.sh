#!/usr/bin/env bash
# Explicit one-command activation for one reviewed, fixed-SHA HENU Kit release.
# The watcher remains the owner of artifact verification, backup/restore,
# Account mock gates, activation, smoke checks, and rollback.
set -Eeuo pipefail

program="activate-henukit-release"

usage() {
  cat >&2 <<'EOF'
usage: activate-henukit-release.sh <full-main-sha> [--preserve-existing] --execute
       activate-henukit-release.sh <full-main-sha> --notice-url <apply|reconcile|restore> --plan <private-plan> --execute
       activate-henukit-release.sh <full-main-sha> --notice-url accept --plan <private-plan> --evidence <private-evidence> --execute
       activate-henukit-release.sh <full-main-sha> \
         --recover-degraded-baseline <full-current-sha> --execute
       activate-henukit-release.sh <full-main-sha> --local-artifacts <artifact-dir> --execute
       activate-henukit-release.sh <full-main-sha> --local-artifacts <artifact-dir> \
         --recover-degraded-baseline <full-current-sha> --execute

The command is the production approval action. Default activation asks the watcher to
prepare and restore-test backups without approval, installs the tested EasyPay
gateway patch set on MetaView, then atomically approves that exact SHA and asks
the watcher to activate HENU Kit. It never approves while the QuizCraft
cutover blocker is open or when preparation, mock, or gateway gates fail.

--preserve-existing uses only valid existing configuration and grants, preserves
credential files in place and checks the installed gateway read-only. Missing
or changed authority fails closed; it never installs credentials or grants.

Set HENUKIT_RETAINED_RELEASE_OWNER_UID only when the exact healthy rollback
release predates the root-owned release boundary. The command then runs the
reviewed ownership adopter in preflight and execute modes before approval.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

# Explicit single-key configuration reconciliation is part of this original
# owner, not a replay of the full release/EasyPay transaction. Default activation
# and the scheduler's already-active no-op remain unchanged.
if [[ ( $# -eq 6 || $# -eq 8 ) && "$2" == "--notice-url" && "$4" == "--plan" ]]; then
  [[ "$1" =~ ^[0-9a-f]{40}$ && ( "$3" == "apply" || "$3" == "reconcile" ||
      "$3" == "restore" || "$3" == "accept" ) ]] || die "invalid Notice URL action or SHA"
  notice_evidence=""
  if [[ $# -eq 8 ]]; then
    [[ "$3" == "accept" && "$6" == "--evidence" && "$8" == "--execute" ]] || { usage; exit 64; }
    notice_evidence="$7"
  else
    [[ "$3" != "accept" && "$6" == "--execute" ]] || { usage; exit 64; }
  fi
  exec python3 -I - "${HENUKIT_WATCHER:-/usr/local/sbin/watch-henukit-actions}" \
    "${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}" "$1" "$3" "$5" "$notice_evidence" \
    "${HENUKIT_TRUST_ANCHOR:-/}" <<'NOTICE_URL_OWNER'
import fcntl,os,pathlib,stat,sys
watcher,state,sha,mode,plan,evidence,anchor=sys.argv[1:]
state,anchor=map(pathlib.Path,(state,anchor));lock=state/'direct-deploy.lock'
def trusted(path,directory=False):
    info=path.lstat()
    assert path.is_absolute() and '..' not in path.parts and info.st_uid==os.geteuid() and not info.st_mode&0o022
    assert stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode)
    if path==anchor:return info
    for parent in path.parents:
        if parent==anchor:break
        assert parent.is_relative_to(anchor)
        item=parent.lstat();assert stat.S_ISDIR(item.st_mode) and item.st_uid==os.geteuid() and not item.st_mode&0o022
    return info
try:
    trusted(anchor,True);trusted(state,True);trusted(pathlib.Path(watcher));info=trusted(lock)
    assert not stat.S_IMODE(state.lstat().st_mode)&0o077 and info.st_size==0
    try:held=os.fstat(8)
    except OSError:
        source=os.open(lock,os.O_RDONLY|os.O_NOFOLLOW)
        os.dup2(source,8,inheritable=True)
        if source!=8:os.close(source)
        held=os.fstat(8)
    assert (held.st_dev,held.st_ino)==(info.st_dev,info.st_ino)
    fcntl.flock(8,fcntl.LOCK_EX|fcntl.LOCK_NB)
    os.set_inheritable(8,True)
except (OSError,AssertionError):
    raise SystemExit('Notice URL original release owner/lock unavailable; no action started')
args=[watcher,'--notice-url',mode,'--sha',sha,'--plan',plan]
if evidence:args+=['--evidence',evidence]
os.execv(watcher,args)
NOTICE_URL_OWNER
fi

notice_url_plan=""
# Optional only on an explicitly requested full activation. Never inferred from
# the normal summary flags or silently installed as a new deployment default.
if [[ $# -ge 4 && "${@: -3:1}" == "--notice-url-plan" && "${@: -1}" == "--execute" ]]; then
  notice_url_plan="${@: -2:1}"
  set -- "${@:1:$#-3}" --execute
fi

preserve_existing=0
if [[ $# -ge 3 && "${@: -2:1}" == "--preserve-existing" && "${@: -1}" == "--execute" ]]; then
  preserve_existing=1
  set -- "${@:1:$#-2}" --execute
fi

release_source="actions"
local_artifact_dir=""
recovery_baseline_sha=""
if [[ $# -eq 2 && "$2" == "--execute" ]]; then
  release_sha="$1"
elif [[ $# -eq 4 && "$2" == "--recover-degraded-baseline" && "$4" == "--execute" ]]; then
  release_sha="$1"
  recovery_baseline_sha="$3"
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
[[ ! -e "$state_root/scoped-food-release" && ! -L "$state_root/scoped-food-release" ]] ||
  die "scoped Food release must be rolled back before full approval"
watcher="${HENUKIT_WATCHER:-/usr/local/sbin/watch-henukit-actions}"
env_file="${HENUKIT_ENV_FILE:-}"
token_file="${GH_TOKEN_FILE:-/etc/henukit/github-actions-read.token}"
release_root="${HENUKIT_RELEASE_ROOT:-/opt/henukit-releases}"
trust_anchor="${HENUKIT_TRUST_ANCHOR:-/}"
epay_ssh_target="${HENUKIT_EPAY_GATEWAY_SSH_TARGET:-root@metaview.top}"
epay_gateway_dir="${HENUKIT_EPAY_GATEWAY_DIR:-/root/epay-gateway}"
retained_release_owner_uid="${HENUKIT_RETAINED_RELEASE_OWNER_UID:-}"
retained_release_adopter="${HENUKIT_RETAINED_RELEASE_ADOPTER:-/usr/local/sbin/adopt-henukit-degraded-baseline}"
notice_url_helper="${HENUKIT_NOTICE_URL_HELPER:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/henukit-notice-url-transaction.py}"

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release SHA must be 40 lowercase hexadecimal characters"
if [[ -n "$recovery_baseline_sha" ]]; then
  [[ "$recovery_baseline_sha" =~ ^[0-9a-f]{40}$ ]] ||
    die "--recover-degraded-baseline must be a full lowercase Git SHA"
  [[ "$recovery_baseline_sha" != "$release_sha" ]] ||
    die "recovery baseline and candidate SHA must differ"
fi
[[ -z "$notice_url_plan" || -z "$recovery_baseline_sha" ]] ||
  die "Notice URL change cannot be combined with degraded-baseline recovery"
if [[ "$release_source" == "local" ]]; then
  [[ -d "$local_artifact_dir" && ! -L "$local_artifact_dir" ]] ||
    die "--local-artifacts must name a non-symlink artifact directory"
fi
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_REPO must be an owner/name pair"
[[ "$blocker_issue" =~ ^[1-9][0-9]*$ ]] || die "HENUKIT_BLOCKER_ISSUE must be a positive issue number"
[[ "$branch" =~ ^[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_BRANCH contains unsupported characters"
[[ "$epay_ssh_target" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_EPAY_GATEWAY_SSH_TARGET contains unsupported characters"
[[ "$epay_gateway_dir" =~ ^/[A-Za-z0-9_./-]+$ && "$epay_gateway_dir" != "/" ]] ||
  die "HENUKIT_EPAY_GATEWAY_DIR must be a specific absolute path"
if [[ -n "$retained_release_owner_uid" ]]; then
  [[ -z "$recovery_baseline_sha" ]] ||
    die "HENUKIT_RETAINED_RELEASE_OWNER_UID is only valid for normal activation"
  [[ "$retained_release_owner_uid" =~ ^[1-9][0-9]*$ ]] ||
    die "HENUKIT_RETAINED_RELEASE_OWNER_UID must be a non-root numeric UID"
fi
if [[ "$preserve_existing" == 1 ]]; then
  [[ -z "$notice_url_plan" && -z "$recovery_baseline_sha" && -z "$retained_release_owner_uid" ]] ||
    die "preservation cannot change configuration or adopt degraded authority"
  preservation_helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/henukit-preserve-existing.py"
  [[ -f "$preservation_helper" && ! -L "$preservation_helper" ]] || die "preservation verifier is unavailable"
fi
[[ -x "$watcher" ]] || die "watcher is not executable: $watcher"
command -v gh >/dev/null 2>&1 || die "gh CLI is required"
command -v ssh >/dev/null 2>&1 || die "ssh is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"
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

validate_trusted_helper() {
  local path="$1" label="$2" mode owner
  [[ "$path" == /* && -f "$path" && -x "$path" && ! -L "$path" ]] ||
    die "$label must be an executable absolute regular non-symlink file"
  mode="$(stat -c '%a' "$path" 2>/dev/null || stat -f '%Lp' "$path")"
  owner="$(stat -c '%u' "$path" 2>/dev/null || stat -f '%u' "$path")"
  [[ "$owner" == "$(id -u)" ]] || die "$label must be owned by the release user"
  (( (8#$mode & 8#022) == 0 )) || die "$label must not be group- or world-writable"
  trusted_nonwritable_directory_chain "$(dirname "$path")" ||
    die "$label parent chain is untrusted"
}

if [[ -n "$retained_release_owner_uid" ]]; then
  validate_trusted_helper "$retained_release_adopter" "retained release ownership adopter"
fi

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
recovery_source_binding="$state_root/prepared/${release_sha}.artifact-source"
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

active_aliases_match() {
  local accepted="$1" alias marker mode owner
  [[ "$accepted" =~ ^[0-9a-f]{40}$ ]] || return 1
  local current="${HENUKIT_CURRENT_LINK:-/opt/henukit-current}"
  local compatible="${HENUKIT_COMPAT_CURRENT_LINK:-/opt/henukit/current}"
  local contract="$state_root/active-alias-contract.json" target
  [[ "$current" != "$compatible" ]] || return 1
  marker="$release_root/$accepted/RELEASE_SHA"
  for alias in "$compatible" "$current"; do
    target="$release_root/$accepted"
    [[ "$alias" != "$current" ]] || target="$compatible"
    [[ "$alias" == /* && -L "$alias" && "$(readlink "$alias")" == "$target" ]] || return 1
    owner="$(stat -c '%u' "$alias" 2>/dev/null || stat -f '%u' "$alias")"
    [[ "$owner" == "$(id -u)" ]] || return 1
    trusted_nonwritable_directory_chain "$(dirname "$alias")" || return 1
    [[ -f "$marker" && ! -L "$marker" ]] || return 1
    trusted_nonwritable_directory_chain "$(dirname "$marker")" || return 1
    owner="$(stat -c '%u' "$marker" 2>/dev/null || stat -f '%u' "$marker")"
    mode="$(stat -c '%a' "$marker" 2>/dev/null || stat -f '%Lp' "$marker")"
    [[ "$owner" == "$(id -u)" ]] && (( (8#$mode & 8#022) == 0 )) || return 1
    [[ "$(tr -d '\r\n' < "$marker")" == "$accepted" ]] || return 1
  done
  (validate_private_file "$contract" "active alias contract"; trusted_nonwritable_directory_chain "$(dirname "$contract")") || return 1
  python3 -I - "$contract" "$release_root" "$compatible" "$current" <<'ALIAS_CONTRACT'
import json, os, pathlib, stat, sys
path, releases, inner, outer = map(pathlib.Path, sys.argv[1:])
def metadata(path):
    info = path.lstat()
    return dict(dev=info.st_dev, ino=info.st_ino, uid=info.st_uid, gid=info.st_gid, mode=stat.S_IMODE(info.st_mode))
try:
    info = path.lstat()
    assert stat.S_ISREG(info.st_mode) and info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) == 0o400
    value = json.loads(path.read_bytes())
    assert set(value) == {'format','release_root','inner','outer','outer_snapshot','parents'}
    assert value['format'] == 'henukit-active-alias-forwarder-v1'
    assert (value['release_root'], value['inner'], value['outer']) == tuple(map(str, (releases, inner, outer)))
    assert outer.is_symlink() and os.readlink(outer) == str(inner)
    assert value['outer_snapshot'] == {**metadata(outer), 'text': os.readlink(outer)}
    assert value['parents'] == {str(p): metadata(p) for p in (inner.parent, outer.parent)}
except (OSError, ValueError, KeyError, AssertionError, TypeError):
    sys.exit(1)
ALIAS_CONTRACT
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

validate_recovery_source_binding() {
  validate_private_file "$recovery_source_binding" "recovery artifact source binding"
  [[ "$(tr -d '\r\n' < "$recovery_source_binding")" == "$verified_artifact_identity" ]] ||
    die "existing approval is not bound to $release_source artifacts"
}

blocker_state="$(gh api "repos/$repo/issues/$blocker_issue" --jq '.state')"
[[ "$blocker_state" == "closed" ]] || die "blocker issue #$blocker_issue must be closed before Account Portfolio deployment"

verified_artifact_identity=""
expected_actions_identity="${HENUKIT_EXPECTED_ACTIONS_IDENTITY:-}"
if [[ -n "$expected_actions_identity" ]]; then
  [[ "$release_source" == actions && "$expected_actions_identity" =~ ^actions:[1-9][0-9]*:[1-9][0-9]*$ ]] ||
    die "expected Actions identity is invalid for this release source"
fi
verify_release_current() {
  local branch_head run_row run_id run_attempt run_sha run_status run_conclusion current_identity manifest
  branch_head="$(gh api "repos/$repo/branches/$branch" --jq '.commit.sha')"
  [[ "$branch_head" == "$release_sha" ]] || die "requested release is not the current $branch head"
  if [[ "$release_source" == "local" ]]; then
    manifest="$local_artifact_dir/henukit-release-${release_sha}.manifest"
    [[ -f "$manifest" && -s "$manifest" && ! -L "$manifest" ]] ||
      die "local artifact manifest is missing or untrusted"
    current_identity="local:$(sha256sum "$manifest" | awk '{print $1}')"
  else
    run_row="$(gh run list --repo "$repo" --workflow "$workflow" --branch "$branch" --event push --limit 1 --json databaseId,attempt,headSha,status,conclusion --jq 'first(.[]) | [.databaseId,.attempt,.headSha,.status,.conclusion] | @tsv')"
    IFS=$'\t' read -r run_id run_attempt run_sha run_status run_conclusion <<< "$run_row"
    [[ "$run_id" =~ ^[1-9][0-9]*$ ]] || die "newest workflow run has an invalid identity"
    [[ "$run_attempt" =~ ^[1-9][0-9]*$ ]] || die "newest workflow run has an invalid attempt"
    [[ "$run_sha" == "$release_sha" && "$run_status" == "completed" && "$run_conclusion" == "success" ]] ||
      die "requested release is not the newest completed successful $workflow run"
    current_identity="actions:${run_id}:${run_attempt}"
  fi
  if [[ -n "$expected_actions_identity" && "$expected_actions_identity" != "$current_identity" ]]; then
    die "release Actions identity differs from the automatic authorization"
  fi
  if [[ -n "$verified_artifact_identity" && "$verified_artifact_identity" != "$current_identity" ]]; then
    die "release artifact identity changed during activation"
  fi
  verified_artifact_identity="$current_identity"
}

verify_release_current
baseline_sha="$(basename "$(readlink "${HENUKIT_COMPAT_CURRENT_LINK:-/opt/henukit/current}")")" ||
  die "active alias baseline is unavailable"
active_aliases_match "$baseline_sha" || die "active alias topology requires a reviewed versioned bootstrap"
if [[ -e "$approval" ]]; then
  [[ -n "$recovery_baseline_sha" ]] ||
    die "an approval already exists for release $release_sha"
  validate_existing_approval
  validate_prepared_evidence
  validate_recovery_binding
  validate_recovery_source_binding
  resume_existing_approval=1
  printf '%s: resuming valid unconsumed approval for release %s\n' "$program" "$release_sha"
fi
preservation_receipt="$state_root/prepared/${release_sha}.preserve-existing.json"
preservation_required() {
  local contract backup root="${HENUKIT_BACKUP_ROOT:-/opt/henukit-backups}"
  [[ ! -e "$preservation_receipt" && ! -L "$preservation_receipt" ]] || return 0
  for contract in "$state_root/rollback-contracts/pending/$release_sha" "$state_root/rollback-contracts/completed/$release_sha".*; do
    [[ -f "$contract" && ! -L "$contract" ]] || continue
    if grep -q '^rollback_mode=preserve-existing' "$contract"; then return 0; fi
  done
  if [[ -f "$prepared" && ! -L "$prepared" ]]; then
    backup="$(tr -d '\r\n' < "$prepared")"
    if [[ "$backup" == "$root/"* && "$backup" =~ ^/[A-Za-z0-9_./-]+$ &&
          -f "$backup.meta" && ! -L "$backup.meta" ]]; then
      if grep -q '^preservation_mode=preserve-existing' "$backup.meta"; then return 0; fi
    fi
  fi
  return 1
}
if preservation_required; then
  [[ "$preserve_existing" == 1 ]] || die "existing preservation attempt requires --preserve-existing"
  [[ -f "$preservation_receipt" && ! -L "$preservation_receipt" ]] ||
    die "existing preservation attempt lost its original receipt; operator review required"
fi
if [[ "$preserve_existing" == 1 ]]; then
  validate_private_file "$env_file" "existing production environment"
  validate_trusted_helper "$preservation_helper" "preservation verifier"
  python3 -I "$preservation_helper" capture --sha "$release_sha" --env-file "$env_file" \
    --receipt "$preservation_receipt" --release-dir "$release_root/$baseline_sha" \
    --role "${HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE:-}"
  export HENUKIT_PRESERVE_EXISTING_SHA="$release_sha"
  export HENUKIT_ROLLBACK_ENV_FILE="$env_file"
fi
if [[ -s "$active" && "$(tr -d '\r\n' < "$active")" == "$release_sha" ]]; then
  active_aliases_match "$release_sha" || die "active release aliases disagree; operator reconciliation is required"
  [[ -z "$notice_url_plan" ]] || die "already-active Notice URL change requires the explicit --notice-url apply entry"
  printf '%s: release %s is already active\n' "$program" "$release_sha"
  exit 0
fi

[[ -n "$env_file" && -r "$env_file" && ! -L "$env_file" && ( "$preserve_existing" == 1 || -w "$env_file" ) ]] ||
  die "HENUKIT_ENV_FILE must be readable, non-symlinked and writable for default activation"
if [[ -n "$notice_url_plan" ]]; then
  python3 -I "$notice_url_helper" --mode preflight --sha "$release_sha" --env-file "$env_file" --plan "$notice_url_plan"
  export HENUKIT_NOTICE_URL_PLAN="$notice_url_plan"
  unset NOTICE_API_URL
else
  unset HENUKIT_NOTICE_URL_PLAN
fi

watcher_args=(--once)
if [[ "$release_source" == "local" ]]; then
  watcher_args=(--local-artifacts "$local_artifact_dir" --sha "$release_sha")
fi
if [[ -n "$recovery_baseline_sha" ]]; then
  watcher_args+=(--recover-degraded-baseline "$recovery_baseline_sha")
fi

if [[ "$preserve_existing" == 0 ]]; then
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

fi

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
recovery_source_binding_incoming=""
remote_stage=""
environment_backup=""
if [[ "$preserve_existing" == 0 ]]; then
  environment_backup="$(dirname "$env_file")/.henukit-env.backup.$$"
cp -p "$env_file" "$environment_backup"
chmod 0600 "$environment_backup"
export HENUKIT_ROLLBACK_ENV_FILE="$environment_backup"
fi
cleanup() {
  if [[ -n "${approval_incoming:-}" && -f "$approval_incoming" ]]; then
    rm -f -- "$approval_incoming"
  fi
  if [[ -n "${recovery_binding_incoming:-}" && -f "$recovery_binding_incoming" ]]; then
    rm -f -- "$recovery_binding_incoming"
  fi
  if [[ -n "${recovery_source_binding_incoming:-}" && -f "$recovery_source_binding_incoming" ]]; then
    rm -f -- "$recovery_source_binding_incoming"
  fi
  if [[ "$remote_stage" =~ ^/tmp/henukit-epay-release\.[A-Za-z0-9]+$ ]]; then
    ssh "$epay_ssh_target" "rm -rf -- '$remote_stage'" >/dev/null 2>&1 || true
  fi
  if [[ -n "${environment_backup:-}" && -f "$environment_backup" ]]; then
    mv "$environment_backup" "$env_file" || true
  fi
}
trap cleanup EXIT
if [[ "$preserve_existing" == 0 ]]; then
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_PID "$tenant_pid" || die "could not install the HENU tenant PID"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_KEY "$tenant_key" || die "could not install the HENU tenant key"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL "https://metaview.top/epay" || die "could not configure the EasyPay base URL"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL "https://henukit.cn/api/v1/payment-providers/easypay/notifications" || die "could not configure the EasyPay callback"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL "https://henukit.cn/account/membership" || die "could not configure the EasyPay return URL"
set_account_env_value ACCOUNT_PORTFOLIO_EASYPAY_ENABLED 1 || die "could not atomically enable Account Portfolio EasyPay"
set_account_env_value QUIZCRAFT_CORE_URL "http://quizcraft:10089" || die "could not configure the containerized QuizCraft Core URL"

fi

# With no approval, the first pass can only verify artifacts and restore-test
# backups. An exact local recovery may resume an approval left unconsumed by a
# fail-closed pre-load rejection, but only after independently validating the
# approval and its SHA-bound prepared evidence above.
if [[ "$resume_existing_approval" -eq 0 ]]; then
  "$watcher" "${watcher_args[@]}"
  [[ -s "$prepared" ]] || die "watcher did not prepare verified backup evidence for release $release_sha"
  validate_prepared_evidence
fi

if [[ -n "$retained_release_owner_uid" ]]; then
  validate_private_file "$active" "active release marker"
  previous_release_sha="$(tr -d '\r\n' < "$active")"
  [[ "$previous_release_sha" =~ ^[0-9a-f]{40}$ && "$previous_release_sha" != "$release_sha" ]] ||
    die "active release marker is not a distinct full lowercase Git SHA"
  adoption_args=(
    --sha "$previous_release_sha"
    --candidate-sha "$release_sha"
    --expected-owner-uid "$retained_release_owner_uid"
  )
  verify_release_current
  "$retained_release_adopter" "${adoption_args[@]}" --preflight
  verify_release_current
  "$retained_release_adopter" "${adoption_args[@]}" --execute
  verify_release_current
fi

release_dir="$release_root/$release_sha"
if [[ "$preserve_existing" == 0 ]]; then
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

fi
if [[ "$preserve_existing" == 1 ]]; then
  python3 -I "$preservation_helper" candidate --sha "$release_sha" --env-file "$env_file" \
    --receipt "$preservation_receipt" --release-dir "$release_dir" \
    --role "$HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE"
fi

if [[ "$resume_existing_approval" -eq 0 ]]; then
  umask 077
  if [[ -n "$recovery_baseline_sha" ]]; then
    recovery_binding_incoming="$(mktemp "$state_root/prepared/.${release_sha}.recovery-baseline.XXXXXX")"
    printf '%s\n' "$recovery_baseline_sha" > "$recovery_binding_incoming"
    chmod 0600 "$recovery_binding_incoming"
    mv "$recovery_binding_incoming" "$recovery_binding"
    recovery_binding_incoming=""
    recovery_source_binding_incoming="$(mktemp "$state_root/prepared/.${release_sha}.artifact-source.XXXXXX")"
    printf '%s\n' "$verified_artifact_identity" > "$recovery_source_binding_incoming"
    chmod 0600 "$recovery_source_binding_incoming"
    mv "$recovery_source_binding_incoming" "$recovery_source_binding"
    recovery_source_binding_incoming=""
  elif [[ -e "$recovery_binding" || -e "$recovery_source_binding" ]]; then
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
active_aliases_match "$release_sha" || die "watcher returned without the complete active-alias contract"
if [[ "$preserve_existing" == 1 ]]; then
  python3 -I "$preservation_helper" runtime --sha "$release_sha" --env-file "$env_file" \
    --receipt "$preservation_receipt" --release-dir "$release_dir" \
    --role "$HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE"
else
  rm -f -- "$environment_backup"
  environment_backup=""
fi

printf '%s: release %s activated\n' "$program" "$release_sha"
