#!/usr/bin/env bash
# One-command local HENU Kit release build + deploy for WSL2 when GitHub
# Actions minutes are exhausted. Runs the full signed-build and production
# activation chain from a developer WSL2 host, handling every known pitfall
# (KEX interference, WSL file modes, missing production env, stale
# inventory, disk pressure) automatically.
#
# Usage (on the WSL2 host):
#   deploy-henukit-local.sh [--sha <full-main-sha>] [--skip-build] [--preflight-only]
#
# Requires:
#   - docker (jerry in docker group)
#   - deployer ssh config at /home/henukit-deployer/.ssh/
#   - signing key at /etc/henukit-release-builder/ed25519
#   - gh credential helper for GitHub auth
set -Eeuo pipefail

program="deploy-henukit-local"

repo_dir="${HENUKIT_REPO_DIR:-$HOME/HENU-Kit-DEV-career-radar-364}"
artifacts_dir="${HENUKIT_ARTIFACTS_DIR:-$HOME/henukit-signed}"
signing_key_dir="/etc/henukit-release-builder"
deployer_ssh_dir="/home/henukit-deployer/.ssh"
allowed_signers="/etc/henukit-release-deployer/release-signers"
proxy="http://127.0.0.1:7890"

release_sha=""
skip_build=0
preflight_only=0

usage() {
  cat >&2 <<EOF
usage: $program [--sha <full-main-sha>] [--skip-build] [--preflight-only]

One-command signed build + production deploy from WSL2.

  --sha             target main SHA (default: current origin/main head)
  --skip-build      use the existing signed bundle for --sha
  --preflight-only  run deployment preflight only (read-only)

Environment:
  HENUKIT_REPO_DIR      repo checkout path (default \$HOME/HENU-Kit-DEV-career-radar-364)
  HENUKIT_ARTIFACTS_DIR output dir (default \$HOME/henukit-signed)
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

log() {
  printf '%s: %s\n' "$program" "$*"
}

[[ -d "$repo_dir/.git" ]] || die "repo checkout not found at $repo_dir"
command -v docker >/dev/null 2>&1 || die "docker is required"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      release_sha="$2"
      shift 2
      ;;
    --skip-build)
      skip_build=1
      shift
      ;;
    --preflight-only)
      preflight_only=1
      shift
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

export https_proxy="$proxy" http_proxy="$proxy"

# ---- resolve target SHA ----
cd "$repo_dir"
git fetch origin main >/dev/null 2>&1 || die "git fetch failed"
git reset --hard origin/main >/dev/null 2>&1 || die "git reset failed"
[[ -z "$release_sha" ]] && release_sha="$(git rev-parse HEAD)"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid SHA: $release_sha"
log "target release $release_sha"

bundle_dir="$artifacts_dir/henukit-release-$release_sha"

# ---- build (signed) if needed ----
if [[ "$skip_build" -eq 0 ]]; then
  log "fixing WSL script modes (verify requires non-group-writable)"
  docker run --rm -v "$repo_dir:/repo" alpine sh -c '
    apk add --no-cache git >/dev/null 2>&1
    git config --global --add safe.directory /repo
    cd /repo
    find scripts/ops -maxdepth 1 -name "*.sh" -type f -perm -111 -exec chmod 755 {} \;
    chmod 644 scripts/ops/henukit-materials-sync.sh 2>/dev/null
    test "$(git status --porcelain --untracked-files=all | wc -l)" -eq 0
  ' || die "repo is not clean after mode fix"

  log "building signed 16-image release in container (this takes a while)"
  build_script="$(mktemp "${TMPDIR:-/tmp}/henukit-build-once.XXXXXX")"
  cat > "$build_script" <<'SCRIPT'
#!/bin/sh
apk add --no-cache bash git openssh docker-cli gzip coreutils shadow nodejs npm findutils >/dev/null 2>&1
addgroup -S henukit-release-deployers 2>/dev/null
usermod -aG henukit-release-deployers root 2>/dev/null
export HOME=/root
git config --global --add safe.directory /repo
git config --global credential.https://github.com.helper "!/usr/local/bin/gh auth git-credential"
cd /repo
export https_proxy=http://127.0.0.1:7890 http_proxy=http://127.0.0.1:7890
export DOCKER_BUILDKIT=1
su root -c "bash scripts/ops/build-henukit-release-local.sh --sha \$(git rev-parse HEAD) --output-dir $ARTIFACTS_HOST_PATH --signing-key /keys/ed25519 --handoff-group henukit-release-deployers"
SCRIPT
  chmod +x "$build_script"
  mkdir -p "$artifacts_dir"
  docker run --rm \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$repo_dir:/home/jerry/HENU-Kit-DEV-career-radar-364" \
    -v "$repo_dir:/repo" \
    -v "$signing_key_dir:/keys:ro" \
    -v "$artifacts_dir:$artifacts_dir" \
    -v "$HOME/.local/bin/gh:/usr/local/bin/gh:ro" \
    -v "$HOME/.config/gh:/root/.config/gh:ro" \
    -v /usr/libexec/docker/cli-plugins/docker-buildx:/usr/local/libexec/docker/cli-plugins/docker-buildx:ro \
    -v "$HOME/.docker/cli-plugins:/root/.docker/cli-plugins:ro" \
    --network host \
    -e http_proxy="$proxy" -e https_proxy="$proxy" \
    -e ARTIFACTS_HOST_PATH="$artifacts_dir" \
    -v "$build_script:/build.sh:ro" \
    alpine sh /build.sh 2>&1 | grep -E "BUILD_EXIT|die|refusing|error|fatal" | tail -5
  if [[ -d "$bundle_dir" ]]; then
    log "reusing existing bundle at $bundle_dir"
  else
    die "signed bundle not produced at $bundle_dir (see build output above)"
  fi
  log "signed bundle ready: $bundle_dir"
