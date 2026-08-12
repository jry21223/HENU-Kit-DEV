#!/usr/bin/env bash
# One-time, explicit ownership adoption for a complete retained release that
# predates the root-owned degraded-recovery trust boundary.
set -Eeuo pipefail

program="adopt-henukit-degraded-baseline"
release_root="${HENUKIT_RELEASE_ROOT:-/opt/henukit-releases}"
current_link="${HENUKIT_CURRENT_LINK:-/opt/henukit-current}"
state_root="${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}"
trust_anchor="${HENUKIT_TRUST_ANCHOR:-/}"

die() { printf '%s: %s\n' "$program" "$*" >&2; exit 1; }
usage() {
  cat >&2 <<'EOF'
usage: adopt-henukit-degraded-baseline.sh \
  --sha <full-current-sha> \
  --candidate-sha <full-candidate-sha> \
  --expected-owner-uid <historical-builder-uid> \
  --preflight|--execute
EOF
}

baseline_sha=""
candidate_sha=""
expected_owner_uid=""
mode=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha) baseline_sha="${2:-}"; shift 2 ;;
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --expected-owner-uid) expected_owner_uid="${2:-}"; shift 2 ;;
    --preflight|--execute) [[ -z "$mode" ]] || die "choose one mode"; mode="$1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 64 ;;
  esac
done

