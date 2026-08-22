#!/usr/bin/env bash
# Watch successful main-branch GitHub Actions builds and deploy their fixed-SHA
# artifacts directly on the production host. This script never compiles source.
set -Eeuo pipefail

program="watch-henukit-actions"
mode=""

usage() {
  cat >&2 <<'EOF'
usage: watch-henukit-actions.sh --once|--watch
       watch-henukit-actions.sh --local-artifacts <artifact-dir> --sha <full-main-sha>
       watch-henukit-actions.sh --local-artifacts <artifact-dir> --sha <full-main-sha> \
         --recover-degraded-baseline <full-current-sha>

Required configuration:
  HENUKIT_ENV_FILE       Existing production Compose environment file
  GH_TOKEN_FILE          Root-only GitHub token file (Actions: Read, Contents: Read)

Optional configuration:
  HENUKIT_REPO           GitHub repository (default: jry21223/HENU-Kit-DEV)
  HENUKIT_WORKFLOW       Workflow file (default: deploy-henukit.yml)
  HENUKIT_BRANCH         Watched branch (default: main)
  HENUKIT_STAGING_ROOT   Verified artifact cache (default: /opt/henukit-staging)
  HENUKIT_RELEASE_ROOT   Extracted releases (default: /opt/henukit-releases)
  HENUKIT_BACKUP_ROOT    Platform and Account Portfolio backups (default: /opt/henukit-backups)
  HENUKIT_STATE_ROOT     Watcher state and lock (default: /var/lib/henukit-actions-watch)
  HENUKIT_POLL_SECONDS   Watch interval (default: 60)
  HENUKIT_PUBLIC_BASE_URL Public smoke-test base URL
  HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE Active role receiving Account Console permissions
  HENUKIT_PLATFORM_MIGRATIONS Comma-separated reviewed Platform Core migrations
  HENUKIT_PLATFORM_CORE_CONTAINER Platform Core container name
  HENUKIT_IMAGE_INVENTORY Trusted canonical image inventory beside this script
  HENUKIT_LOCAL_ARTIFACT_VERIFIER Trusted signed-artifact verifier beside this script
  HENUKIT_RELEASE_SIGNERS_FILE Root-managed allowed-signers file for local artifacts
EOF
}

