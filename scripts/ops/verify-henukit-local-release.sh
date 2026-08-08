#!/usr/bin/env bash
# Verify a fixed-SHA artifact set assembled by the controlled Linux/amd64
# builder. The inventory and allowed-signers file are local trust roots; this
# script deliberately never executes anything from the candidate artifact set.
set -Eeuo pipefail

program="verify-henukit-local-release"

usage() {
  cat >&2 <<'EOF'
usage: verify-henukit-local-release.sh --artifact-dir <dir> --sha <full-main-sha> \
  --inventory <trusted-inventory> --allowed-signers <trusted-allowed-signers>

The artifact directory must be flat and contain exactly the image archives,
runtime archive, checksums, RELEASE_SHA marker, and the signed local manifest
for the supplied SHA.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  else
    shasum -a 256 "$file" | awk '{print $1}'
  fi
}

safe_trust_root() {
  local file="$1"
  local mode
  [[ -f "$file" && -r "$file" && ! -L "$file" ]] ||
    die "trusted file must be a readable regular non-symlink: $file"
  mode="$(stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || die "could not determine trusted file mode: $file"
  (( (8#$mode & 8#022) == 0 )) || die "trusted file must not be group- or world-writable: $file"
}

artifact_dir=""
release_sha=""
inventory=""
allowed_signers=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact-dir)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      artifact_dir="$2"
      shift 2
      ;;
    --sha)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      release_sha="$2"
      shift 2
      ;;
    --inventory)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      inventory="$2"
      shift 2
      ;;
    --allowed-signers)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      allowed_signers="$2"
      shift 2
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
[[ -n "$artifact_dir" && -d "$artifact_dir" && ! -L "$artifact_dir" ]] ||
  die "--artifact-dir must be a readable, non-symlink directory"
safe_trust_root "$inventory"
safe_trust_root "$allowed_signers"
[[ -x "$inventory" ]] || die "trusted inventory is not executable: $inventory"
command -v ssh-keygen >/dev/null 2>&1 || die "ssh-keygen with -Y support is required"
command -v cmp >/dev/null 2>&1 || die "cmp is required"

inventory_images=()
while IFS= read -r image; do
  [[ "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] || die "trusted inventory returned invalid image: $image"
  inventory_images+=("$image")
done < <("$inventory" --artifact-images)
[[ "${#inventory_images[@]}" -gt 0 ]] || die "trusted inventory returned no images"

manifest="henukit-release-${release_sha}.manifest"
signature="${manifest}.sig"
expected_names=(RELEASE_SHA "$manifest" "$signature")
for image in "${inventory_images[@]}"; do
  archive="${image}-${release_sha}.docker.tar.gz"
  expected_names+=("$archive" "${archive}.sha256")
done
runtime_archive="henukit-runtime-${release_sha}.tar.gz"
expected_names+=("$runtime_archive" "${runtime_archive}.sha256")

expected_name() {
  local candidate="$1"
  local expected
  for expected in "${expected_names[@]}"; do
    [[ "$candidate" == "$expected" ]] && return 0
  done
  return 1
}

while IFS= read -r -d '' candidate; do
  die "artifact set contains symbolic link $(basename "$candidate")"
done < <(find "$artifact_dir" -mindepth 1 -type l -print0)
while IFS= read -r -d '' candidate; do
  die "artifact set contains unexpected non-file $(basename "$candidate")"
done < <(find "$artifact_dir" -mindepth 1 ! -type f -print0)
while IFS= read -r -d '' candidate; do
  expected_name "$(basename "$candidate")" || die "unexpected artifact file $(basename "$candidate")"
done < <(find "$artifact_dir" -mindepth 1 -maxdepth 1 -type f -print0)

for expected in "${expected_names[@]}"; do
  [[ -s "$artifact_dir/$expected" && -f "$artifact_dir/$expected" && ! -L "$artifact_dir/$expected" ]] ||
    die "artifact set is missing $expected"
done
[[ "$(tr -d '[:space:]' < "$artifact_dir/RELEASE_SHA")" == "$release_sha" ]] ||
  die "artifact RELEASE_SHA does not match requested SHA"

signer="${HENUKIT_RELEASE_SIGNER:-henukit-release}"
namespace="${HENUKIT_RELEASE_SIGNATURE_NAMESPACE:-henukit-release}"
[[ "$signer" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_RELEASE_SIGNER contains unsupported characters"
[[ "$namespace" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_RELEASE_SIGNATURE_NAMESPACE contains unsupported characters"

inventory_sha="$(sha256_file "$inventory")"
scratch="$(mktemp "${TMPDIR:-/tmp}/henukit-local-manifest.XXXXXX")"
trap 'rm -f -- "$scratch"' EXIT
{
  printf 'format=henukit-local-release-v1\n'
  printf 'release_sha=%s\n' "$release_sha"
  printf 'source_ref=refs/heads/main\n'
  printf 'builder_platform=linux/amd64\n'
  printf 'signer=%s\n' "$signer"
  printf 'signature_namespace=%s\n' "$namespace"
  printf 'inventory_sha256=%s\n' "$inventory_sha"
  for image in "${inventory_images[@]}"; do
    archive="${image}-${release_sha}.docker.tar.gz"
    digest="$(sha256_file "$artifact_dir/$archive")"
    printf 'artifact_sha256=%s  %s\n' "$digest" "$archive"
    [[ "$(tr -d '\r\n' < "$artifact_dir/${archive}.sha256")" == "$digest  $archive" ]] ||
      die "artifact checksum file does not match its archive: $archive"
  done
  digest="$(sha256_file "$artifact_dir/$runtime_archive")"
  printf 'artifact_sha256=%s  %s\n' "$digest" "$runtime_archive"
  [[ "$(tr -d '\r\n' < "$artifact_dir/${runtime_archive}.sha256")" == "$digest  $runtime_archive" ]] ||
    die "runtime checksum file does not match its archive"
} > "$scratch"

cmp -s "$scratch" "$artifact_dir/$manifest" || die "signed manifest is not the exact artifact manifest"
ssh-keygen -Y verify \
  -f "$allowed_signers" \
  -I "$signer" \
  -n "$namespace" \
  -s "$artifact_dir/$signature" < "$artifact_dir/$manifest" >/dev/null ||
  die "local artifact manifest signature verification failed"

printf '%s: verified signed local artifact set for %s\n' "$program" "$release_sha"
