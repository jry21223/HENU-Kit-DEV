#!/usr/bin/env bash
# One-shot HENU Kit release: build (signed) and deploy from WSL2 to production
# in a single command, for the identity that owns the henu-prod SSH alias
# (root or the dedicated deployment account). This wraps the two existing
# release entries and keeps every safety gate: clean origin/main checkout,
# exact-SHA signing with a temporary ssh-agent key, artifact verification,
# read-only preflight before execute, and the production activation entry
# with backup/restore, approval, smoke, and rollback.
#
# Use when GitHub Actions minutes are exhausted: the artifact is built on this
# WSL2 host instead of Actions, then deployed through the same signed bundle
# path the production watcher would have used.
set -Eeuo pipefail

program="deploy-henukit-one-shot"

usage() {
  cat >&2 <<'EOF'
usage: deploy-henukit-one-shot.sh \
  [--sha <full-main-sha>] \
  [--output-dir <dir>] \
  [--signing-key <private-key-or-agent-.pub>] \
  [--handoff-group <deployment-reader-group>] \
  [--allowed-signers <trusted-public-signers-file>] \
  [--remote-env-file <absolute-production-env-path>] \
  [--account-operator-role <role-code>] \
  [--platform-migrations <comma-separated-reviewed-files>] \
  [--preflight-only]

Wraps build-henukit-release-local.sh + deploy-henukit-release-from-wsl.sh.

Defaults:
  --sha            current origin/main head (fetched first)
  --output-dir     /srv/henukit-artifacts
  --signing-key    required (private key, or .pub agent handle)
  --handoff-group  henukit-release-deployers
  --allowed-signers /etc/henukit-release-deployer/release-signers
  --remote-env-file /opt/henukit/.env.henukit
  --account-operator-role operations-operator

With --preflight-only, the deploy step runs --preflight (read-only) and
stops. Without it, the deploy step runs --preflight first, and then asks for
confirmation before --execute.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
builder="$repo_root/scripts/ops/build-henukit-release-local.sh"
transporter="$repo_root/scripts/ops/deploy-henukit-release-from-wsl.sh"

release_sha=""
output_dir=""
signing_key=""
handoff_group="henukit-release-deployers"
allowed_signers="/etc/henukit-release-deployer/release-signers"
remote_env_file="/opt/henukit/.env.henukit"
account_operator_role="operations-operator"
platform_migrations=""
mode="full"

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
    --allowed-signers)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      allowed_signers="$2"
      shift 2
      ;;
    --remote-env-file)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      remote_env_file="$2"
      shift 2
      ;;
    --account-operator-role)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      account_operator_role="$2"
      shift 2
      ;;
    --platform-migrations)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      platform_migrations="$2"
      shift 2
      ;;
    --preflight-only)
      mode="preflight"
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

[[ -x "$builder" && -x "$transporter" ]] ||
  die "release builder and transporter must be executable"

[[ -z "$release_sha" ]] && release_sha="$(git -C "$repo_root" ls-remote --exit-code origin refs/heads/main | awk 'NR == 1 { print $1 }')"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] ||
  die "--sha must be a full lowercase Git SHA (got '$release_sha')"
[[ -n "$signing_key" ]] || die "--signing-key is required (private key, or .pub agent handle)"

printf '%s: target release %s\n' "$program" "$release_sha"

build_args=(
  --sha "$release_sha"
  --output-dir "${output_dir:-/srv/henukit-artifacts}"
  --signing-key "$signing_key"
  --handoff-group "$handoff_group"
)
printf '%s: building signed artifact set\n' "$program"
"$builder" "${build_args[@]}"

artifact_dir="${output_dir:-/srv/henukit-artifacts}/henukit-release-$release_sha"
[[ -d "$artifact_dir" ]] || die "builder did not produce $artifact_dir"

deploy_args=(
  --sha "$release_sha"
  --artifact-dir "$artifact_dir"
  --allowed-signers "$allowed_signers"
  --remote-env-file "$remote_env_file"
  --account-operator-role "$account_operator_role"
  --preflight
)
if [[ -n "$platform_migrations" ]]; then
  deploy_args+=(--platform-migrations "$platform_migrations")
fi
printf '%s: running read-only deployment preflight\n' "$program"
"$transporter" "${deploy_args[@]}"

if [[ "$mode" == "preflight" ]]; then
  printf '%s: preflight passed. Deploy with --execute after review.\n' "$program"
  exit 0
fi

printf '%s: preflight passed.\n' "$program"
printf '%s: about to transfer the signed bundle to henu-prod and activate release %s.\n' "$program" "$release_sha"
printf 'Press Enter to execute, Ctrl-C to abort: '
read -r _
deploy_args=(
  --sha "$release_sha"
  --artifact-dir "$artifact_dir"
  --allowed-signers "$allowed_signers"
  --remote-env-file "$remote_env_file"
  --account-operator-role "$account_operator_role"
  --execute
)
if [[ -n "$platform_migrations" ]]; then
  deploy_args+=(--platform-migrations "$platform_migrations")
fi
printf '%s: executing deployment\n' "$program"
"$transporter" "${deploy_args[@]}"

printf '%s: release %s activated.\n' "$program" "$release_sha"
