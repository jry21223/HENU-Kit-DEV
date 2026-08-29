#!/usr/bin/env bash
# Package the fixed-SHA runtime payload shared by GitHub Actions and the
# controlled Linux/amd64 local builder. Application services remain images;
# small host-side materials tools are compiled here, never on production.
set -Eeuo pipefail

program="package-henukit-runtime"

usage() {
  cat >&2 <<'EOF'
usage: package-henukit-runtime.sh --sha <full-git-sha> --output-dir <directory> \
  --oauth-gate-receipt <sha-bound-receipt>

Writes henukit-runtime-<sha>.tar.gz and its SHA-256 checksum into the supplied
output directory.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

sha256_write() {
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
oauth_gate_receipt=""
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
    --oauth-gate-receipt)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      oauth_gate_receipt="$2"
      shift 2
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
[[ -n "$output_dir" && -n "$oauth_gate_receipt" ]] || { usage; exit 64; }
install -d -m 0700 "$output_dir"
output_dir="$(cd "$output_dir" && pwd -P)"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
gate="$repo_root/scripts/ops/oauth-continuation-release-gate.sh"
[[ -x "$gate" ]] || die "OAuth continuation release gate is required"
"$gate" verify --sha "$release_sha" --receipt "$oauth_gate_receipt"
oauth_gate_receipt="$(cd "$(dirname "$oauth_gate_receipt")" && pwd -P)/$(basename "$oauth_gate_receipt")"
command -v docker >/dev/null 2>&1 || die "Docker is required"
command -v node >/dev/null 2>&1 || die "node is required for production-boundary validation"
command -v git >/dev/null 2>&1 || die "git is required for the fixed-SHA source snapshot"
command -v tar >/dev/null 2>&1 || die "tar is required for the fixed-SHA source snapshot"
command -v jq >/dev/null 2>&1 || die "jq is required for the fixed-SHA Compose contract"

archive="henukit-runtime-${release_sha}.tar.gz"
[[ ! -e "$output_dir/$archive" && ! -e "$output_dir/${archive}.sha256" ]] ||
  die "refusing to overwrite an existing runtime artifact for $release_sha"

source_root=""
runtime=""
archive_incoming="$output_dir/.${archive}.incoming.$$"
cleanup() {
  [[ -z "$source_root" ]] || rm -rf -- "$source_root"
  [[ -z "$runtime" ]] || rm -rf -- "$runtime"
  rm -f -- "$archive_incoming"
}
trap cleanup EXIT
source_root="$(mktemp -d "${TMPDIR:-/tmp}/henukit-runtime-source-${release_sha}.XXXXXX")"
git -C "$repo_root" archive --format=tar "$release_sha" | tar -xf - -C "$source_root"
[[ -f "$source_root/docker-compose.henukit.yml" ]] ||
  die "fixed-SHA source snapshot is incomplete"
runtime="$(mktemp -d "${TMPDIR:-/tmp}/henukit-runtime-stage-${release_sha}.XXXXXX")"

install -d \
  "$runtime/bin" \
  "$runtime/infra/nginx" \
  "$runtime/infra/systemd" \
  "$runtime/infra/epay-gateway/patches" \
  "$runtime/migrations/platform-core" \
  "$runtime/migrations/account-portfolio" \
  "$runtime/migrations/notice" \
  "$runtime/migrations/food" \
  "$runtime/migrations/library" \
  "$runtime/migrations/career" \
  "$runtime/migrations/portal" \
  "$runtime/release-gates" \
  "$runtime/getwork-node-deploy" \
  "$runtime/materials-runtime/bin" \
  "$runtime/materials-runtime/libexec" \
  "$runtime/materials-runtime/systemd"
install -m 0444 "$oauth_gate_receipt" \
  "$runtime/release-gates/oauth-continuation.env"

docker compose \
  -f "$source_root/docker-compose.henukit.yml" \
  -f "$source_root/docker-compose.henukit.prebuilt.yml" \
  config --format json --no-interpolate --no-path-resolution \
  | jq 'del(.services[] | select(((.profiles // []) | length) > 0))' \
  | docker compose -f - config --no-interpolate --no-path-resolution \
  > "$runtime/docker-compose.henukit.release.yml"

cp "$source_root/infra/nginx/henukit.conf.example" "$runtime/infra/nginx/"
cp "$source_root/infra/systemd/henukit-actions-watch.service" "$runtime/infra/systemd/"
cp "$source_root"/infra/epay-gateway/patches/*.patch "$runtime/infra/epay-gateway/patches/"
cp "$source_root"/services/platform-core/db/migrations/*.up.sql "$runtime/migrations/platform-core/"
cp "$source_root"/services/account-portfolio/db/migrations/*.up.sql "$runtime/migrations/account-portfolio/"
cp "$source_root"/services/notice/db/migrations/*.up.sql "$runtime/migrations/notice/"
cp "$source_root"/services/food/db/migrations/*.up.sql "$runtime/migrations/food/"
cp "$source_root"/services/library/db/migrations/*.up.sql "$runtime/migrations/library/"
cp -a "$source_root"/services/getwork-mcp/deploy/. "$runtime/getwork-node-deploy/"
cp "$source_root"/services/career-opportunities/db/migrations/*.up.sql "$runtime/migrations/career/"
# Portal API keeps a MySQL variant beside PostgreSQL; production only runs the
# PostgreSQL migration stream.
cp "$source_root"/services/portal-api/db/migrations/postgres/*.up.sql "$runtime/migrations/portal/"

docker run --rm --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  --env CGO_ENABLED=0 --env GOOS=linux --env GOARCH=amd64 \
  --env GOCACHE=/tmp/go-cache --env GOMODCACHE=/tmp/go-mod \
  --volume "$source_root:/src:ro" \
  --volume "$runtime/materials-runtime/bin:/out" \
  --volume "$runtime/bin:/host-out" \
  --workdir /src \
  golang:1.26.6-alpine \
  sh -ceu '
    cd /src/services/deploy-webhook
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/henukit-deploy-webhook ./cmd/server
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/materials-oss-canary ./cmd/materials-oss-canary
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/materials-oss-release ./cmd/materials-oss-release
    cd /src/services/library
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/library-activate-public-release ./cmd/activate-public-release
    cd /src/services/food
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /host-out/food-sanitize-post-image ./cmd/sanitize-post-image
  '
chmod 0555 "$runtime/materials-runtime/bin"/*
chmod 0555 "$runtime/bin/food-sanitize-post-image"

for helper in \
  henukit-materials-orchestrate \
  henukit-materials-prepare \
  henukit-materials-seal \
  henukit-materials-activate \
  henukit-materials-publish-oss \
  henukit-materials-publish-release-oss; do
  install -m 0555 "$source_root/services/deploy-webhook/deploy/$helper" \
    "$runtime/materials-runtime/libexec/$helper"
done
for helper in \
  prepare-henukit-materials.mjs \
  seal-henukit-materials.mjs \
  activate-henukit-materials.mjs \
  build-henukit-library-activation-bundle.mjs; do
  install -m 0444 "$source_root/scripts/ops/$helper" "$runtime/materials-runtime/libexec/$helper"
done
install -m 0555 "$source_root/services/deploy-webhook/deploy/install-materials-runtime.sh" \
  "$runtime/materials-runtime/install.sh"
install -m 0444 "$source_root"/services/deploy-webhook/deploy/systemd/henukit-materials-* \
  "$runtime/materials-runtime/systemd/"
(
  cd "$runtime/materials-runtime"
  while IFS= read -r -d '' path; do
    sha256sum "$path"
  done < <(
    { find bin libexec systemd -type f -print0; printf 'install.sh\0'; } |
      LC_ALL=C sort -z
  ) > SHA256SUMS
  [[ -s SHA256SUMS ]] || die "materials runtime checksum manifest is empty"
  chmod 0444 SHA256SUMS
)

for helper in \
  deploy-henukit-artifact.sh \
  watch-henukit-actions.sh \
  activate-henukit-release.sh \
  adopt-henukit-degraded-baseline.sh \
  rotate-henukit-release-signers.sh \
  deploy-epay-gateway-patches.sh \
  henukit-release-images.sh \
  verify-henukit-local-release.sh; do
  install -m 0555 "$source_root/scripts/ops/$helper" "$runtime/bin/$helper"
done
install -m 0555 "$source_root/scripts/ops/import-legacy-portal-food-images.mjs" \
  "$runtime/bin/import-legacy-portal-food-images.mjs"

RELEASE_SHA="$release_sha" node "$source_root/scripts/ops/check-account-production-boundary.mjs" \
  --report "$runtime/release-gates/account-production-boundary.env"
printf '%s\n' "$release_sha" > "$runtime/RELEASE_SHA"
"$gate" verify --sha "$release_sha" --receipt "$oauth_gate_receipt"
tar -C "$runtime" -czf "$archive_incoming" .
[[ -s "$archive_incoming" ]] || die "runtime archive is empty"
mv "$archive_incoming" "$output_dir/$archive"
sha256_write "$output_dir" "$archive"

printf '%s\n' "$output_dir/$archive"
