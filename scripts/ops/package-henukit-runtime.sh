#!/usr/bin/env bash
# Package the fixed-SHA runtime payload shared by GitHub Actions and the
# controlled Linux/amd64 local builder. Application services remain images;
# small host-side materials tools are compiled here, never on production.
set -Eeuo pipefail

program="package-henukit-runtime"

usage() {
  cat >&2 <<'EOF'
usage: package-henukit-runtime.sh --sha <full-git-sha> --output-dir <directory>

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
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
[[ -n "$output_dir" ]] || { usage; exit 64; }
install -d -m 0700 "$output_dir"
output_dir="$(cd "$output_dir" && pwd -P)"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
command -v docker >/dev/null 2>&1 || die "Docker is required"
command -v node >/dev/null 2>&1 || die "node is required for production-boundary validation"

archive="henukit-runtime-${release_sha}.tar.gz"
[[ ! -e "$output_dir/$archive" && ! -e "$output_dir/${archive}.sha256" ]] ||
  die "refusing to overwrite an existing runtime artifact for $release_sha"

runtime="$(mktemp -d "$output_dir/.henukit-runtime-${release_sha}.XXXXXX")"
archive_incoming="$output_dir/.${archive}.incoming.$$"
cleanup() {
  rm -rf -- "$runtime"
  rm -f -- "$archive_incoming"
}
trap cleanup EXIT

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
  "$runtime/materials-runtime/bin" \
  "$runtime/materials-runtime/libexec" \
  "$runtime/materials-runtime/systemd"

docker compose \
  -f "$repo_root/docker-compose.henukit.yml" \
  -f "$repo_root/docker-compose.henukit.prebuilt.yml" \
  config --no-interpolate --no-path-resolution > "$runtime/docker-compose.henukit.release.yml"

cp "$repo_root/infra/nginx/henukit.conf.example" "$runtime/infra/nginx/"
cp "$repo_root/infra/systemd/henukit-actions-watch.service" "$runtime/infra/systemd/"
cp "$repo_root"/infra/epay-gateway/patches/*.patch "$runtime/infra/epay-gateway/patches/"
cp "$repo_root"/services/platform-core/db/migrations/*.up.sql "$runtime/migrations/platform-core/"
cp "$repo_root"/services/account-portfolio/db/migrations/*.up.sql "$runtime/migrations/account-portfolio/"
cp "$repo_root"/services/notice/db/migrations/*.up.sql "$runtime/migrations/notice/"
cp "$repo_root"/services/food/db/migrations/*.up.sql "$runtime/migrations/food/"
cp "$repo_root"/services/library/db/migrations/*.up.sql "$runtime/migrations/library/"
cp "$repo_root"/services/career-opportunities/db/migrations/*.up.sql "$runtime/migrations/career/"
# Portal API keeps a MySQL variant beside PostgreSQL; production only runs the
# PostgreSQL migration stream.
cp "$repo_root"/services/portal-api/db/migrations/postgres/*.up.sql "$runtime/migrations/portal/"

docker run --rm --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  --env CGO_ENABLED=0 --env GOOS=linux --env GOARCH=amd64 \
  --env GOCACHE=/tmp/go-cache --env GOMODCACHE=/tmp/go-mod \
  --volume "$repo_root:/src:ro" \
  --volume "$runtime/materials-runtime/bin:/out" \
  --volume "$runtime/bin:/host-out" \
  --workdir /src \
  golang:1.26.6-alpine \
  sh -ceu '
    cd /src/services/deploy-webhook
    go build -trimpath -ldflags="-s -w" -o /out/henukit-deploy-webhook ./cmd/server
    go build -trimpath -ldflags="-s -w" -o /out/materials-oss-canary ./cmd/materials-oss-canary
    go build -trimpath -ldflags="-s -w" -o /out/materials-oss-release ./cmd/materials-oss-release
    cd /src/services/library
    go build -trimpath -ldflags="-s -w" -o /out/library-activate-public-release ./cmd/activate-public-release
    cd /src/services/food
    go build -trimpath -ldflags="-s -w" -o /host-out/food-sanitize-post-image ./cmd/sanitize-post-image
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
  install -m 0555 "$repo_root/services/deploy-webhook/deploy/$helper" \
    "$runtime/materials-runtime/libexec/$helper"
done
for helper in \
  prepare-henukit-materials.mjs \
  seal-henukit-materials.mjs \
  activate-henukit-materials.mjs \
  build-henukit-library-activation-bundle.mjs; do
  install -m 0444 "$repo_root/scripts/ops/$helper" "$runtime/materials-runtime/libexec/$helper"
done
install -m 0555 "$repo_root/services/deploy-webhook/deploy/install-materials-runtime.sh" \
  "$runtime/materials-runtime/install.sh"
install -m 0444 "$repo_root"/services/deploy-webhook/deploy/systemd/henukit-materials-* \
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
  install -m 0555 "$repo_root/scripts/ops/$helper" "$runtime/bin/$helper"
done
install -m 0555 "$repo_root/scripts/ops/import-legacy-portal-food-images.mjs" \
  "$runtime/bin/import-legacy-portal-food-images.mjs"

RELEASE_SHA="$release_sha" node "$repo_root/scripts/ops/check-account-production-boundary.mjs" \
  --report "$runtime/release-gates/account-production-boundary.env"
printf '%s\n' "$release_sha" > "$runtime/RELEASE_SHA"
tar -C "$runtime" -czf "$archive_incoming" .
[[ -s "$archive_incoming" ]] || die "runtime archive is empty"
mv "$archive_incoming" "$output_dir/$archive"
sha256_write "$output_dir" "$archive"

printf '%s\n' "$output_dir/$archive"