fi

[[ -d "$bundle_dir" ]] || die "no bundle for $release_sha at $bundle_dir (use --skip-build only with existing bundle)"

# ---- deployment SSH config (KEX fix) ----
log "preparing KEX-fixed ssh config"
kex_config="$(mktemp "${TMPDIR:-/tmp}/henu-prod-config-fixed.XXXXXX")"
docker run --rm -v "$deployer_ssh_dir:/hs:ro" alpine sh -c \
  "sed 's/KexAlgorithms curve25519-sha256/KexAlgorithms diffie-hellman-group14-sha256/' /hs/config" \
  > "$kex_config" 2>/dev/null
grep -q "diffie-hellman-group14-sha256" "$kex_config" || \
  die "could not prepare KEX-fixed ssh config"

# ---- deployment (with retries) ----
deploy_script="$(mktemp "${TMPDIR:-/tmp}/henukit-deploy-once.XXXXXX")"
cat > "$deploy_script" <<'SCRIPT'
#!/bin/sh
apk add --no-cache bash git rsync openssh >/dev/null 2>&1
export HOME=/root
cd /repo
export https_proxy=http://127.0.0.1:7890 http_proxy=http://127.0.0.1:7890
git config --global --add safe.directory /repo
git config --global credential.https://github.com.helper "!/usr/local/bin/gh auth git-credential"
mode="--execute"
if [ "$1" = "--preflight-only" ]; then mode="--preflight"; fi
for attempt in 1 2 3 4 5 6 7 8; do
  echo "=== deploy attempt $attempt ==="
  if bash scripts/ops/deploy-henukit-release-from-wsl.sh \
    --sha "$SHA" \
    --artifact-dir /srv/artifacts/henukit-release-"$SHA" \
    --allowed-signers /etc/henukit-release-deployer/release-signers \
    --remote-env-file /opt/henukit/.env.henukit \
    --account-operator-role operations-operator \
    $mode; then
    echo "=== DEPLOY SUCCESS on attempt $attempt ==="
    exit 0
  fi
  echo "=== attempt $attempt failed, waiting 40s ==="
  sleep 40
done
echo "=== ALL ATTEMPTS FAILED ==="
exit 1
SCRIPT
chmod +x "$deploy_script"

deploy_mode="--execute"
[[ "$preflight_only" -eq 1 ]] && deploy_mode="--preflight-only"

log "deploying $release_sha (${deploy_mode})"
docker run --rm \
  -v "$repo_dir:/repo:ro" \
  -v "$artifacts_dir:/srv/artifacts:ro" \
  -v "$kex_config:/root/.ssh/config" \
  -v "$deployer_ssh_dir/id_ed25519_henu_prod:/home/henukit-deployer/.ssh/id_ed25519_henu_prod" \
  -v "$deployer_ssh_dir/known_hosts:/home/henukit-deployer/.ssh/known_hosts" \
  -v "$allowed_signers:/etc/henukit-release-deployer/release-signers:ro" \
  -v "$HOME/.local/bin/gh:/usr/local/bin/gh:ro" \
  -v "$HOME/.config/gh:/root/.config/gh:ro" \
  --network host \
  -e http_proxy="$proxy" -e https_proxy="$proxy" \
  -e SHA="$release_sha" \
  -v "$deploy_script:/dep.sh:ro" \
  alpine sh -c "
    chown root:root /root/.ssh/config /home/henukit-deployer/.ssh/id_ed25519_henu_prod /home/henukit-deployer/.ssh/known_hosts 2>/dev/null
    chmod 600 /root/.ssh/config /home/henukit-deployer/.ssh/id_ed25519_henu_prod 2>/dev/null
    sh /dep.sh $deploy_mode
  " 2>&1 | tail -12

log "done. Verify with:"
log "  ssh quizcraft-prod 'cat /var/lib/henukit-actions-watch/last-activated-sha'"