[[ "$baseline_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
[[ "$candidate_sha" =~ ^[0-9a-f]{40}$ && "$candidate_sha" != "$baseline_sha" ]] ||
  die "--candidate-sha must be a different full lowercase Git SHA"
[[ "$expected_owner_uid" =~ ^[1-9][0-9]*$ ]] || die "--expected-owner-uid must be a non-root numeric UID"
[[ -n "$mode" ]] || { usage; exit 64; }
[[ "$(id -u)" == "0" ]] || die "ownership adoption must run as root"

for path in "$release_root" "$current_link" "$state_root" "$trust_anchor"; do
  [[ "$path" == /* ]] || die "all configured paths must be absolute"
done

file_mode() { stat -c '%a' "$1"; }
file_owner() { stat -c '%u' "$1"; }
trusted_chain_to_anchor() {
  local path="$1" parent
  parent="$path"
  while :; do
    [[ -d "$parent" && ! -L "$parent" ]] || return 1
    [[ "$(file_owner "$parent")" == "0" ]] || return 1
    (( (8#$(file_mode "$parent") & 8#022) == 0 )) || return 1
    [[ "$parent" == "$trust_anchor" ]] && return 0
    [[ "$parent" != "/" ]] || return 1
    parent="$(dirname "$parent")"
  done
}
trusted_root_file() {
  local path="$1"
  [[ -f "$path" && ! -L "$path" && "$(file_owner "$path")" == "0" ]] || return 1
  (( (8#$(file_mode "$path") & 8#022) == 0 ))
}
trusted_root_private_directory() {
  local path="$1"
  [[ -d "$path" && ! -L "$path" && "$(file_owner "$path")" == "0" ]] || return 1
  (( (8#$(file_mode "$path") & 8#077) == 0 )) || return 1
  trusted_chain_to_anchor "$(dirname "$path")"
}

release_dir="$release_root/$baseline_sha"
marker="$release_dir/RELEASE_SHA"
compose="$release_dir/docker-compose.henukit.release.yml"
helper="$release_dir/bin/deploy-henukit-artifact.sh"
audit_dir="$state_root/degraded-recoveries"
audit="$audit_dir/${candidate_sha}.baseline-adopted"
intent="${audit}.adopting"

[[ -L "$current_link" && "$(file_owner "$current_link")" == "0" ]] ||
  die "current release link must be a root-owned symlink"
[[ "$(readlink -f "$current_link")" == "$(readlink -f "$release_dir")" ]] ||
  die "declared baseline does not own the current release link"
trusted_chain_to_anchor "$release_root" || die "release root trust chain is not root-owned and non-writable"
trusted_chain_to_anchor "$(dirname "$current_link")" ||
  die "current link parent trust chain is not root-owned and non-writable"
trusted_chain_to_anchor "$state_root" || die "state root trust chain is not root-owned and non-writable"
exec 9<"$state_root"
flock -n 9 || die "another baseline adoption is already running"
if [[ -e "$audit_dir" ]]; then
  trusted_root_private_directory "$audit_dir" ||
    die "degraded recovery audit directory is not root-owned and private"
elif [[ "$mode" == "--execute" ]]; then
  install -d -o root -g root -m 0700 "$audit_dir"
  trusted_root_private_directory "$audit_dir" ||
    die "could not establish a trusted degraded recovery audit directory"
fi
[[ -d "$release_dir" && ! -L "$release_dir" ]] || die "retained release directory is missing or symlinked"
[[ -f "$marker" && ! -L "$marker" && "$(tr -d '[:space:]' < "$marker")" == "$baseline_sha" ]] ||
  die "retained release has no exact RELEASE_SHA"
[[ -f "$compose" && ! -L "$compose" ]] || die "retained release has no regular Compose contract"
[[ -f "$helper" && ! -L "$helper" && -x "$helper" ]] ||
  die "retained release has no executable deployment helper"
[[ -z "$(find "$release_dir" -xdev \( -type l -o -type b -o -type c -o -type p -o -type s \) -print -quit)" ]] ||
  die "retained release contains a symlink or special file"
unexpected_owner="$(find "$release_dir" -xdev ! -uid 0 ! -uid "$expected_owner_uid" -print -quit)"
[[ -z "$unexpected_owner" ]] || die "retained release contains an unexpected owner"

validate_record() {
  local record="$1"
  trusted_root_file "$record" || die "ownership adoption record is untrusted"
  [[ "$(grep -Fxc "candidate_sha=$candidate_sha" "$record")" == "1" &&
     "$(grep -Fxc "previous_sha=$baseline_sha" "$record")" == "1" &&
     "$(grep -Fxc "previous_owner_uid=$expected_owner_uid" "$record")" == "1" &&
     "$(grep -Ec '^metadata_sha256=[0-9a-f]{64}$' "$record")" == "1" &&
     "$(grep -Ec '^content_sha256=[0-9a-f]{64}$' "$record")" == "1" ]] ||
    die "ownership adoption record does not match this tuple"
}

if [[ -e "$audit" ]]; then
  validate_record "$audit"
  [[ -z "$(find "$release_dir" -xdev ! -uid 0 -print -quit)" ]] ||
    die "audited retained release is no longer root-owned"
  printf '%s: ownership adoption already complete for %s\n' "$program" "$baseline_sha"
  exit 0
fi

owner_entry="$(find "$release_dir" -xdev -uid "$expected_owner_uid" -print -quit)"
if [[ -e "$intent" ]]; then
  validate_record "$intent"
elif [[ -z "$owner_entry" ]]; then
  die "retained release is root-owned without the required adoption audit"
fi

metadata_digest="$(find "$release_dir" -xdev -printf '%P\0%y\0%m\0%u\0%g\0' | LC_ALL=C sort -z | sha256sum | awk '{print $1}')"
content_digest="$(find "$release_dir" -xdev -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum --zero | sha256sum | awk '{print $1}')"
if [[ -e "$intent" ]]; then
  recorded_content_digest="$(sed -n 's/^content_sha256=//p' "$intent")"
  [[ "$recorded_content_digest" == "$content_digest" ]] ||
    die "retained release content changed during ownership adoption"
fi
if [[ "$mode" == "--preflight" ]]; then
  printf '%s: preflight passed for %s owner %s\n' "$program" "$baseline_sha" "$expected_owner_uid"
  exit 0
fi

if [[ ! -e "$intent" ]]; then
  umask 077
  incoming="$(mktemp "$audit_dir/.${candidate_sha}.baseline-adopting.XXXXXX")"
  {
    printf 'candidate_sha=%s\n' "$candidate_sha"
    printf 'previous_sha=%s\n' "$baseline_sha"
    printf 'previous_owner_uid=%s\n' "$expected_owner_uid"
    printf 'metadata_sha256=%s\n' "$metadata_digest"
    printf 'content_sha256=%s\n' "$content_digest"
    printf 'recorded_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "$incoming"
  chmod 0400 "$incoming"
  mv "$incoming" "$intent"
fi

chown -R -h root:root "$release_dir"
chmod -R go-w "$release_dir"
[[ -z "$(find "$release_dir" -xdev ! -uid 0 -print -quit)" ]] || die "ownership adoption did not converge"
trusted_root_file "$marker" && trusted_root_file "$compose" && trusted_root_file "$helper" ||
  die "adopted retained release did not satisfy root trust"
adopted_content_digest="$(find "$release_dir" -xdev -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum --zero | sha256sum | awk '{print $1}')"
recorded_content_digest="$(sed -n 's/^content_sha256=//p' "$intent")"
[[ "$adopted_content_digest" == "$recorded_content_digest" ]] ||
  die "retained release content changed during ownership adoption"

mv "$intent" "$audit"
printf '%s: ownership adoption completed for %s\n' "$program" "$baseline_sha"
