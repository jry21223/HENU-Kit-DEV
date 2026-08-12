#!/usr/bin/env bash
# Build a signed, fixed-SHA HENU Kit artifact set on a clean Linux/amd64
# builder. This is the controlled substitute for the Actions artifact job; it
# intentionally has no SSH, upload, or production credentials.
set -Eeuo pipefail

program="build-henukit-release-local"

usage() {
  cat >&2 <<'EOF'
usage: build-henukit-release-local.sh --sha <full-main-sha> --output-dir <directory> \
  --signing-key <private-key-or-agent-public-key> --handoff-group <deployment-reader-group>

The builder must be a clean checkout at the exact current origin/main SHA, on
Linux x86_64 with a linux/amd64 Docker daemon. It writes a signed artifact
directory but never uploads or deploys it.
After signature verification it makes only the completed bundle read-only to
the named deployment-reader group; the private signing key remains owner-only.
For a passphrase-protected key, pass its owner-only `.pub` file as a public key
handle after loading the matching private key into a temporary ssh-agent.
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

write_checksum() {
  local directory="$1"
  local file="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$directory" && sha256sum "$file" > "${file}.sha256")
  else
    (cd "$directory" && shasum -a 256 "$file" > "${file}.sha256")
  fi
}

safe_signing_key() {
  local file="$1"
  local mode
  [[ -f "$file" && -r "$file" && ! -L "$file" ]] ||
    die "--signing-key must be a readable regular non-symlink file"
  mode="$(stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || die "could not determine signing-key mode"
  (( (8#$mode & 8#077) == 0 )) || die "--signing-key must not be accessible by group or world"
}

safe_output_directory() {
  local directory="$1"
  local mode
  [[ -d "$directory" && ! -L "$directory" ]] ||
    die "--output-dir must be a non-symlink directory"
  mode="$(stat -c '%a' "$directory" 2>/dev/null || stat -f '%Lp' "$directory")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || die "could not determine output-dir mode"
  (( (8#$mode & 8#022) == 0 )) ||
    die "--output-dir must not be group- or world-writable"
}

require_wsl2() {
  local kernel_release
  kernel_release="$(uname -r)"
  [[ "$kernel_release" =~ [Ww][Ss][Ll]2 ]] ||
    die "builder must run on WSL2; a generic Linux host is not an authorized builder"
}

release_sha=""
output_dir=""
signing_key=""
handoff_group=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      release_sha="$2"
      shift 2
      ;;
    --output-dir)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      output_dir="$2"
      shift 2
      ;;
    --signing-key)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      signing_key="$2"
      shift 2
      ;;
    --handoff-group)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      handoff_group="$2"
      shift 2
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
[[ -n "$output_dir" && -n "$signing_key" && -n "$handoff_group" ]] || { usage; exit 64; }
[[ "$handoff_group" =~ ^[a-z_][a-z0-9_-]{0,31}$ ]] ||
  die "--handoff-group contains unsupported characters"
[[ "$(uname -s)" == "Linux" ]] || die "builder must run on Linux; use the x86_64 WSL builder, not macOS Docker"
case "$(uname -m)" in
  x86_64|amd64) ;;
  *) die "builder CPU must be x86_64/amd64" ;;
esac
require_wsl2
safe_signing_key "$signing_key"
signing_key="$(cd "$(dirname "$signing_key")" && pwd -P)/$(basename "$signing_key")"
command -v git >/dev/null 2>&1 || die "git is required"
command -v docker >/dev/null 2>&1 || die "docker is required"
command -v gzip >/dev/null 2>&1 || die "gzip is required"
command -v ssh-keygen >/dev/null 2>&1 || die "ssh-keygen with -Y support is required"
command -v getent >/dev/null 2>&1 || die "getent is required to validate the handoff group"
getent group "$handoff_group" >/dev/null || die "--handoff-group does not exist"
id -nG | tr ' ' '\n' | grep -Fx -- "$handoff_group" >/dev/null ||
  die "builder must belong to --handoff-group"

if [[ "$signing_key" == *.pub ]]; then
  command -v ssh-add >/dev/null 2>&1 || die "ssh-add is required for an agent-backed signing key"
  [[ -n "${SSH_AUTH_SOCK:-}" ]] || die "public key handle requires a temporary ssh-agent"
  signing_fingerprint="$(ssh-keygen -l -E sha256 -f "$signing_key" | awk 'NR == 1 { print $2 }')"
  ssh-add -l -E sha256 | awk '{print $2}' | grep -Fx -- "$signing_fingerprint" >/dev/null ||
    die "public key handle has no matching key in ssh-agent"
  read -r signing_type signing_blob signing_comment < "$signing_key" || die "public key handle is empty"
  [[ "$signing_type" == "ssh-ed25519" && -n "$signing_blob" ]] ||
    die "public key handle must contain one ssh-ed25519 key"
  signing_public_key="$signing_type $signing_blob"
else
  signing_public_key="$(ssh-keygen -y -f "$signing_key")"
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
inventory="$repo_root/scripts/ops/henukit-release-images.sh"
runtime_packager="$repo_root/scripts/ops/package-henukit-runtime.sh"
verifier="$repo_root/scripts/ops/verify-henukit-local-release.sh"
[[ -x "$inventory" && -x "$runtime_packager" && -x "$verifier" ]] ||
  die "release inventory, runtime packager, and verifier must be executable"
"$inventory" --check

source_tree="$(git -C "$repo_root" rev-parse "${release_sha}^{tree}")" ||
  die "requested SHA is not available as a source tree in this checkout"
assert_source_snapshot() {
  [[ "$(git -C "$repo_root" rev-parse HEAD)" == "$release_sha" ]] ||
    die "checkout HEAD does not match requested SHA"
  [[ "$(git -C "$repo_root" rev-parse HEAD^{tree})" == "$source_tree" ]] ||
    die "checkout tree does not match requested SHA"
  [[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
    die "builder checkout must be clean, including untracked files"
}
assert_source_snapshot

remote_main_sha() {
  git -C "$repo_root" ls-remote --exit-code origin refs/heads/main | awk 'NR == 1 { print $1 }'
}
[[ "$(remote_main_sha)" == "$release_sha" ]] ||
  die "requested SHA is not the current origin/main head"
[[ "$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}')" == "linux/amd64" ]] ||
  die "Docker server must be linux/amd64"

signer="${HENUKIT_RELEASE_SIGNER:-henukit-release}"
namespace="${HENUKIT_RELEASE_SIGNATURE_NAMESPACE:-henukit-release}"
[[ "$signer" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_RELEASE_SIGNER contains unsupported characters"
[[ "$namespace" =~ ^[A-Za-z0-9_.@-]+$ ]] || die "HENUKIT_RELEASE_SIGNATURE_NAMESPACE contains unsupported characters"

install -d -m 0750 "$output_dir"
chgrp -- "$handoff_group" "$output_dir"
chmod 0750 "$output_dir"
safe_output_directory "$output_dir"
output_dir="$(cd "$output_dir" && pwd -P)"
case "$signing_key" in
  "$output_dir"/*) die "--signing-key must remain outside the artifact handoff tree" ;;
esac
final_dir="$output_dir/henukit-release-$release_sha"
[[ ! -e "$final_dir" ]] || die "refusing to overwrite existing artifact directory: $final_dir"
incoming="$(mktemp -d "$output_dir/.henukit-release-${release_sha}.incoming.XXXXXX")"
signers_file="$(mktemp "${TMPDIR:-/tmp}/henukit-local-builder-signers.XXXXXX")"
cleanup() {
  if [[ -n "$incoming" ]]; then
    rm -rf -- "$incoming"
  fi
  rm -f -- "$signers_file"
}
trap cleanup EXIT

cd "$repo_root"
while IFS=$'\t' read -r name image service role; do
  [[ "$name" =~ ^[a-z0-9][a-z0-9-]*$ && "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] ||
    die "inventory emitted an invalid build record"
  context="$("$inventory" --field "$name" context)"
  dockerfile="$("$inventory" --field "$name" dockerfile)"
  build_args=()
  while IFS= read -r argument || [[ -n "${argument:-}" ]]; do
    [[ -n "$argument" ]] && build_args+=(--build-arg "$argument")
  done < <("$inventory" --field "$name" build_args)
  printf '%s: building %s:%s\n' "$program" "$image" "$release_sha"
  docker build \
    --platform linux/amd64 \
    "${build_args[@]}" \
    --file "$dockerfile" \
    --tag "$image:$release_sha" \
    "$context"
  [[ "$(docker image inspect "$image:$release_sha" --format '{{.Os}}/{{.Architecture}}')" == "linux/amd64" ]] ||
    die "built image is not linux/amd64: $image"
  archive="${image}-${release_sha}.docker.tar.gz"
  docker save "$image:$release_sha" | gzip -1 > "$incoming/$archive"
  [[ -s "$incoming/$archive" ]] || die "image archive is empty: $archive"
  write_checksum "$incoming" "$archive"
done < <("$inventory" --records)

"$runtime_packager" --sha "$release_sha" --output-dir "$incoming" >/dev/null
printf '%s\n' "$release_sha" > "$incoming/RELEASE_SHA"

assert_source_snapshot
[[ "$(remote_main_sha)" == "$release_sha" ]] ||
  die "origin/main changed during build; refusing to sign stale artifacts"
manifest="$incoming/henukit-release-${release_sha}.manifest"
inventory_sha="$(sha256_file "$inventory")"
{
  printf 'format=henukit-local-release-v1\n'
  printf 'release_sha=%s\n' "$release_sha"
  printf 'source_ref=refs/heads/main\n'
  printf 'builder_platform=linux/amd64\n'
  printf 'signer=%s\n' "$signer"
  printf 'signature_namespace=%s\n' "$namespace"
  printf 'inventory_sha256=%s\n' "$inventory_sha"
  while IFS= read -r image; do
    archive="${image}-${release_sha}.docker.tar.gz"
    printf 'artifact_sha256=%s  %s\n' "$(sha256_file "$incoming/$archive")" "$archive"
  done < <("$inventory" --artifact-images)
  archive="henukit-runtime-${release_sha}.tar.gz"
  printf 'artifact_sha256=%s  %s\n' "$(sha256_file "$incoming/$archive")" "$archive"
} > "$manifest"
chmod 0400 "$manifest"
assert_source_snapshot
ssh-keygen -Y sign -f "$signing_key" -n "$namespace" "$manifest" >/dev/null
chmod 0400 "${manifest}.sig"

# The deployment identity receives only a read-only signed bundle. It cannot
# modify artifacts or traverse to the owner-only signing key.
chgrp -R -- "$handoff_group" "$incoming"
find "$incoming" -type d -exec chmod 0550 {} +
find "$incoming" -type f -exec chmod 0440 {} +

printf '%s %s\n' "$signer" "$signing_public_key" > "$signers_file"
chmod 0600 "$signers_file"
"$verifier" \
  --artifact-dir "$incoming" \
  --sha "$release_sha" \
  --inventory "$inventory" \
  --allowed-signers "$signers_file" >/dev/null
assert_source_snapshot
[[ "$(remote_main_sha)" == "$release_sha" ]] ||
  die "origin/main changed during build; refusing to publish stale artifacts"

mv "$incoming" "$final_dir"
incoming=""
printf '%s\n' "$final_dir"
