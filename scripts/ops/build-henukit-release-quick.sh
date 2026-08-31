#!/usr/bin/env bash
# One-command fixed-SHA HENU Kit artifact build for a developer's own WSL2
# machine when GitHub Actions minutes are exhausted. This is a controlled
# subset of build-henukit-release-local.sh: it keeps the release safety
# baseline (clean checkout, exact SHA, linux/amd64 images, checksums, runtime
# packager) but drops the production signing-key and deployment-handoff-group
# gates, which only matter for an operator-owned formal release.
#
# Output matches the CI artifact layout so the existing production activation
# and watcher flows can consume it unchanged.
#
# WARNING: the bundle produced here is NOT signed and will be rejected by the
# production verifier. Use it only to validate the build pipeline locally.
# For an actual production deployment, use deploy-henukit-local.sh (signed
# container build + KEX-fixed transport), see
# docs/operations/henukit-local-deploy.md.
set -Eeuo pipefail

program="build-henukit-release-quick"

usage() {
  cat >&2 <<'EOF'
usage: build-henukit-release-quick.sh [--sha <full-main-sha>] [--output-dir <dir>]

Builds every canonical HENU Kit image and the fixed-SHA runtime payload into
<output-dir> (default ./artifacts/henukit-release-quick), without production signing or handoff-group
gates.

Safety baseline enforced here:
  - must run on WSL2 Linux x86_64 with a linux/amd64 Docker daemon
  - the checkout must be clean and HEAD must match the requested SHA
  - every built image is verified linux/amd64
  - each archive gets an independent SHA-256 checksum
  - the runtime packager validates the Account production boundary
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

release_sha=""
output_dir=""
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
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
inventory="$repo_root/scripts/ops/henukit-release-images.sh"
runtime_packager="$repo_root/scripts/ops/package-henukit-runtime.sh"
oauth_gate="$repo_root/scripts/ops/oauth-continuation-release-gate.sh"

[[ -x "$inventory" && -x "$runtime_packager" && -x "$oauth_gate" ]] ||
  die "release inventory, runtime packager, and OAuth gate must be executable"
"$inventory" --check

[[ "$(uname -s)" == "Linux" ]] || die "builder must run on Linux; use the WSL2 builder, not macOS Docker"
case "$(uname -m)" in
  x86_64|amd64) ;;
  *) die "builder CPU must be x86_64/amd64" ;;
esac
[[ "$(uname -r)" =~ [Ww][Ss][Ll]2 ]] ||
  die "builder must run on WSL2; a generic Linux host is not an authorized builder"

command -v git >/dev/null 2>&1 || die "git is required"
command -v docker >/dev/null 2>&1 || die "docker is required"
command -v gzip >/dev/null 2>&1 || die "gzip is required"
command -v tar >/dev/null 2>&1 || die "tar is required for the fixed-SHA source snapshot"
docker info >/dev/null 2>&1 || die "Docker daemon is not running"

[[ -z "$release_sha" ]] && release_sha="$(git -C "$repo_root" rev-parse HEAD)"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"

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
[[ "$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}')" == "linux/amd64" ]] ||
  die "Docker server must be linux/amd64"

oauth_gate_dir="$(mktemp -d "${TMPDIR:-/tmp}/henukit-oauth-gate.XXXXXX")"
oauth_gate_receipt="$oauth_gate_dir/oauth-continuation-${release_sha}.env"
source_root=""
cleanup() {
  [[ -z "$source_root" ]] || rm -rf -- "$source_root"
  rm -rf -- "$oauth_gate_dir"
}
trap cleanup EXIT
"$oauth_gate" run --sha "$release_sha" --output "$oauth_gate_receipt"
"$oauth_gate" verify --sha "$release_sha" --receipt "$oauth_gate_receipt"
assert_source_snapshot
source_root="$(mktemp -d "${TMPDIR:-/tmp}/henukit-release-source-${release_sha}.XXXXXX")"
git -C "$repo_root" -c tar.umask=0022 archive --format=tar "$release_sha" |
  tar -xf - -C "$source_root"
build_inventory="$source_root/scripts/ops/henukit-release-images.sh"
[[ -x "$build_inventory" ]] || die "fixed-SHA source snapshot is incomplete"
"$build_inventory" --check

[[ -z "$output_dir" ]] && output_dir="$repo_root/artifacts/henukit-release-quick"
install -d -m 0750 "$output_dir"
output_dir="$(cd "$output_dir" && pwd -P)"

mkdir -p "$output_dir"
printf '%s: building fixed-SHA release %s\n' "$program" "$release_sha"

cd "$source_root"
while IFS=$'\t' read -r name image service role; do
  [[ "$name" =~ ^[a-z0-9][a-z0-9-]*$ && "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] ||
    die "inventory emitted an invalid build record"
  context="$("$build_inventory" --field "$name" context)"
  dockerfile="$("$build_inventory" --field "$name" dockerfile)"
  build_args=()
  while IFS= read -r argument || [[ -n "${argument:-}" ]]; do
    [[ -n "$argument" ]] && build_args+=(--build-arg "$argument")
  done < <("$build_inventory" --field "$name" build_args)
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
  docker save "$image:$release_sha" | gzip -1 > "$output_dir/$archive"
  [[ -s "$output_dir/$archive" ]] || die "image archive is empty: $archive"
  write_checksum "$output_dir" "$archive"
done < <("$build_inventory" --records)

"$runtime_packager" \
  --sha "$release_sha" \
  --output-dir "$output_dir" \
  --oauth-gate-receipt "$oauth_gate_receipt" >/dev/null
printf '%s\n' "$release_sha" > "$output_dir/RELEASE_SHA"
assert_source_snapshot

{
  printf 'format=henukit-release-quick-v1\n'
  printf 'release_sha=%s\n' "$release_sha"
  printf 'builder_platform=linux/amd64\n'
  while IFS= read -r image; do
    archive="${image}-${release_sha}.docker.tar.gz"
    printf 'artifact_sha256=%s  %s\n' "$(sha256_file "$output_dir/$archive")" "$archive"
  done < <("$inventory" --artifact-images)
  archive="henukit-runtime-${release_sha}.tar.gz"
  printf 'artifact_sha256=%s  %s\n' "$(sha256_file "$output_dir/$archive")" "$archive"
} > "$output_dir/henukit-release-${release_sha}.manifest"

printf '%s: done. Artifacts written to %s\n' "$program" "$output_dir"
