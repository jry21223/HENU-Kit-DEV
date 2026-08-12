#!/usr/bin/env bash
# Atomically add one reviewed release signer while retaining an explicitly
# named existing signer for a bounded rollback window.
set -Eeuo pipefail

program="rotate-henukit-release-signers"
state_root="${HENUKIT_SIGNER_ROTATION_STATE_ROOT:-/var/lib/henukit-release-signers}"
trust_anchor="${HENUKIT_TRUST_ANCHOR:-/}"

die() { printf '%s: %s\n' "$program" "$*" >&2; exit 1; }
usage() {
  cat >&2 <<'EOF'
usage: rotate-henukit-release-signers.sh \
  --allowed-signers <root-owned-file> \
  --new-public-key <root-owned-public-key> \
  --principal <allowed-signers-principal> \
  --retain-fingerprint <SHA256:fingerprint> \
  --candidate-sha <full-current-main-sha> \
  --preflight|--execute
EOF
}

allowed_signers=""
new_public_key=""
principal=""
retain_fingerprint=""
candidate_sha=""
mode=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --allowed-signers) allowed_signers="${2:-}"; shift 2 ;;
    --new-public-key) new_public_key="${2:-}"; shift 2 ;;
    --principal) principal="${2:-}"; shift 2 ;;
    --retain-fingerprint) retain_fingerprint="${2:-}"; shift 2 ;;
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --preflight|--execute) [[ -z "$mode" ]] || die "choose one mode"; mode="$1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 64 ;;
  esac
done

