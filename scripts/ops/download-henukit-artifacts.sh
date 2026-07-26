#!/usr/bin/env bash
# Download fixed-SHA HENU Kit release artifacts produced by GitHub Actions.
# Production hosts must never compile the monorepo; they only load these archives.
set -Eeuo pipefail

usage() {
  cat >&2 <<'EOF'
usage: download-henukit-artifacts.sh <full-git-sha> <dest-dir> [github-repo]

Downloads every image *.docker.tar.gz (+ .sha256) and henukit-runtime-<sha>.tar.gz
from a successful "Build HENU Kit release artifacts" workflow run for that SHA.

Requires: gh (authenticated), gzip-friendly disk space under <dest-dir>.
EOF
}

die() {
  echo "download-henukit-artifacts: $*" >&2
  exit 1
}

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage
  exit 64
fi

release_sha="$1"
dest_dir="$2"
repo="${3:-}"

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release SHA must be a full 40-char lowercase git SHA"
command -v gh >/dev/null 2>&1 || die "gh CLI is required"

mkdir -p "$dest_dir"
dest_dir="$(cd "$dest_dir" && pwd -P)"

gh_args=(run list --workflow deploy-henukit.yml --json databaseId,headSha,conclusion,status,displayTitle,url --limit 50)
if [[ -n "$repo" ]]; then
  gh_args+=(--repo "$repo")
fi

# Pick the newest successful run whose headSha matches the release SHA.
run_id=""
run_url=""
while IFS= read -r row; do
  [[ -z "$row" ]] && continue
  sha="${row%%$'\t'*}"
  rest="${row#*$'\t'}"
  conclusion="${rest%%$'\t'*}"
  rest="${rest#*$'\t'}"
  status="${rest%%$'\t'*}"
  rest="${rest#*$'\t'}"
  id="${rest%%$'\t'*}"
  url="${rest#*$'\t'}"
  if [[ "$sha" == "$release_sha" && "$conclusion" == "success" && "$status" == "completed" ]]; then
    run_id="$id"
    run_url="$url"
    break
  fi
done < <(
  gh "${gh_args[@]}" --jq '.[] | [.headSha,.conclusion,.status,(.databaseId|tostring),.url] | @tsv'
)

[[ -n "$run_id" ]] || die "no successful Build HENU Kit release artifacts run for $release_sha"

echo "Downloading artifacts for $release_sha from run $run_id ($run_url)"
download_args=(run download "$run_id" --dir "$dest_dir")
if [[ -n "$repo" ]]; then
  download_args+=(--repo "$repo")
fi
gh "${download_args[@]}"

# Flatten nested artifact directories produced by actions/upload-artifact.
# Expected leaves: henukit-*-<sha>.docker.tar.gz(+.sha256), henukit-runtime-<sha>.tar.gz(+.sha256)
find "$dest_dir" -type f \( -name '*.docker.tar.gz' -o -name '*.docker.tar.gz.sha256' -o -name 'henukit-runtime-*.tar.gz' -o -name 'henukit-runtime-*.tar.gz.sha256' \) -print0 |
  while IFS= read -r -d '' file; do
    base="$(basename "$file")"
    target="$dest_dir/$base"
    if [[ "$file" != "$target" ]]; then
      mv -f "$file" "$target"
    fi
  done

# Drop empty leftover directories from gh run download.
find "$dest_dir" -mindepth 1 -type d -empty -delete 2>/dev/null || true

required=(
  "henukit-console-${release_sha}.docker.tar.gz"
  "henukit-console-gateway-${release_sha}.docker.tar.gz"
  "henukit-platform-core-${release_sha}.docker.tar.gz"
  "henukit-platform-mail-worker-${release_sha}.docker.tar.gz"
  "henukit-platform-smtp-provider-${release_sha}.docker.tar.gz"
  "henukit-portal-${release_sha}.docker.tar.gz"
  "henukit-portal-api-${release_sha}.docker.tar.gz"
  "henukit-portal-gateway-${release_sha}.docker.tar.gz"
  "henukit-runtime-${release_sha}.tar.gz"
)

missing=0
for name in "${required[@]}"; do
  if [[ ! -s "$dest_dir/$name" ]]; then
    echo "download-henukit-artifacts: missing $name" >&2
    missing=1
  fi
  if [[ ! -s "$dest_dir/${name}.sha256" ]]; then
    echo "download-henukit-artifacts: missing ${name}.sha256" >&2
    missing=1
  fi
done
[[ "$missing" -eq 0 ]] || die "artifact set for $release_sha is incomplete"

(
  cd "$dest_dir"
  for name in "${required[@]}"; do
    sha256sum -c "${name}.sha256"
  done
)

printf '%s\n' "$release_sha" >"$dest_dir/RELEASE_SHA"
echo "Artifacts verified under $dest_dir"
echo "Next: docker load each *.docker.tar.gz, extract henukit-runtime-${release_sha}.tar.gz, then run deploy-henukit-artifact.sh"