log() {
  printf '%s: %s\n' "$program" "$*"
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

local_artifact_dir=""
local_release_sha=""
recovery_baseline_sha=""
if [[ $# -eq 1 && ( "$1" == "--once" || "$1" == "--watch" ) ]]; then
  mode="$1"
elif [[ $# -eq 4 && "$1" == "--local-artifacts" && "$3" == "--sha" ]]; then
  mode="--local-artifacts"
  local_artifact_dir="$2"
  local_release_sha="$4"
elif [[ $# -eq 6 && "$1" == "--local-artifacts" && "$3" == "--sha" &&
        "$5" == "--recover-degraded-baseline" ]]; then
  mode="--local-artifacts"
  local_artifact_dir="$2"
  local_release_sha="$4"
  recovery_baseline_sha="$6"
else
  usage
  exit 64
fi

repo="${HENUKIT_REPO:-jry21223/HENU-Kit-DEV}"
workflow="${HENUKIT_WORKFLOW:-deploy-henukit.yml}"
branch="${HENUKIT_BRANCH:-main}"
env_file="${HENUKIT_ENV_FILE:-}"
rollback_env_file="${HENUKIT_ROLLBACK_ENV_FILE:-$env_file}"
token_file="${GH_TOKEN_FILE:-/etc/henukit/github-actions-read.token}"
staging_root="${HENUKIT_STAGING_ROOT:-/opt/henukit-staging}"
release_root="${HENUKIT_RELEASE_ROOT:-/opt/henukit-releases}"
backup_root="${HENUKIT_BACKUP_ROOT:-/opt/henukit-backups}"
state_root="${HENUKIT_STATE_ROOT:-/var/lib/henukit-actions-watch}"
poll_seconds="${HENUKIT_POLL_SECONDS:-60}"
public_base_url="${HENUKIT_PUBLIC_BASE_URL:-https://superhuazai.me}"
account_public_origin="https://henukit.cn"
postgres_container="${HENUKIT_POSTGRES_CONTAINER:-henukit-postgres-1}"
account_portfolio_container="${HENUKIT_ACCOUNT_PORTFOLIO_CONTAINER:-henukit-account-portfolio-1}"
platform_core_container="${HENUKIT_PLATFORM_CORE_CONTAINER:-henukit-platform-core-1}"
notice_container="${HENUKIT_NOTICE_CONTAINER:-henukit-notice-1}"
food_container="${HENUKIT_FOOD_CONTAINER:-henukit-food-1}"
library_container="${HENUKIT_LIBRARY_CONTAINER:-henukit-library-1}"
migration="${HENUKIT_PLATFORM_MIGRATIONS:-${HENUKIT_PLATFORM_MIGRATION:-}}"
account_operator_role="${HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE:-}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
image_inventory="${HENUKIT_IMAGE_INVENTORY:-$script_dir/henukit-release-images.sh}"
local_artifact_verifier="${HENUKIT_LOCAL_ARTIFACT_VERIFIER:-$script_dir/verify-henukit-local-release.sh}"
release_signers_file="${HENUKIT_RELEASE_SIGNERS_FILE:-/etc/henukit/release-signers}"
current_release_link="${HENUKIT_CURRENT_LINK:-/opt/henukit-current}"

images=()
load_images=()
base_images=()
conditional_services=()
conditional_images=()

[[ -n "$env_file" && -r "$env_file" ]] || die "HENUKIT_ENV_FILE must point to a readable production environment file"
[[ -n "$rollback_env_file" && -r "$rollback_env_file" && ! -L "$rollback_env_file" ]] ||
  die "HENUKIT_ROLLBACK_ENV_FILE must point to a readable, non-symlink environment file"
[[ -r "$token_file" && -f "$token_file" ]] || die "GH_TOKEN_FILE must point to a readable regular file"
[[ "$poll_seconds" =~ ^[1-9][0-9]*$ ]] || die "HENUKIT_POLL_SECONDS must be a positive integer"
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_REPO must be an owner/name pair"
[[ "$branch" =~ ^[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_BRANCH contains unsupported characters"
[[ "$account_operator_role" =~ ^[a-z0-9][a-z0-9-]{0,63}$ ]] ||
  die "HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE must name an explicit role using lowercase letters, digits, or hyphens"
command -v gh >/dev/null 2>&1 || die "gh CLI is required"
command -v docker >/dev/null 2>&1 || die "docker is required"
command -v flock >/dev/null 2>&1 || die "flock is required"
command -v jq >/dev/null 2>&1 || die "jq is required"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"
command -v tar >/dev/null 2>&1 || die "tar is required"

file_mode() {
  stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
}

file_owner() {
  stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"
}

trusted_root_parent_chain() {
  local path="$1"
  local label="$2"
  local directory mode owner
  directory="$(dirname "$path")"
  while [[ "$directory" != "/" ]]; do
    [[ -d "$directory" && ! -L "$directory" ]] ||
      die "$label parent must be a real directory: $directory"
    mode="$(file_mode "$directory")"
    owner="$(file_owner "$directory")"
    [[ "$mode" =~ ^[0-7]{3,4}$ ]] || die "could not determine $label parent mode: $directory"
    [[ "$owner" == "0" ]] || die "$label parent must be owned by root: $directory"
    (( (8#$mode & 8#022) == 0 )) ||
      die "$label parent must not be group- or world-writable: $directory"
    directory="$(dirname "$directory")"
  done
}

trusted_root_file() {
  local file="$1"
  local label="$2"
  local mode owner
  [[ "$file" == /* ]] || die "$label must use an absolute path: $file"
  [[ -f "$file" && ! -L "$file" ]] ||
    die "$label must be a regular non-symlink file: $file"
  mode="$(file_mode "$file")"
  owner="$(file_owner "$file")"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || die "could not determine $label mode: $file"
  [[ "$owner" == "0" ]] || die "$label must be owned by root: $file"
  (( (8#$mode & 8#022) == 0 )) || die "$label must not be group- or world-writable: $file"

  trusted_root_parent_chain "$file" "$label"
}

trusted_helper() {
  local file="$1"
  trusted_root_file "$file" "trusted helper"
  [[ -x "$file" ]] ||
    die "trusted helper must be executable: $file"
}

trusted_helper "$image_inventory"
"$image_inventory" --check
while IFS= read -r image; do
  [[ "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] || die "image inventory emitted an invalid artifact image"
  images+=("$image")
done < <("$image_inventory" --artifact-images)
while IFS= read -r image; do
  [[ "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] || die "image inventory emitted an invalid load image"
  load_images+=("$image")
done < <("$image_inventory" --load-images)
while IFS= read -r image; do
  [[ "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] || die "image inventory emitted an invalid baseline image"
  base_images+=("$image")
done < <("$image_inventory" --baseline-images)
while IFS=$'\t' read -r service image; do
  [[ "$service" =~ ^[a-z0-9][a-z0-9-]*$ && "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] ||
    die "image inventory emitted an invalid conditional service record"
  conditional_services+=("$service")
  conditional_images+=("$image")
done < <("$image_inventory" --conditional-services)
[[ "${#images[@]}" -gt 0 && "${#load_images[@]}" -gt 0 && "${#base_images[@]}" -gt 0 ]] ||
  die "image inventory is incomplete"
if [[ "$mode" == "--local-artifacts" ]]; then
  [[ "$local_release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
  if [[ -n "$recovery_baseline_sha" ]]; then
    [[ "$recovery_baseline_sha" =~ ^[0-9a-f]{40}$ ]] ||
      die "--recover-degraded-baseline must be a full lowercase Git SHA"
    [[ "$recovery_baseline_sha" != "$local_release_sha" ]] ||
      die "recovery baseline and candidate SHA must differ"
  fi
  [[ "$branch" == "main" ]] || die "local artifacts may only be activated from main"
  [[ -d "$local_artifact_dir" && ! -L "$local_artifact_dir" ]] ||
    die "--local-artifacts must name a non-symlink directory"
  trusted_helper "$local_artifact_verifier"
  trusted_root_file "$release_signers_file" "HENUKIT_RELEASE_SIGNERS_FILE"
  [[ -r "$release_signers_file" ]] || die "HENUKIT_RELEASE_SIGNERS_FILE must be readable"
fi

environment_value() {
  local key="$1"
  local count value
  count="$(grep -Ec "^[[:space:]]*${key}[[:space:]]*=" "$env_file" || true)"
  [[ "$count" -le 1 ]] || die "$key is assigned more than once in HENUKIT_ENV_FILE"
  if [[ "$count" -eq 0 ]]; then
    return 0
  fi
  value="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$env_file")"
  value="${value#*=}"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  if [[ "$value" == \"*\" && "$value" == *\" ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "$value"
}

verify_production_data_boundary() {
  local portal_api_mode portal_allow_mock easypay_enabled easypay_pid easypay_key
  local easypay_base_url easypay_notify_url easypay_return_url
  local career_sources career_ai_base career_ai_key career_ai_model career_ai_insecure career_digest_secret
  local career_digest_client career_digest_key normalized_secret normalized_ai
  portal_api_mode="$(environment_value PORTAL_API_MODE)"
  portal_allow_mock="$(environment_value NEXT_PUBLIC_PORTAL_ALLOW_MOCK)"
  [[ "$portal_api_mode" == "live" ]] ||
    die "PORTAL_API_MODE must be explicitly live before production deployment"
  [[ -z "$portal_allow_mock" || "$portal_allow_mock" == "0" ]] ||
    die "NEXT_PUBLIC_PORTAL_ALLOW_MOCK must be 0 or absent before production deployment"
  easypay_enabled="$(environment_value ACCOUNT_PORTFOLIO_EASYPAY_ENABLED)"
  easypay_pid="$(environment_value ACCOUNT_PORTFOLIO_EASYPAY_PID)"
  easypay_key="$(environment_value ACCOUNT_PORTFOLIO_EASYPAY_KEY)"
  easypay_base_url="$(environment_value ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL)"
  easypay_notify_url="$(environment_value ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL)"
  easypay_return_url="$(environment_value ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL)"
  [[ "$easypay_enabled" == "1" ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED must be 1 for the Account payment release"
  [[ "$easypay_pid" =~ ^[A-Za-z0-9_-]{1,64}$ ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_PID is missing or invalid"
  [[ ${#easypay_key} -ge 16 && "$easypay_key" != *[[:space:]]* ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_KEY is missing or invalid"
  [[ ! "$easypay_key" =~ (replace|example|changeme|test-secret) ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_KEY is a deployment placeholder"
  [[ "$easypay_base_url" == "https://metaview.top/epay" ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL must use the production MetaView EasyPay gateway"
  [[ "$easypay_notify_url" == "https://henukit.cn/api/v1/payment-providers/easypay/notifications" ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL must use the exact public callback ingress"
  [[ "$easypay_return_url" == "https://henukit.cn/account/membership" ]] || die "ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL must use the public membership route"
  career_sources="$(environment_value CAREER_SOURCE_ALLOWLIST)"
  career_ai_base="$(environment_value CAREER_AI_BASE_URL)"
  career_ai_key="$(environment_value CAREER_AI_API_KEY)"
  career_ai_model="$(environment_value CAREER_AI_MODEL)"
  career_ai_insecure="$(environment_value CAREER_ALLOW_INSECURE_AI_HTTP)"
  career_digest_client="$(environment_value PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID)"
  career_digest_key="$(environment_value PLATFORM_CORE_CAREER_DIGEST_KEY_ID)"
  career_digest_secret="$(environment_value PLATFORM_CORE_CAREER_DIGEST_SECRET)"
  [[ "$career_sources" == "official.meituan" ]] || die "CAREER_SOURCE_ALLOWLIST must enable only official.meituan"
  if [[ "$career_ai_base" == "http://125.46.96.207:30000/v1" ]]; then
    [[ "$career_ai_insecure" == "1" ]] || die "the approved plaintext Career LLM requires CAREER_ALLOW_INSECURE_AI_HTTP=1"
  else
    [[ "$career_ai_base" == https://* || "$career_ai_base" == http://127.0.0.1* || "$career_ai_base" == http://localhost* ]] ||
      die "CAREER_AI_BASE_URL must use HTTPS, loopback, or the exact approved HTTP endpoint"
    [[ -z "$career_ai_insecure" || "$career_ai_insecure" == "0" ]] ||
      die "CAREER_ALLOW_INSECURE_AI_HTTP=1 is valid only for the exact approved HTTP endpoint"
  fi
  [[ ${#career_ai_key} -ge 16 && "$career_ai_key" != *[[:space:]]* && -n "$career_ai_model" ]] ||
    die "Career extraction LLM credentials are missing or invalid"
  normalized_ai="$(printf '%s\n%s\n%s' "$career_ai_base" "$career_ai_key" "$career_ai_model" | tr '[:upper:]' '[:lower:]')"
  [[ ! "$normalized_ai" =~ (replace|example|change-me|changeme|test-secret|test-only) ]] ||
    die "Career extraction LLM configuration contains a deployment placeholder"
  [[ -n "$career_digest_client" && -n "$career_digest_key" && ${#career_digest_secret} -ge 32 && "$career_digest_secret" != *[[:space:]]* ]] ||
    die "Career digest credentials are missing or invalid"
  normalized_secret="$(printf '%s' "$career_digest_secret" | tr '[:upper:]' '[:lower:]')"
  [[ "$normalized_secret" != "local-career-digest-secret-32bytes-only!" ]] ||
    die "Career digest secret contains a deployment placeholder"
  [[ ! "$normalized_secret" =~ (replace|example|change-me|changeme|test-secret|for-test|test-only) ]] ||
    die "Career digest secret contains a deployment placeholder"
}

verify_account_boundary_manifest() {
  local release_dir="$1"
  local release_sha="$2"
  local manifest="$release_dir/release-gates/account-production-boundary.env"
  local expected actual
  [[ -f "$manifest" && -r "$manifest" && ! -L "$manifest" ]] ||
    die "Account production-boundary manifest is missing"
  expected="$(printf '%s\n' \
    "release_sha=$release_sha" \
    "status=pass" \
    "account_console_mock_sources=absent" \
    "account_transitive_mock_sources=absent" \
    "account_payment_provider=easypay_or_disabled" \
    "portal_require_gateway=1" \
    "portal_allow_mock=0" \
    "portal_api_default_mode=live")"
  actual="$(tr -d '\r' < "$manifest")"
  [[ "$actual" == "$expected" ]] ||
    die "Account production-boundary manifest did not pass for release $release_sha"
}

verify_production_data_boundary

token_mode="$(file_mode "$token_file")"
token_owner="$(file_owner "$token_file")"
[[ "$token_mode" == "600" || "$token_mode" == "400" ]] || die "GitHub token file mode must be 0600 or 0400"
[[ "$token_owner" == "$(id -u)" ]] || die "GitHub token file must be owned by the watcher user"
GH_TOKEN="$(tr -d '\r\n' < "$token_file")"
[[ -n "$GH_TOKEN" ]] || die "GitHub token file is empty"
export GH_TOKEN

install -d -m 0700 \
  "$staging_root" "$release_root" "$backup_root" "$state_root" \
  "$state_root/approvals" "$state_root/approvals/consumed" "$state_root/prepared" \
  "$state_root/degraded-recoveries"
scratch_dirs=()
restore_database=""
restore_account_database=""
watcher_instance_file="$state_root/watcher.instance"
watcher_instance_owned=0
exec 9>"$state_root/watcher.lock"
flock -n 9 || die "another watcher process holds $state_root/watcher.lock"
cleanup() {
  local scratch
  if [[ "$restore_database" =~ ^henukit_verify_[0-9a-f]{8}_[0-9]+$ ]]; then
    docker exec "$postgres_container" sh -ceu \
      'dropdb --if-exists -U "$POSTGRES_USER" "$1"' sh "$restore_database" \
      >/dev/null 2>&1 || true
  fi
  if [[ "$restore_account_database" =~ ^henukit_account_verify_[0-9a-f]{8}_[0-9]+$ ]]; then
    docker exec "$postgres_container" sh -ceu \
      'dropdb --if-exists -U "$POSTGRES_USER" "$1"' sh "$restore_account_database" \
      >/dev/null 2>&1 || true
  fi
  for scratch in "${scratch_dirs[@]-}"; do
    if [[ "$scratch" == "$staging_root/."*.incoming.* ||
          "$scratch" == "$release_root/."*.incoming.* ]]; then
      rm -rf -- "$scratch"
    fi
  done
  if [[ "$watcher_instance_owned" == "1" && -f "$watcher_instance_file" &&
        "$(tr -d '\r\n' < "$watcher_instance_file")" == "$$" ]]; then
    rm -f -- "$watcher_instance_file"
  fi
}
trap cleanup EXIT

expected_name() {
  local candidate="$1"
  local release_sha="$2"
  local signature_policy="${3:-unsigned}"
  local image
  for image in "${images[@]}"; do
    if [[ "$candidate" == "${image}-${release_sha}.docker.tar.gz" ||
          "$candidate" == "${image}-${release_sha}.docker.tar.gz.sha256" ]]; then
      return 0
    fi
  done
  if [[ "$candidate" == "henukit-runtime-${release_sha}.tar.gz" ||
        "$candidate" == "henukit-runtime-${release_sha}.tar.gz.sha256" ]]; then
    return 0
  fi
  [[ "$signature_policy" == "signed" &&
     ( "$candidate" == "henukit-release-${release_sha}.manifest" ||
       "$candidate" == "henukit-release-${release_sha}.manifest.sig" ) ]]
}

verify_artifact_dir() {
  local artifact_dir="$1"
  local release_sha="$2"
  local signature_policy="${3:-unsigned}"
  local image name file base directory manifest

  [[ "$signature_policy" == "unsigned" || "$signature_policy" == "signed" ]] ||
    die "unknown artifact signature policy"

  [[ -s "$artifact_dir/RELEASE_SHA" ]] || die "artifact set has no RELEASE_SHA marker"
  while IFS= read -r -d '' file; do
    die "artifact set contains symbolic link $(basename "$file")"
  done < <(find "$artifact_dir" -type l -print0)
  while IFS= read -r -d '' directory; do
    die "artifact set contains unexpected directory $(basename "$directory")"
  done < <(find "$artifact_dir" -mindepth 1 -type d -print0)
  while IFS= read -r -d '' file; do
    base="$(basename "$file")"
    if [[ "$base" == "RELEASE_SHA" ]]; then
      [[ "$(tr -d '[:space:]' < "$file")" == "$release_sha" ]] ||
        die "artifact cache RELEASE_SHA does not match"
    else
      expected_name "$base" "$release_sha" "$signature_policy" || die "unexpected artifact file $base"
    fi
  done < <(find "$artifact_dir" -type f -print0)

  for image in "${images[@]}"; do
    name="${image}-${release_sha}.docker.tar.gz"
    [[ -s "$artifact_dir/$name" ]] || die "artifact set is missing $name"
    [[ -s "$artifact_dir/$name.sha256" ]] || die "artifact set is missing $name.sha256"
  done
  name="henukit-runtime-${release_sha}.tar.gz"
  [[ -s "$artifact_dir/$name" ]] || die "artifact set is missing $name"
  [[ -s "$artifact_dir/$name.sha256" ]] || die "artifact set is missing $name.sha256"
  if [[ "$signature_policy" == "signed" ]]; then
    manifest="henukit-release-${release_sha}.manifest"
    [[ -s "$artifact_dir/$manifest" && -s "$artifact_dir/${manifest}.sig" ]] ||
      die "signed artifact set is missing its manifest or signature"
  fi

  (
    cd "$artifact_dir"
    for image in "${images[@]}"; do
      sha256sum -c "${image}-${release_sha}.docker.tar.gz.sha256" || exit 1
    done
    sha256sum -c "henukit-runtime-${release_sha}.tar.gz.sha256" || exit 1
  ) >&2 || die "artifact checksum verification failed"
}

download_artifacts() {
  local run_id="$1"
  local release_sha="$2"
  local final_dir="$staging_root/$release_sha"
  local incoming file base target

  if [[ -d "$final_dir" ]]; then
    verify_artifact_dir "$final_dir" "$release_sha"
    downloaded_artifact_dir="$final_dir"
    return
  fi

  incoming="$(mktemp -d "$staging_root/.${release_sha}.incoming.XXXXXX")"
  scratch_dirs+=("$incoming")
  gh run download "$run_id" --repo "$repo" --dir "$incoming" >&2

  while IFS= read -r -d '' file; do
    base="$(basename "$file")"
    expected_name "$base" "$release_sha" || die "unexpected artifact file $base"
    target="$incoming/$base"
    if [[ "$file" != "$target" ]]; then
      [[ ! -e "$target" ]] || die "duplicate artifact file $base"
      mv "$file" "$target"
    fi
  done < <(find "$incoming" -type f -print0)
  find "$incoming" -mindepth 1 -type d -empty -delete 2>/dev/null || true

  printf '%s\n' "$release_sha" > "$incoming/RELEASE_SHA"
  verify_artifact_dir "$incoming" "$release_sha"
  mv "$incoming" "$final_dir"
  downloaded_artifact_dir="$final_dir"
}

stage_local_artifacts() {
  local source_dir="$1"
  local release_sha="$2"
  local final_dir="$staging_root/$release_sha"
  local incoming image archive manifest

  if [[ -d "$final_dir" ]]; then
    "$local_artifact_verifier" \
      --artifact-dir "$final_dir" \
      --sha "$release_sha" \
      --inventory "$image_inventory" \
      --allowed-signers "$release_signers_file" >&2
    verify_artifact_dir "$final_dir" "$release_sha" signed
    downloaded_artifact_dir="$final_dir"
    return
  fi

  "$local_artifact_verifier" \
    --artifact-dir "$source_dir" \
    --sha "$release_sha" \
    --inventory "$image_inventory" \
    --allowed-signers "$release_signers_file" >&2
  incoming="$(mktemp -d "$staging_root/.${release_sha}.incoming.XXXXXX")"
  scratch_dirs+=("$incoming")
  install -m 0400 "$source_dir/RELEASE_SHA" "$incoming/RELEASE_SHA"
  for image in "${images[@]}"; do
    archive="${image}-${release_sha}.docker.tar.gz"
    install -m 0400 "$source_dir/$archive" "$incoming/$archive"
    install -m 0400 "$source_dir/${archive}.sha256" "$incoming/${archive}.sha256"
  done
  archive="henukit-runtime-${release_sha}.tar.gz"
  install -m 0400 "$source_dir/$archive" "$incoming/$archive"
  install -m 0400 "$source_dir/${archive}.sha256" "$incoming/${archive}.sha256"
  manifest="henukit-release-${release_sha}.manifest"
  install -m 0400 "$source_dir/$manifest" "$incoming/$manifest"
  install -m 0400 "$source_dir/${manifest}.sig" "$incoming/${manifest}.sig"
  # Re-verify after the copy. The source directory is outside the root-owned
  # staging cache, so validating it only before copying would leave a TOCTOU
  # window in which the archive, checksums, and manifest could be replaced.
  "$local_artifact_verifier" \
    --artifact-dir "$incoming" \
    --sha "$release_sha" \
    --inventory "$image_inventory" \
    --allowed-signers "$release_signers_file" >&2
  verify_artifact_dir "$incoming" "$release_sha" signed
  mv "$incoming" "$final_dir"
  downloaded_artifact_dir="$final_dir"
}

active_release_matches() {
  local release_sha="$1"
  local running image index service
  running="$(docker ps --format '{{.Image}}')"
  for image in "${base_images[@]}"; do
    if [[ "$image" == "henukit-portal-summary" ]]; then
      if release_has_service "$release_sha" "portal-summary"; then
        :
      elif [[ $? -eq 1 ]]; then
        continue
      else
        return 1
      fi
    fi
    grep -Fqx "${image}:${release_sha}" <<<"$running" || return 1
  done
  # Conditional owners are asserted only when the extracted fixed-SHA Compose
  # contract includes them. That preserves rollback to older runtimes while
  # requiring every owner (including Library) in a current release to run.
  for ((index = 0; index < ${#conditional_services[@]}; index++)); do
    service="${conditional_services[$index]}"
    if release_has_service "$release_sha" "$service"; then
      grep -Fqx "${conditional_images[$index]}:${release_sha}" <<<"$running" || return 1
    fi
  done
}

degraded_baseline_matches() {
  local release_sha="$1"
  local expected_target actual_target image index service container actual_image
  [[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ -L "$current_release_link" ]] || return 1
  expected_target="$(cd "$release_root/$release_sha" 2>/dev/null && pwd -P)" || return 1
  actual_target="$(readlink -f "$current_release_link" 2>/dev/null)" || return 1
  [[ "$actual_target" == "$expected_target" ]] || return 1
  for image in "${base_images[@]}"; do
    if [[ "$image" == "henukit-portal-summary" ]]; then
      if release_has_service "$release_sha" "portal-summary"; then
        :
      elif [[ $? -eq 1 ]]; then
        continue
      else
        return 1
      fi
    fi
    container="henukit-${image#henukit-}-1"
    actual_image="$(docker inspect --format '{{.Config.Image}}' "$container" 2>/dev/null)" || return 1
    [[ "$actual_image" == "${image}:${release_sha}" ]] || return 1
  done
  for ((index = 0; index < ${#conditional_services[@]}; index++)); do
    service="${conditional_services[$index]}"
    if release_has_service "$release_sha" "$service"; then
      container="henukit-${service}-1"
      actual_image="$(docker inspect --format '{{.Config.Image}}' "$container" 2>/dev/null)" || return 1
      [[ "$actual_image" == "${conditional_images[$index]}:${release_sha}" ]] || return 1
    fi
  done
}

validate_degraded_baseline_authority() {
  local release_sha="$1"
  local previous_dir="$release_root/$release_sha"
  local marker="$previous_dir/RELEASE_SHA"
  local compose="$previous_dir/docker-compose.henukit.release.yml"
  local helper="$previous_dir/bin/deploy-henukit-artifact.sh"
  local link_owner
  [[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] ||
    die "declared degraded baseline SHA is invalid"
  [[ "$current_release_link" == /* && -L "$current_release_link" ]] ||
    die "degraded baseline current link must be an absolute symlink"
  link_owner="$(file_owner "$current_release_link")"
  [[ "$link_owner" == "0" ]] || die "degraded baseline current link must be owned by root"
  trusted_root_parent_chain "$current_release_link" "degraded baseline current link"
  trusted_root_file "$marker" "degraded baseline RELEASE_SHA"
  [[ "$(tr -d '[:space:]' < "$marker")" == "$release_sha" ]] ||
    die "degraded baseline RELEASE_SHA does not match the declared SHA"
  trusted_root_file "$compose" "degraded baseline Compose contract"
  trusted_root_file "$helper" "degraded baseline deployment helper"
  [[ -x "$helper" ]] || die "degraded baseline deployment helper must be executable"
}

release_has_service() {
  local release_sha="$1"
  local service_name="$2"
  local compose_file="$release_root/$release_sha/docker-compose.henukit.release.yml"
  [[ -r "$compose_file" ]] || return 2
  grep -Eq "^[[:space:]]{2}${service_name}:[[:space:]]*$" "$compose_file"
}

release_uses_account_portfolio() {
  release_has_service "$1" "account-portfolio"
}

container_is_healthy() {
  [[ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$1" 2>/dev/null)" == "healthy" ]]
}

account_portfolio_is_healthy() {
  container_is_healthy "$account_portfolio_container"
}

current_release_sha() {
  local running image line found_sha image_sha
  running="$(docker ps --format '{{.Image}}')" || return 1
  found_sha=""
  # The first Account Portfolio release must be able to roll back to the
  # preceding eight-image runtime, and a Notice/Food release must be able to
  # roll back to a runtime without those owners. Their presence is verified
  # separately from the stable base image set using each release's extracted
  # Compose contract.
  for image in "${base_images[@]}"; do
    [[ "$image" == "henukit-portal-summary" ]] && continue
    line="$(grep -E "^${image}:[0-9a-f]{40}$" <<<"$running" | head -n 1)" || return 1
    image_sha="${line##*:}"
    if [[ -z "$found_sha" ]]; then
      found_sha="$image_sha"
    elif [[ "$image_sha" != "$found_sha" ]]; then
      return 1
    fi
  done
  if release_has_service "$found_sha" "portal-summary"; then
    line="$(grep -E '^henukit-portal-summary:[0-9a-f]{40}$' <<<"$running" | head -n 1)" || return 1
    [[ "${line##*:}" == "$found_sha" ]] || return 1
  elif [[ $? -ne 1 ]]; then
    return 1
  fi
  printf '%s\n' "$found_sha"
}

verify_practice_flow() {
  local release_sha="$1"
  local catalog bank_id bank_version_id session_status answer_status probe_id
  catalog="$(curl --fail --silent --show-error "$public_base_url/api/v1/practice/catalog")" || return 1
  bank_id="$(jq -r '[.banks[] | select(.available == true and .question_count > 0)][0].bank_id // empty' <<<"$catalog")" || return 1
  bank_version_id="$(jq -r '[.banks[] | select(.available == true and .question_count > 0)][0].bank_version_id // empty' <<<"$catalog")" || return 1
  [[ "$bank_id" =~ ^[0-9a-f-]{36}$ && "$bank_version_id" =~ ^[0-9a-f-]{36}$ ]] || return 1
  # These malformed/nonexistent-resource probes cannot persist facts, but they
  # prove that both public command routes are present and reachable rather
  # than being stale 404s. The container probe below exercises real selection,
  # scoring, attempt, and statistics statements inside an explicit rollback.
  probe_id="11111111-1111-4111-8111-111111111111"
  session_status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
    --request POST --header 'Content-Type: application/json' \
    --header "Idempotency-Key: release-practice-route-session-${release_sha}" \
    --data "{\"bank_id\":\"$probe_id\",\"bank_version_id\":\"$probe_id\",\"mode\":\"random\",\"question_count\":1}" \
    "$public_base_url/api/v1/practice/sessions")" || return 1
  [[ "$session_status" == "400" ]] || return 1
  answer_status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
    --request POST --header 'Content-Type: application/json' \
    --header "Idempotency-Key: release-practice-route-answer-${release_sha}" \
    --data "{\"question_id\":\"$probe_id\",\"question_version_id\":\"$probe_id\",\"answer\":false}" \
    "$public_base_url/api/v1/practice/sessions/$probe_id/answers")" || return 1
  [[ "$answer_status" == "404" ]] || return 1
  docker exec henukit-quizcraft-1 /quizcraft verify-practice >/dev/null
}

verify_active_release() {
  local release_sha="$1"
  local account_portfolio_state account_status callback_status practice_state service index
  active_release_matches "$release_sha" || return 1
  if release_uses_account_portfolio "$release_sha"; then
    account_portfolio_state=0
  else
    account_portfolio_state=$?
  fi
  case "$account_portfolio_state" in
    0) account_portfolio_is_healthy || return 1 ;;
    1) ;;
    *) return 1 ;;
  esac
  for ((index = 0; index < ${#conditional_services[@]}; index++)); do
    service="${conditional_services[$index]}"
    if release_has_service "$release_sha" "$service"; then
      case "$service" in
        notice) container_is_healthy "$notice_container" || return 1 ;;
        food) container_is_healthy "$food_container" || return 1 ;;
        library) container_is_healthy "$library_container" || return 1 ;;
      esac
    fi
  done
  if release_has_service "$release_sha" "portal-summary"; then
    container_is_healthy "henukit-portal-summary-1" || return 1
    docker exec henukit-portal-summary-1 /usr/local/bin/portal-summary verify-summary >/dev/null || return 1
  elif [[ $? -ne 1 ]]; then
    return 1
  fi
  curl --fail --silent --show-error "$public_base_url/api/v1/healthz" >/dev/null || return 1
  curl --fail --silent --show-error "$public_base_url/" >/dev/null || return 1
  curl --fail --silent --show-error "$public_base_url/practice" >/dev/null || return 1
  if release_has_service "$release_sha" "quizcraft"; then
    practice_state=0
  else
    practice_state=$?
  fi
  case "$practice_state" in
    0) verify_practice_flow "$release_sha" || return 1 ;;
    1) ;;
    *) return 1 ;;
  esac
  curl --fail --silent --show-error "$public_base_url/library" >/dev/null || return 1
  if [[ "$account_portfolio_state" -eq 0 ]]; then
    account_status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$account_public_origin/api/v1/account/summary")" || return 1
    [[ "$account_status" =~ ^[234][0-9]{2}$ && "$account_status" != "404" ]] || return 1
    callback_status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
      --request POST --header 'Content-Type: application/json' --data '{}' \
      "$account_public_origin/api/v1/payment-providers/easypay/notifications")" || return 1
    [[ "$callback_status" =~ ^4[0-9]{2}$ && "$callback_status" != "404" ]] || return 1
  fi
  [[ "$(curl --location --max-redirs 3 --silent --show-error --output /dev/null --write-out '%{http_code}' "$public_base_url/quiz/")" == "404" ]] ||
    return 1
  [[ "$(curl --location --max-redirs 3 --silent --show-error --output /dev/null --write-out '%{http_code}' "$public_base_url/study-api/healthz")" == "404" ]] ||
    return 1
}

wait_for_active_release() {
  local release_sha="$1"
  local attempt
  for ((attempt = 1; attempt <= 30; attempt++)); do
    if verify_active_release "$release_sha"; then
      return 0
    fi
    if ((attempt < 30)); then
      log "release $release_sha is not ready yet (attempt $attempt/30)"
      sleep 2
    fi
  done
  return 1
}

grant_account_operator_permissions() {
  local release_sha="$1"
  docker exec "$platform_core_container" grant-account-operator-role \
    --role-code "$account_operator_role" \
    --request-id "req_release_${release_sha}" \
    --reason "Account Portfolio production release $release_sha" || return 1
  log "Platform Core audited the Account Console permission grant for role $account_operator_role"
}

record_activation() {
  local release_sha="$1"
  local temporary="$state_root/.last-activated-sha.$$"
  printf '%s\n' "$release_sha" > "$temporary"
  chmod 0600 "$temporary"
  mv "$temporary" "$state_root/last-activated-sha"
}

ensure_account_portfolio_database() {
  # A long-lived PostgreSQL volume skips docker-entrypoint init scripts on
  # later releases. Creating this empty owner database before its backup makes
  # the first Account Portfolio cutover recoverable without deleting it on a
  # rollback to the preceding runtime.
  docker exec "$postgres_container" sh -ceu '
    if ! psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc \
      "SELECT 1 FROM pg_database WHERE datname = '\''account_portfolio'\''" | grep -qx 1; then
      createdb -U "$POSTGRES_USER" account_portfolio
    fi
  '
}

account_portfolio_schema_is_present() {
  docker exec "$postgres_container" sh -ceu \
    'psql -U "$POSTGRES_USER" -d account_portfolio -Atqc "$1"' sh \
    "SELECT to_regclass('public.account_portfolio_accounts') IS NOT NULL AND to_regclass('public.account_portfolio_points') IS NOT NULL AND to_regclass('public.account_portfolio_memberships') IS NOT NULL AND to_regclass('public.account_portfolio_point_ledger') IS NOT NULL AND to_regclass('public.account_portfolio_notifications') IS NOT NULL AND to_regclass('public.account_portfolio_tickets') IS NOT NULL AND to_regclass('public.account_portfolio_ticket_messages') IS NOT NULL AND to_regclass('public.account_portfolio_membership_orders') IS NOT NULL AND to_regclass('public.account_portfolio_service_nonces') IS NOT NULL AND to_regclass('public.account_portfolio_schema_migrations') IS NOT NULL"
}

prepare_backup() {
  local release_sha="$1"
  local refresh="${2:-no}"
  local marker="$state_root/prepared/$release_sha"
  local timestamp backup_file backup_sha backup_size database_version restored_counts
  local account_backup_file account_backup_sha account_backup_size account_schema_present account_restored_counts
  local marker_incoming

  if [[ "$refresh" != "yes" && -s "$marker" ]]; then
    backup_file="$(tr -d '\r\n' < "$marker")"
    [[ "$backup_file" == "$backup_root/"* ]] || die "prepared backup path is outside HENUKIT_BACKUP_ROOT"
    [[ -s "$backup_file" && -s "$backup_file.sha256" && -s "$backup_file.meta" ]] ||
      die "prepared backup evidence is incomplete"
    account_backup_file="$(awk -F= '$1 == "account_portfolio_backup" { print substr($0, index($0, "=") + 1); exit }' "$backup_file.meta")"
    [[ "$account_backup_file" == "$backup_root/"* ]] || die "prepared Account Portfolio backup evidence is incomplete"
    [[ -s "$account_backup_file" && -s "$account_backup_file.sha256" ]] ||
      die "prepared Account Portfolio backup evidence is incomplete"
    (cd "$backup_root" && sha256sum -c "$(basename "$backup_file").sha256") >&2 ||
      die "prepared backup checksum verification failed"
    (cd "$backup_root" && sha256sum -c "$(basename "$account_backup_file").sha256") >&2 ||
      die "prepared Account Portfolio backup checksum verification failed"
    prepared_backup_file="$backup_file"
    prepared_account_portfolio_backup_file="$account_backup_file"
    return
  fi

  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_file="$backup_root/platform-${timestamp}-${release_sha:0:12}-$$.dump"
  account_backup_file="$backup_root/account-portfolio-${timestamp}-${release_sha:0:12}-$$.dump"
  log "creating Platform database backup $backup_file"
  docker exec "$postgres_container" sh -ceu \
    'pg_dump -U "$POSTGRES_USER" -d platform -Fc' > "$backup_file"
  [[ -s "$backup_file" ]] || die "Platform database backup is empty"
  chmod 0600 "$backup_file"
  (
    cd "$backup_root"
    sha256sum "$(basename "$backup_file")" > "$(basename "$backup_file").sha256"
  )
  backup_sha="$(awk '{print $1}' "$backup_file.sha256")"
  backup_size="$(wc -c < "$backup_file" | tr -d '[:space:]')"
  database_version="$(
    docker exec "$postgres_container" sh -ceu \
      'psql -U "$POSTGRES_USER" -d platform -Atqc "SHOW server_version"'
  )"

  ensure_account_portfolio_database
  account_schema_present="$(account_portfolio_schema_is_present)"
  [[ "$account_schema_present" == "t" || "$account_schema_present" == "f" ]] ||
    die "Account Portfolio schema presence check returned an invalid value"
  log "creating Account Portfolio database backup $account_backup_file"
  docker exec "$postgres_container" sh -ceu \
    'pg_dump -U "$POSTGRES_USER" -d account_portfolio -Fc' > "$account_backup_file"
  [[ -s "$account_backup_file" ]] || die "Account Portfolio database backup is empty"
  chmod 0600 "$account_backup_file"
  (
    cd "$backup_root"
    sha256sum "$(basename "$account_backup_file")" > "$(basename "$account_backup_file").sha256"
  )
  account_backup_sha="$(awk '{print $1}' "$account_backup_file.sha256")"
  account_backup_size="$(wc -c < "$account_backup_file" | tr -d '[:space:]')"

  restore_database="henukit_verify_${release_sha:0:8}_$$"
  docker exec "$postgres_container" sh -ceu \
    'createdb -U "$POSTGRES_USER" -T template0 "$1"' sh "$restore_database"
  docker exec -i "$postgres_container" sh -ceu \
    'pg_restore --exit-on-error --no-owner --no-privileges -U "$POSTGRES_USER" -d "$1"' \
    sh "$restore_database" < "$backup_file"
  restored_counts="$(
    docker exec "$postgres_container" sh -ceu \
      'psql -U "$POSTGRES_USER" -d "$1" -Atqc "$2"' \
      sh "$restore_database" \
      "SELECT (SELECT count(*) FROM users)::text || ',' || (SELECT count(*) FROM oauth_clients)::text || ',' || (SELECT count(*) FROM sessions)::text"
  )"
  [[ "$restored_counts" =~ ^[0-9]+,[0-9]+,[0-9]+$ ]] ||
    die "isolated restore key-table count check failed"
  docker exec "$postgres_container" sh -ceu \
    'dropdb -U "$POSTGRES_USER" "$1"' sh "$restore_database"
  restore_database=""

  restore_account_database="henukit_account_verify_${release_sha:0:8}_$$"
  docker exec "$postgres_container" sh -ceu \
    'createdb -U "$POSTGRES_USER" -T template0 "$1"' sh "$restore_account_database"
  docker exec -i "$postgres_container" sh -ceu \
    'pg_restore --exit-on-error --no-owner --no-privileges -U "$POSTGRES_USER" -d "$1"' \
    sh "$restore_account_database" < "$account_backup_file"
  if [[ "$account_schema_present" == "t" ]]; then
    account_restored_counts="$(
      docker exec "$postgres_container" sh -ceu \
        'psql -U "$POSTGRES_USER" -d "$1" -Atqc "$2"' \
        sh "$restore_account_database" \
        "SELECT (SELECT count(*) FROM account_portfolio_accounts)::text || ',' || (SELECT count(*) FROM account_portfolio_points)::text || ',' || (SELECT count(*) FROM account_portfolio_memberships)::text || ',' || (SELECT count(*) FROM account_portfolio_point_ledger)::text || ',' || (SELECT count(*) FROM account_portfolio_notifications)::text || ',' || (SELECT count(*) FROM account_portfolio_tickets)::text || ',' || (SELECT count(*) FROM account_portfolio_ticket_messages)::text || ',' || (SELECT count(*) FROM account_portfolio_membership_orders)::text || ',' || (SELECT count(*) FROM account_portfolio_service_nonces)::text || ',' || (SELECT count(*) FROM account_portfolio_schema_migrations)::text"
    )"
    [[ "$account_restored_counts" =~ ^[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+,[0-9]+$ ]] ||
      die "isolated Account Portfolio restore count check failed"
  else
    account_restored_counts="$(
      docker exec "$postgres_container" sh -ceu \
        'psql -U "$POSTGRES_USER" -d "$1" -Atqc "$2"' \
        sh "$restore_account_database" \
        "SELECT to_regclass('public.account_portfolio_accounts') IS NOT NULL"
    )"
    [[ "$account_restored_counts" == "f" ]] ||
      die "isolated empty Account Portfolio database restore check failed"
  fi
  docker exec "$postgres_container" sh -ceu \
    'dropdb -U "$POSTGRES_USER" "$1"' sh "$restore_account_database"
  restore_account_database=""

  {
    printf 'release_sha=%s\n' "$release_sha"
    printf 'created_at_utc=%s\n' "$timestamp"
    printf 'sha256=%s\n' "$backup_sha"
    printf 'size_bytes=%s\n' "$backup_size"
    printf 'postgres_version=%s\n' "$database_version"
    printf 'restored_counts_users_oauth_clients_sessions=%s\n' "$restored_counts"
    printf 'account_portfolio_backup=%s\n' "$account_backup_file"
    printf 'account_portfolio_sha256=%s\n' "$account_backup_sha"
    printf 'account_portfolio_size_bytes=%s\n' "$account_backup_size"
    printf 'account_portfolio_schema_before_release=%s\n' "$account_schema_present"
    printf 'account_portfolio_restored_counts=%s\n' "$account_restored_counts"
  } > "$backup_file.meta"
  chmod 0600 "$backup_file.sha256" "$account_backup_file.sha256" "$backup_file.meta"

  marker_incoming="$state_root/prepared/.${release_sha}.$$"
  printf '%s\n' "$backup_file" > "$marker_incoming"
  chmod 0600 "$marker_incoming"
  mv "$marker_incoming" "$marker"
  prepared_backup_file="$backup_file"
  prepared_account_portfolio_backup_file="$account_backup_file"
}

release_is_approved() {
  local release_sha="$1"
  local approval="$state_root/approvals/$release_sha"
  local approval_mode approval_owner
  [[ -f "$approval" && -r "$approval" ]] || return 1
  approval_mode="$(file_mode "$approval")"
  approval_owner="$(file_owner "$approval")"
  [[ "$approval_mode" == "600" || "$approval_mode" == "400" ]] || return 1
  [[ "$approval_owner" == "$(id -u)" ]] || return 1
  [[ "$(tr -d '\r\n' < "$approval")" == "$release_sha" ]]
}

consume_approval() {
  local release_sha="$1"
  local approval="$state_root/approvals/$release_sha"
  local consumed="$state_root/approvals/consumed/${release_sha}.$(date -u +%Y%m%dT%H%M%SZ).$$"
  release_is_approved "$release_sha" || die "exact-SHA approval disappeared before activation"
  mv "$approval" "$consumed"
  chmod 0400 "$consumed"
}

github_branch_head() {
  gh api "repos/$repo/branches/$branch" --jq '.commit.sha'
}

rollback_release() {
  local previous_sha="$1"
  local previous_dir="$release_root/$previous_sha"
  local previous_helper="$previous_dir/bin/deploy-henukit-artifact.sh"
  [[ "$previous_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA" 2>/dev/null)" == "$previous_sha" ]] ||
    return 1
  [[ -x "$previous_helper" ]] || return 1
  log "rolling back to release $previous_sha"
  "$previous_helper" "$previous_dir" "$rollback_env_file" || return 1
  wait_for_active_release "$previous_sha"
}

rollback_release_is_ready() {
  local previous_sha="$1"
  local previous_dir="$release_root/$previous_sha"
  [[ "$previous_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA" 2>/dev/null)" == "$previous_sha" ]] ||
    return 1
  [[ -x "$previous_dir/bin/deploy-henukit-artifact.sh" ]] || return 1
  verify_active_release "$previous_sha"
}

degraded_recovery_audit_matches() {
  local target="$1"
  local candidate_sha="$2"
  local previous_sha="$3"
  local status="$4"
  local backup_file="${5:-}"
  local line_count expected_count
  [[ -f "$target" && ! -L "$target" ]] || return 1
  [[ "$(file_owner "$target")" == "$(id -u)" ]] || return 1
  [[ "$(file_mode "$target")" == "400" ]] || return 1
  [[ "$(grep -Fxc "candidate_sha=$candidate_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "previous_sha=$previous_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "status=$status" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Ec '^recorded_at_utc=[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$' "$target" || true)" == "1" ]] || return 1
  expected_count=4
  if [[ "$backup_file" == "*" ]]; then
    [[ "$(grep -Ec '^verified_backup=/[^[:cntrl:]]+$' "$target" || true)" == "1" ]] || return 1
    expected_count=5
  elif [[ -n "$backup_file" ]]; then
    [[ "$(grep -Fxc "verified_backup=$backup_file" "$target" || true)" == "1" ]] || return 1
    expected_count=5
  else
    ! grep -q '^verified_backup=' "$target" || return 1
  fi
  line_count="$(wc -l < "$target" | tr -d '[:space:]')"
  [[ "$line_count" == "$expected_count" ]]
}

ensure_degraded_recovery_audit() {
  local candidate_sha="$1"
  local previous_sha="$2"
  local status="$3"
  local suffix="$4"
  local backup_file="${5:-}"
  local directory="$state_root/degraded-recoveries"
  local target="$directory/${candidate_sha}.${suffix}"
  local incoming="$directory/.${candidate_sha}.${suffix}.$$"
  if [[ -e "$target" ]]; then
    degraded_recovery_audit_matches \
      "$target" "$candidate_sha" "$previous_sha" "$status" "$backup_file" ||
      die "degraded recovery audit conflicts with the requested recovery: $target"
    return
  fi
  umask 077
  {
    printf 'candidate_sha=%s\n' "$candidate_sha"
    printf 'previous_sha=%s\n' "$previous_sha"
    printf 'status=%s\n' "$status"
    printf 'recorded_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    [[ -z "$backup_file" ]] || printf 'verified_backup=%s\n' "$backup_file"
  } > "$incoming"
  chmod 0400 "$incoming"
  ln "$incoming" "$target" || {
    rm -f -- "$incoming"
    die "could not atomically publish degraded recovery audit"
  }
  rm -f -- "$incoming"
}

restore_degraded_baseline() {
  local previous_sha="$1"
  local previous_dir="$release_root/$previous_sha"
  local previous_helper="$previous_dir/bin/deploy-henukit-artifact.sh"
  [[ "$previous_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA" 2>/dev/null)" == "$previous_sha" ]] ||
    return 1
  [[ -x "$previous_helper" ]] || return 1
  log "restoring known degraded baseline $previous_sha"
  "$previous_helper" "$previous_dir" "$rollback_env_file" || return 1
  degraded_baseline_matches "$previous_sha"
}

fail_candidate_activation() {
  local phase="$1"
  local previous_sha="$2"
  local release_sha="$3"
  if [[ -n "$recovery_baseline_sha" ]]; then
    restore_degraded_baseline "$previous_sha" ||
      die "$phase failed and restoration of degraded baseline $previous_sha also failed"
    ensure_degraded_recovery_audit \
      "$release_sha" "$previous_sha" restored_known_degraded_baseline restored
    die "$phase failed; restored known degraded baseline $previous_sha"
  fi
  rollback_release "$previous_sha" ||
    die "$phase failed and rollback to $previous_sha also failed"
  die "$phase failed; rolled back to $previous_sha"
}

deploy_release() {
  local run_id="$1"
  local release_sha="$2"
  local run_url="$3"
  local artifact_override="${4:-}"
  local artifact_dir runtime_archive release_dir release_incoming
  local image helper previous_sha activation_status

  [[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release source returned an invalid SHA"

  if active_release_matches "$release_sha"; then
    verify_active_release "$release_sha" || die "active release failed public health verification"
    if [[ -n "$recovery_baseline_sha" ]]; then
      degraded_recovery_audit_matches \
        "$state_root/degraded-recoveries/${release_sha}.authorized" \
        "$release_sha" "$recovery_baseline_sha" authorized "*" ||
        die "active recovery candidate has no matching immutable authorization audit"
    fi
    if release_uses_account_portfolio "$release_sha"; then
      grant_account_operator_permissions "$release_sha" || die "active release permission grant did not converge"
    fi
    if [[ -n "$recovery_baseline_sha" ]]; then
      ensure_degraded_recovery_audit \
        "$release_sha" "$recovery_baseline_sha" activated activated
    fi
    record_activation "$release_sha"
    log "release $release_sha is already active"
    return
  fi

  if [[ -n "$recovery_baseline_sha" &&
        -e "$state_root/degraded-recoveries/${release_sha}.authorized" ]]; then
    degraded_recovery_audit_matches \
      "$state_root/degraded-recoveries/${release_sha}.authorized" \
      "$release_sha" "$recovery_baseline_sha" authorized "*" ||
      die "existing degraded recovery authorization audit is invalid"
    validate_degraded_baseline_authority "$recovery_baseline_sha"
    degraded_baseline_matches "$recovery_baseline_sha" ||
      die "prior degraded recovery is neither active nor restored to its exact baseline"
    ensure_degraded_recovery_audit \
      "$release_sha" "$recovery_baseline_sha" restored_known_degraded_baseline restored
    die "prior degraded recovery attempt converged to the known degraded baseline; issue a new exact-SHA approval before retrying"
  fi

  if [[ -n "$artifact_override" ]]; then
    log "using verified local main artifact set from $run_url"
    verify_artifact_dir "$artifact_override" "$release_sha" signed
    artifact_dir="$artifact_override"
  else
    log "downloading successful main artifact set from $run_url"
    downloaded_artifact_dir=""
    download_artifacts "$run_id" "$release_sha"
    artifact_dir="$downloaded_artifact_dir"
  fi
  runtime_archive="$artifact_dir/henukit-runtime-${release_sha}.tar.gz"
  release_dir="$release_root/$release_sha"

  if [[ ! -d "$release_dir" ]]; then
    release_incoming="$(mktemp -d "$release_root/.${release_sha}.incoming.XXXXXX")"
    scratch_dirs+=("$release_incoming")
    tar -xzf "$runtime_archive" -C "$release_incoming"
    [[ "$(tr -d '[:space:]' < "$release_incoming/RELEASE_SHA")" == "$release_sha" ]] ||
      die "runtime RELEASE_SHA does not match the release source"
    [[ -x "$release_incoming/bin/deploy-henukit-artifact.sh" ]] ||
      die "runtime artifact has no executable deployment helper"
    mv "$release_incoming" "$release_dir"
  fi
  [[ "$(tr -d '[:space:]' < "$release_dir/RELEASE_SHA")" == "$release_sha" ]] ||
    die "release directory SHA does not match the workflow run"
  [[ -x "$release_dir/bin/deploy-henukit-artifact.sh" ]] ||
    die "release directory has no executable deployment helper"
  verify_account_boundary_manifest "$release_dir" "$release_sha"

  if ! release_is_approved "$release_sha"; then
    prepared_backup_file=""
    prepare_backup "$release_sha"
    log "release $release_sha prepared with verified backup $prepared_backup_file"
    log "release $release_sha awaits exact-SHA approval at $state_root/approvals/$release_sha"
    return
  fi

  if [[ -n "$recovery_baseline_sha" ]]; then
    previous_sha="$recovery_baseline_sha"
    validate_degraded_baseline_authority "$previous_sha"
    degraded_baseline_matches "$previous_sha" ||
      die "declared degraded baseline does not match the current release link and exact image set"
    if rollback_release_is_ready "$previous_sha"; then
      die "declared recovery baseline is healthy; use the normal rollback-protected activation path"
    fi
    log "authorized degraded-baseline recovery from exact release $previous_sha"
  else
    previous_sha="$(current_release_sha 2>/dev/null || true)"
    rollback_release_is_ready "$previous_sha" ||
      die "no healthy fixed-SHA rollback release is ready; refusing production activation"
  fi
  prepared_backup_file=""
  prepare_backup "$release_sha" yes
  log "release $release_sha has a fresh verified pre-activation backup $prepared_backup_file"
  [[ "$(github_branch_head)" == "$release_sha" ]] ||
    die "GitHub branch head changed during preparation; refusing stale activation"
  if [[ -n "$recovery_baseline_sha" ]]; then
    ensure_degraded_recovery_audit \
      "$release_sha" "$previous_sha" authorized authorized "$prepared_backup_file"
  fi
  consume_approval "$release_sha"
  for image in "${load_images[@]}"; do
    log "loading ${image}:${release_sha}"
    gzip -dc "$artifact_dir/${image}-${release_sha}.docker.tar.gz" | docker load >/dev/null
  done

  helper="$release_dir/bin/deploy-henukit-artifact.sh"
  set +e
  if [[ -n "$migration" ]]; then
    "$helper" "$release_dir" "$env_file" "$migration"
    activation_status=$?
  else
    "$helper" "$release_dir" "$env_file"
    activation_status=$?
  fi
  set -e
  if [[ "$activation_status" -ne 0 ]]; then
    fail_candidate_activation "release activation" "$previous_sha" "$release_sha"
  fi

  if ! wait_for_active_release "$release_sha"; then
    fail_candidate_activation "release verification" "$previous_sha" "$release_sha"
  fi
  if ! grant_account_operator_permissions "$release_sha"; then
    fail_candidate_activation "permission grant" "$previous_sha" "$release_sha"
  fi
  if [[ -n "$recovery_baseline_sha" ]]; then
    ensure_degraded_recovery_audit "$release_sha" "$previous_sha" activated activated
  fi
  record_activation "$release_sha"
  log "release $release_sha activated and deterministic smoke checks passed; manual acceptance remains"
}

check_once() {
  local run_row run_id release_sha run_status run_conclusion run_url branch_head
  run_row="$(
    gh run list \
      --repo "$repo" \
      --workflow "$workflow" \
      --branch "$branch" \
      --event push \
      --limit 20 \
      --json databaseId,headSha,status,conclusion,url \
      --jq 'first(.[]) | [(.databaseId|tostring),.headSha,.status,.conclusion,.url] | @tsv'
  )"
  if [[ -z "$run_row" ]]; then
    log "no completed successful $workflow run found on $branch"
    return
  fi
  IFS=$'\t' read -r run_id release_sha run_status run_conclusion run_url <<<"$run_row"
  [[ "$run_id" =~ ^[0-9]+$ ]] || die "GitHub returned an invalid workflow run id"
  if [[ "$run_status" != "completed" || "$run_conclusion" != "success" ]]; then
    log "latest $branch workflow run $run_id is not successfully completed; refusing stale artifacts"
    return
  fi
  branch_head="$(github_branch_head)"
  [[ "$branch_head" =~ ^[0-9a-f]{40}$ ]] || die "GitHub returned an invalid $branch head SHA"
  if [[ "$branch_head" != "$release_sha" ]]; then
    log "successful run SHA is no longer current $branch; refusing stale artifacts"
    return
  fi
  deploy_release "$run_id" "$release_sha" "$run_url"
}

check_local_artifacts() {
  local branch_head
  branch_head="$(github_branch_head)"
  [[ "$branch_head" =~ ^[0-9a-f]{40}$ ]] || die "GitHub returned an invalid $branch head SHA"
  [[ "$branch_head" == "$local_release_sha" ]] ||
    die "local artifact SHA is no longer the current $branch head"
  stage_local_artifacts "$local_artifact_dir" "$local_release_sha"
  deploy_release \
    "local-${local_release_sha}" \
    "$local_release_sha" \
    "$local_artifact_dir" \
    "$downloaded_artifact_dir"
}

quiesce_file="$state_root/quiesce.request"
quiesced_file="$state_root/quiesced"
quiesce_sha=""
quiesce_instance=""
quiesce_nonce=""
quiesce_requested() {
  local mode owner
  [[ -e "$quiesce_file" ]] || return 1
  [[ -f "$quiesce_file" && ! -L "$quiesce_file" ]] ||
    die "watcher quiesce request must be a regular non-symlink file"
  mode="$(file_mode "$quiesce_file")"
  owner="$(file_owner "$quiesce_file")"
  [[ "$owner" == "$(id -u)" ]] || die "watcher quiesce request has the wrong owner"
  [[ "$mode" == "600" || "$mode" == "400" ]] ||
    die "watcher quiesce request mode must be 0600 or 0400"
  read -r quiesce_sha quiesce_instance quiesce_nonce extra < "$quiesce_file"
  [[ "$quiesce_sha" =~ ^[0-9a-f]{40}$ && "$quiesce_instance" =~ ^[1-9][0-9]*$ &&
     "$quiesce_nonce" =~ ^[0-9a-f]{32}$ && -z "${extra:-}" ]] ||
    die "watcher quiesce request is not bound to this watcher instance"
  if [[ "$quiesce_instance" != "$$" ]]; then
    log "ignoring stale quiesce request for watcher instance $quiesce_instance"
    return 1
  fi
  return 0
}

acknowledge_quiesce() {
  local incoming="$state_root/.quiesced.$$"
  umask 077
  printf '%s %s %s\n' "$quiesce_sha" "$quiesce_instance" "$quiesce_nonce" > "$incoming"
  chmod 0600 "$incoming"
  mv "$incoming" "$quiesced_file"
}

if [[ "$mode" == "--local-artifacts" ]]; then
  check_local_artifacts
  exit 0
fi

if [[ "$mode" == "--once" ]]; then
  check_once
  exit 0
fi

log "watching $repo $workflow on $branch every ${poll_seconds}s"
instance_incoming="$state_root/.watcher.instance.$$"
umask 077
printf '%s\n' "$$" > "$instance_incoming"
chmod 0600 "$instance_incoming"
mv "$instance_incoming" "$watcher_instance_file"
watcher_instance_owned=1
while true; do
  if quiesce_requested; then
    acknowledge_quiesce
    log "quiesce requested at a safe boundary; releasing the watcher lock"
    exit 0
  fi
  check_once
  if quiesce_requested; then
    acknowledge_quiesce
    log "quiesce requested after a completed check; releasing the watcher lock"
    exit 0
  fi
  sleep "$poll_seconds"
done
