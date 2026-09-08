#!/usr/bin/env bash
set -Eeuo pipefail

program=create-getwork-actions-manifest
release_sha=""
artifact_dir=""
output=""
incoming=""

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

cleanup() {
  [[ -z "$incoming" ]] || rm -f -- "$incoming"
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha) release_sha="${2:-}"; shift 2 ;;
    --artifact-dir) artifact_dir="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *)
      die "usage: $program --sha <40-hex-main-sha> --artifact-dir <directory> --output <manifest>"
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid release SHA"
[[ -n "$artifact_dir" && -n "$output" ]] ||
  die "--artifact-dir and --output are required"
[[ -d "$artifact_dir" && ! -L "$artifact_dir" ]] || die "invalid artifact directory"
artifact_dir="$(cd "$artifact_dir" && pwd -P)"
[[ "$output" == /* ]] || output="$PWD/$output"
[[ -d "$(dirname "$output")" && ! -L "$(dirname "$output")" ]] ||
  die "output parent must be a real directory"
[[ ! -e "$output" && ! -L "$output" ]] || die "refusing to overwrite output manifest"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"

image="henukit-getwork-mcp-${release_sha}.docker.tar.gz"
runtime="henukit-runtime-${release_sha}.tar.gz"

artifact_digest() {
  local name="$1"
  local digest recorded_digest recorded_name extra
  [[ -f "$artifact_dir/$name" && ! -L "$artifact_dir/$name" ]] ||
    die "missing artifact: $name"
  [[ -f "$artifact_dir/${name}.sha256" && ! -L "$artifact_dir/${name}.sha256" ]] ||
    die "missing checksum: ${name}.sha256"
  [[ "$(wc -l < "$artifact_dir/${name}.sha256" | tr -d '[:space:]')" == 1 ]] ||
    die "checksum must contain exactly one record: $name"
  read -r recorded_digest recorded_name extra < "$artifact_dir/${name}.sha256"
  recorded_name="${recorded_name#\*}"
  digest="$(sha256sum "$artifact_dir/$name" | awk '{print $1}')"
  [[ -z "${extra:-}" && "$recorded_digest" == "$digest" && "$recorded_name" == "$name" ]] ||
    die "checksum does not bind the exact artifact: $name"
  printf '%s' "$digest"
}

image_digest="$(artifact_digest "$image")"
runtime_digest="$(artifact_digest "$runtime")"
incoming="$(mktemp "${output}.incoming.XXXXXX")"
cat > "$incoming" <<EOF
format=henukit-getwork-actions-release-v1
release_sha=${release_sha}
source_repository=jry21223/HENU-Kit-DEV
source_ref=refs/heads/main
signer_workflow=.github/workflows/deploy-henukit.yml
builder_platform=linux/amd64
artifact_sha256=${image_digest}  ${image}
artifact_sha256=${runtime_digest}  ${runtime}
EOF
chmod 0444 "$incoming"
mv "$incoming" "$output"
incoming=""