[[ "$allowed_signers" == /* && "$new_public_key" == /* && "$state_root" == /* && "$trust_anchor" == /* ]] ||
  die "all configured paths must be absolute"
[[ "$principal" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "principal contains unsupported characters"
[[ "$retain_fingerprint" =~ ^SHA256:[A-Za-z0-9+/=]+$ ]] || die "retained fingerprint is invalid"
[[ "$candidate_sha" =~ ^[0-9a-f]{40}$ ]] || die "candidate SHA must be a full lowercase Git SHA"
[[ -n "$mode" ]] || { usage; exit 64; }
[[ "$(id -u)" == "0" ]] || die "signer rotation must run as root"

file_mode() { stat -c '%a' "$1"; }
file_owner() { stat -c '%u' "$1"; }
trusted_chain_to_anchor() {
  local path="$1"
  while :; do
    [[ -d "$path" && ! -L "$path" ]] || return 1
    [[ "$(file_owner "$path")" == "0" ]] || return 1
    (( (8#$(file_mode "$path") & 8#022) == 0 )) || return 1
    [[ "$path" == "$trust_anchor" ]] && return 0
    [[ "$path" != "/" ]] || return 1
    path="$(dirname "$path")"
  done
}
trusted_root_file() {
  local path="$1"
  [[ -f "$path" && ! -L "$path" && "$(file_owner "$path")" == "0" ]] || return 1
  (( (8#$(file_mode "$path") & 8#022) == 0 )) || return 1
  trusted_chain_to_anchor "$(dirname "$path")"
}
trusted_private_directory() {
  local path="$1"
  [[ -d "$path" && ! -L "$path" && "$(file_owner "$path")" == "0" ]] || return 1
  (( (8#$(file_mode "$path") & 8#077) == 0 )) || return 1
  trusted_chain_to_anchor "$(dirname "$path")"
}
fingerprint_of() {
  ssh-keygen -l -E sha256 -f "$1" | awk 'NR == 1 { print $2 }'
}
durable_move() {
  local source="$1" target="$2" directory
  directory="$(dirname "$target")"
  sync -f "$source"
  mv -T "$source" "$target"
  sync -f "$target"
  sync -f "$directory"
}

trusted_root_file "$allowed_signers" || die "allowed-signers trust root is not root-owned and non-writable"
trusted_root_file "$new_public_key" || die "new public key is not a trusted root-owned file"
trusted_private_directory "$state_root" || die "rotation state directory is not root-owned and private"
if [[ "$mode" == "--execute" ]]; then
  exec 9<"$state_root"
  flock -n 9 || die "another signer rotation is already running"
fi

umask 077
scratch="$(mktemp -d "${TMPDIR:-/tmp}/henukit-signer-rotation.XXXXXX")"
cleanup() { rm -rf -- "$scratch"; }
trap cleanup EXIT

read -r new_type new_blob new_comment < "$new_public_key" || die "new public key is empty"
[[ "$new_type" == "ssh-ed25519" && -n "$new_blob" ]] || die "new public key must be one ssh-ed25519 key"
[[ "$(awk 'END { print NR }' "$new_public_key")" == "1" ]] || die "new public key file must contain exactly one line"
printf '%s %s\n' "$new_type" "$new_blob" > "$scratch/new.pub"
new_fingerprint="$(fingerprint_of "$scratch/new.pub")"
[[ "$new_fingerprint" =~ ^SHA256: ]] || die "could not derive the new signer fingerprint"

retain_count=0
new_count=0
line_number=0
while IFS= read -r line || [[ -n "$line" ]]; do
  line_number=$((line_number + 1))
  [[ -n "$line" ]] || die "allowed-signers contains a blank line"
  read -r line_principal key_type key_blob trailing <<< "$line"
  [[ "$line_principal" =~ ^[A-Za-z0-9_.@-]+$ && "$key_type" == "ssh-ed25519" && -n "$key_blob" ]] ||
    die "allowed-signers line $line_number is invalid"
  printf '%s %s\n' "$key_type" "$key_blob" > "$scratch/line.pub"
  line_fingerprint="$(fingerprint_of "$scratch/line.pub")"
  [[ "$line_fingerprint" == "$retain_fingerprint" ]] && retain_count=$((retain_count + 1))
  if [[ "$line_fingerprint" == "$new_fingerprint" ]]; then
    [[ "$line_principal" == "$principal" ]] || die "new signer already exists for a different principal"
    new_count=$((new_count + 1))
  fi
done < "$allowed_signers"
[[ "$retain_count" == "1" ]] || die "retained fingerprint must identify exactly one existing signer"
(( new_count <= 1 )) || die "new signer fingerprint is duplicated"

cp -- "$allowed_signers" "$scratch/desired"
if [[ "$new_count" == "0" ]]; then
  printf '%s %s %s\n' "$principal" "$new_type" "$new_blob" >> "$scratch/desired"
fi
before_sha256="$(sha256sum "$allowed_signers" | awk '{print $1}')"
after_sha256="$(sha256sum "$scratch/desired" | awk '{print $1}')"
rotations="$state_root/rotations"
audit="$rotations/${candidate_sha}.rotated"
intent="${audit}.rotating"
backup="$rotations/${candidate_sha}.allowed-signers.before"

if [[ -e "$rotations" ]]; then
  trusted_private_directory "$rotations" || die "rotation audit directory is not root-owned and private"
fi
if [[ "$new_count" == "1" && ! -e "$audit" && ! -e "$intent" ]]; then
  die "new signer is installed without a matching rotation audit"
fi
for existing_record in "$audit" "$intent"; do
  if [[ -e "$existing_record" ]]; then
    trusted_root_file "$existing_record" || die "signer rotation record is untrusted"
    [[ "$(grep -Fxc "candidate_sha=$candidate_sha" "$existing_record")" == "1" &&
       "$(grep -Fxc "principal=$principal" "$existing_record")" == "1" &&
       "$(grep -Fxc "old_fingerprint=$retain_fingerprint" "$existing_record")" == "1" &&
       "$(grep -Fxc "new_fingerprint=$new_fingerprint" "$existing_record")" == "1" &&
       "$(grep -Fxc "after_sha256=$after_sha256" "$existing_record")" == "1" ]] ||
      die "signer rotation record does not match this exact transition"
  fi
done
if [[ -e "$audit" ]]; then
  audited_before_sha256="$(sed -n 's/^before_sha256=//p' "$audit")"
  trusted_root_file "$backup" || die "completed rotation before-image backup is missing or untrusted"
  [[ "$(sha256sum "$backup" | awk '{print $1}')" == "$audited_before_sha256" ]] ||
    die "completed rotation before-image backup digest does not match"
fi

if [[ "$mode" == "--preflight" ]]; then
  printf '%s: preflight passed for %s; new fingerprint %s\n' "$program" "$candidate_sha" "$new_fingerprint"
  exit 0
fi

if [[ -e "$rotations" ]]; then
  trusted_private_directory "$rotations" || die "rotation audit directory is not root-owned and private"
else
  install -d -o root -g root -m 0700 "$rotations"
  sync -f "$rotations"
  sync -f "$state_root"
  trusted_private_directory "$rotations" || die "could not establish a trusted rotation audit directory"
fi

validate_record() {
  local record="$1"
  trusted_root_file "$record" || die "signer rotation record is untrusted"
  [[ "$(grep -Fxc "candidate_sha=$candidate_sha" "$record")" == "1" &&
     "$(grep -Fxc "principal=$principal" "$record")" == "1" &&
     "$(grep -Fxc "old_fingerprint=$retain_fingerprint" "$record")" == "1" &&
     "$(grep -Fxc "new_fingerprint=$new_fingerprint" "$record")" == "1" &&
     "$(grep -Ec '^before_sha256=[0-9a-f]{64}$' "$record")" == "1" &&
     "$(grep -Ec '^after_sha256=[0-9a-f]{64}$' "$record")" == "1" ]] ||
    die "signer rotation record does not match this exact transition"
  record_before_sha256="$(sed -n 's/^before_sha256=//p' "$record")"
  record_after_sha256="$(sed -n 's/^after_sha256=//p' "$record")"
  [[ "$record_after_sha256" == "$after_sha256" ]] ||
    die "signer rotation record targets different allowed-signers content"
  if [[ "$new_count" == "0" ]]; then
    [[ "$record_before_sha256" == "$before_sha256" ]] ||
      die "signer rotation record starts from different allowed-signers content"
  fi
}

if [[ -e "$audit" ]]; then
  validate_record "$audit"
  [[ "$(sha256sum "$allowed_signers" | awk '{print $1}')" == "$record_after_sha256" ]] ||
    die "audited allowed-signers content no longer matches"
  printf '%s: rotation already completed; new fingerprint %s\n' "$program" "$new_fingerprint"
  exit 0
fi

if [[ -e "$intent" ]]; then
  validate_record "$intent"
  transition_before_sha256="$record_before_sha256"
  transition_after_sha256="$record_after_sha256"
else
  [[ "$new_count" == "0" ]] || die "new signer is installed without a matching rotation audit"
  transition_before_sha256="$before_sha256"
  transition_after_sha256="$after_sha256"
  incoming="$(mktemp "$rotations/.${candidate_sha}.rotating.XXXXXX")"
  {
    printf 'candidate_sha=%s\n' "$candidate_sha"
    printf 'principal=%s\n' "$principal"
    printf 'old_fingerprint=%s\n' "$retain_fingerprint"
    printf 'new_fingerprint=%s\n' "$new_fingerprint"
    printf 'before_sha256=%s\n' "$before_sha256"
    printf 'after_sha256=%s\n' "$after_sha256"
    printf 'recorded_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } > "$incoming"
  chmod 0400 "$incoming"
  durable_move "$incoming" "$intent"
fi

if [[ ! -e "$backup" ]]; then
  backup_incoming="$(mktemp "$rotations/.${candidate_sha}.allowed-signers.before.XXXXXX")"
  cp -- "$allowed_signers" "$backup_incoming"
  chmod 0400 "$backup_incoming"
  durable_move "$backup_incoming" "$backup"
fi
trusted_root_file "$backup" || die "allowed-signers backup is untrusted"
[[ "$(sha256sum "$backup" | awk '{print $1}')" == "$transition_before_sha256" ]] ||
  die "allowed-signers backup does not match the rotation intent"

current_sha256="$(sha256sum "$allowed_signers" | awk '{print $1}')"
if [[ "$current_sha256" == "$transition_before_sha256" ]]; then
  allowed_dir="$(dirname "$allowed_signers")"
  replacement="$(mktemp "$allowed_dir/.release-signers.XXXXXX")"
  cp -- "$scratch/desired" "$replacement"
  chown "$(stat -c '%u:%g' "$allowed_signers")" "$replacement"
  chmod "$(file_mode "$allowed_signers")" "$replacement"
  sync -f "$replacement"
  mv -T "$replacement" "$allowed_signers"
  sync -f "$allowed_dir"
elif [[ "$current_sha256" != "$transition_after_sha256" ]]; then
  die "allowed-signers changed outside this exact rotation"
fi

trusted_root_file "$allowed_signers" || die "installed allowed-signers file is untrusted"
[[ "$(sha256sum "$allowed_signers" | awk '{print $1}')" == "$transition_after_sha256" ]] ||
  die "installed allowed-signers content does not match the rotation intent"
durable_move "$intent" "$audit"
printf '%s: rotation completed for %s; new fingerprint %s\n' "$program" "$candidate_sha" "$new_fingerprint"
