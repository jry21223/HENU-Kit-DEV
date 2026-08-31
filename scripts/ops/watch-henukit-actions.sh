#!/usr/bin/env bash
# Watch successful main-branch GitHub Actions builds and deploy their fixed-SHA
# artifacts directly on the production host. This script never compiles source.
set -Eeuo pipefail

program="watch-henukit-actions"
mode=""

usage() {
  cat >&2 <<'EOF'
usage: watch-henukit-actions.sh --once|--watch
       watch-henukit-actions.sh --once \
         --recover-degraded-baseline <full-current-sha>
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
  HENUKIT_ACTIVE_RELEASE_ATTEMPTS Readiness attempts per activation (default: 30)
  HENUKIT_MIN_ACTIVATION_FREE_MIB Minimum free space before approval consumption (default: 4096)
  HENUKIT_PUBLIC_BASE_URL Public smoke-test base URL
  HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE Active role receiving Account Console permissions
  HENUKIT_PLATFORM_MIGRATIONS Additional comma-separated reviewed Platform Core migrations
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
elif [[ $# -eq 3 && "$1" == "--once" &&
        "$2" == "--recover-degraded-baseline" ]]; then
  mode="--once"
  recovery_baseline_sha="$3"
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
active_release_attempts="${HENUKIT_ACTIVE_RELEASE_ATTEMPTS:-30}"
minimum_activation_free_mib="${HENUKIT_MIN_ACTIVATION_FREE_MIB:-4096}"
public_base_url="${HENUKIT_PUBLIC_BASE_URL:-https://superhuazai.me}"
account_public_origin="https://henukit.cn"
postgres_container="${HENUKIT_POSTGRES_CONTAINER:-henukit-postgres-1}"
account_portfolio_container="${HENUKIT_ACCOUNT_PORTFOLIO_CONTAINER:-henukit-account-portfolio-1}"
platform_core_container="${HENUKIT_PLATFORM_CORE_CONTAINER:-henukit-platform-core-1}"
notice_container="${HENUKIT_NOTICE_CONTAINER:-henukit-notice-1}"
food_container="${HENUKIT_FOOD_CONTAINER:-henukit-food-1}"
library_container="${HENUKIT_LIBRARY_CONTAINER:-henukit-library-1}"
migration="${HENUKIT_PLATFORM_MIGRATIONS:-${HENUKIT_PLATFORM_MIGRATION:-}}"
required_platform_migrations=(
  000019_career_digest_mail.up.sql
  000020_mail_outbox_allow_bulk_priority.up.sql
)
migration_candidates=()
if [[ -n "$migration" ]]; then
  IFS=',' read -r -a migration_candidates <<<"$migration"
fi
migration_candidates+=("${required_platform_migrations[@]}")
sorted_migration_candidates=()
while IFS= read -r migration_name; do
  sorted_migration_candidates+=("$migration_name")
done < <(printf '%s\n' "${migration_candidates[@]}" | LC_ALL=C sort -u)
migration_candidates=("${sorted_migration_candidates[@]}")
migration="$(IFS=,; printf '%s' "${migration_candidates[*]}")"
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
[[ "$active_release_attempts" =~ ^[1-9][0-9]*$ ]] ||
  die "HENUKIT_ACTIVE_RELEASE_ATTEMPTS must be a positive integer"
[[ "$minimum_activation_free_mib" =~ ^[1-9][0-9]*$ ]] ||
  die "HENUKIT_MIN_ACTIVATION_FREE_MIB must be a positive integer"
((minimum_activation_free_mib >= 4096)) ||
  die "HENUKIT_MIN_ACTIVATION_FREE_MIB cannot lower the 4096 MiB production floor"
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_REPO must be an owner/name pair"
[[ "$branch" =~ ^[A-Za-z0-9_.-]+$ ]] || die "HENUKIT_BRANCH contains unsupported characters"
[[ "$account_operator_role" =~ ^[a-z0-9][a-z0-9-]{0,63}$ ]] ||
  die "HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE must name an explicit role using lowercase letters, digits, or hyphens"
command -v gh >/dev/null 2>&1 || die "gh CLI is required"
command -v cmp >/dev/null 2>&1 || die "cmp is required"
command -v docker >/dev/null 2>&1 || die "docker is required"
command -v df >/dev/null 2>&1 || die "df is required"
command -v flock >/dev/null 2>&1 || die "flock is required"
command -v jq >/dev/null 2>&1 || die "jq is required"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"
command -v systemctl >/dev/null 2>&1 || die "systemctl is required"
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
if [[ -n "$recovery_baseline_sha" ]]; then
  [[ "$recovery_baseline_sha" =~ ^[0-9a-f]{40}$ ]] ||
    die "--recover-degraded-baseline must be a full lowercase Git SHA"
  [[ "$branch" == "main" ]] || die "degraded-baseline recovery may only activate main"
fi
if [[ "$mode" == "--local-artifacts" ]]; then
  [[ "$local_release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
  if [[ -n "$recovery_baseline_sha" ]]; then
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
  local career_ai_base career_ai_key career_ai_model career_ai_insecure career_suify_ai_insecure career_digest_secret
  local career_digest_client career_digest_key normalized_secret normalized_ai
  local getwork_token normalized_getwork_token
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
  career_ai_base="$(environment_value CAREER_AI_BASE_URL)"
  career_ai_key="$(environment_value CAREER_AI_API_KEY)"
  career_ai_model="$(environment_value CAREER_AI_MODEL)"
  career_ai_insecure="$(environment_value CAREER_ALLOW_INSECURE_AI_HTTP)"
  career_suify_ai_insecure="$(environment_value CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP)"
  career_digest_client="$(environment_value PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID)"
  career_digest_key="$(environment_value PLATFORM_CORE_CAREER_DIGEST_KEY_ID)"
  career_digest_secret="$(environment_value PLATFORM_CORE_CAREER_DIGEST_SECRET)"
  getwork_token="$(environment_value GETWORK_MCP_ACCESS_TOKEN)"
  if [[ "$career_ai_base" == "http://125.46.96.207:30000/v1" ]]; then
    [[ "$career_ai_insecure" == "1" ]] || die "the approved plaintext Career LLM requires CAREER_ALLOW_INSECURE_AI_HTTP=1"
    [[ -z "$career_suify_ai_insecure" || "$career_suify_ai_insecure" == "0" || "$career_suify_ai_insecure" == "1" ]] ||
      die "CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP must be 0 or 1"
  else
    [[ "$career_ai_base" == https://* || "$career_ai_base" == http://127.0.0.1* || "$career_ai_base" == http://localhost* ]] ||
      die "CAREER_AI_BASE_URL must use HTTPS, loopback, or the exact approved HTTP endpoint"
    [[ -z "$career_ai_insecure" || "$career_ai_insecure" == "0" ]] ||
      die "CAREER_ALLOW_INSECURE_AI_HTTP=1 is valid only for the exact approved HTTP endpoint"
    [[ -z "$career_suify_ai_insecure" || "$career_suify_ai_insecure" == "0" ]] ||
      die "CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP=1 is valid only for the exact approved HTTP endpoint"
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
  [[ ${#getwork_token} -ge 32 && "$getwork_token" != *[[:space:]]* ]] ||
    die "getWork MCP access token is missing or invalid"
  normalized_getwork_token="$(printf '%s' "$getwork_token" | tr '[:upper:]' '[:lower:]')"
  [[ ! "$normalized_getwork_token" =~ (replace|example|change-me|changeme|test-secret|for-test|test-only) ]] ||
    die "getWork MCP access token contains a deployment placeholder"
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
  "$state_root/degraded-recoveries" \
  "$state_root/rollback-contracts/pending" "$state_root/rollback-contracts/completed"
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
  if [[ "$signature_policy" == "unsigned" &&
        "$candidate" == "oauth-continuation-${release_sha}.env" ]]; then
    return 0
  fi
  [[ "$signature_policy" == "signed" &&
     ( "$candidate" == "henukit-release-${release_sha}.manifest" ||
       "$candidate" == "henukit-release-${release_sha}.manifest.sig" ) ]]
}

is_getwork_provenance_file() {
  local candidate="$1"
  local release_sha="$2"
  [[ "$candidate" == "henukit-getwork-actions-${release_sha}.manifest" ||
     "$candidate" == "henukit-getwork-actions-${release_sha}.attestation.json" ]]
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

verify_workflow_continuation_gate_receipt() {
  local artifact_dir="$1"
  local release_dir="$2"
  local release_sha="$3"
  local receipt="$artifact_dir/oauth-continuation-${release_sha}.env"
  local packaged_receipt="$release_dir/release-gates/oauth-continuation.env"

  [[ -e "$receipt" || -L "$receipt" ]] ||
    die "workflow continuation gate receipt is missing"
  [[ -f "$receipt" && -r "$receipt" && -s "$receipt" && ! -L "$receipt" ]] ||
    die "workflow continuation gate receipt must be a readable regular non-symlink file"
  [[ -f "$packaged_receipt" && -r "$packaged_receipt" && -s "$packaged_receipt" &&
     ! -L "$packaged_receipt" ]] ||
    die "packaged runtime has no continuation gate receipt"
  cmp -s "$receipt" "$packaged_receipt" ||
    die "continuation gate receipt does not match the packaged runtime"
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
    # The WSL handoff has its own attested artifact pair. The production
    # watcher downloads the complete run for the fixed-SHA Compose release,
    # but does not consume that pair; remove only these two exact, known names
    # before the ordinary production artifact contract is checked.
    if is_getwork_provenance_file "$base" "$release_sha"; then
      rm -f -- "$file"
      continue
    fi
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
  local running image index service tagged_image
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
  getwork_relay_matches "$release_sha" || return 1
  # A previous release is not exact when a partial candidate switch left any
  # additional HENU image on a different SHA. This prevents rollback from
  # treating a mixed runtime as an already-healthy previous release.
  while IFS= read -r tagged_image; do
    [[ "${tagged_image##*:}" == "$release_sha" ]] || return 1
  done < <(grep -E '^henukit-[a-z0-9-]+:[0-9a-f]{40}$' <<<"$running" || true)
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
  getwork_relay_matches "$release_sha" || return 1
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

getwork_relay_matches() {
  local release_sha="$1"
  local state actual_image network_mode expected_bridge relay_environment
  if release_has_service "$release_sha" "getwork-mcp-relay"; then
    state=0
  else
    state=$?
  fi
  case "$state" in
    0)
      actual_image="$(docker inspect --format '{{.Config.Image}}' henukit-getwork-mcp-relay-1 2>/dev/null)" || return 1
      [[ "$actual_image" == "henukit-career-opportunities:${release_sha}" ]] || return 1
      network_mode="$(docker inspect --format '{{.HostConfig.NetworkMode}}' henukit-getwork-mcp-relay-1 2>/dev/null)" ||
        return 1
      [[ "$network_mode" == host ]] || return 1
      expected_bridge="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}' 2>/dev/null)" ||
        return 1
      [[ "$expected_bridge" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
      relay_environment="$(docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' henukit-getwork-mcp-relay-1 2>/dev/null)" ||
        return 1
      [[ "$(grep -Fxc "GETWORK_RELAY_ADDR=${expected_bridge}:18101" <<<"$relay_environment")" -eq 1 ]] ||
        return 1
      [[ "$(grep -Fxc 'GETWORK_RELAY_UPSTREAM_URL=http://127.0.0.1:18100' <<<"$relay_environment")" -eq 1 ]] ||
        return 1
      ! docker ps -a --filter 'name=^/henukit-getwork-mcp-1$' --format '{{.Names}}' |
        grep -q . || return 1
      ss -ltnH | awk -v endpoint="${expected_bridge}:18101" '$4 == endpoint { found = 1 } END { exit !found }'
      getwork_relay_ingress_is_restricted || return 1
      ;;
    1) return 0 ;;
    *) return 1 ;;
  esac
}

configure_getwork_relay_ingress() {
  local release_sha="$1"
  local state bridge_address henu_subnet input_chain output_chain iptables_bin
  if release_has_service "$release_sha" "getwork-mcp-relay"; then
    state=0
  else
    state=$?
  fi
  [[ "$state" -eq 0 ]] || {
    [[ "$state" -eq 1 || "$state" -eq 2 ]]
    return
  }
  iptables_bin="$(command -v iptables)" || return 1
  bridge_address="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')" ||
    return 1
  henu_subnet="$(docker network inspect henukit_default --format '{{(index .IPAM.Config 0).Subnet}}')" ||
    return 1
  [[ "$bridge_address" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ &&
     "$henu_subnet" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$ ]] || return 1
  input_chain=HENUKIT-GETWORK-INGRESS
  output_chain=HENUKIT-GETWORK-OUTPUT
  "$iptables_bin" -N "$input_chain" 2>/dev/null || true
  "$iptables_bin" -F "$input_chain"
  "$iptables_bin" -A "$input_chain" -s "$henu_subnet" -j ACCEPT
  "$iptables_bin" -A "$input_chain" -s "${bridge_address}/32" -j ACCEPT
  "$iptables_bin" -A "$input_chain" -j REJECT
  while "$iptables_bin" -C INPUT -d "${bridge_address}/32" -p tcp --dport 18101 -j "$input_chain" 2>/dev/null; do
    "$iptables_bin" -D INPUT -d "${bridge_address}/32" -p tcp --dport 18101 -j "$input_chain"
  done
  "$iptables_bin" -I INPUT 1 -d "${bridge_address}/32" -p tcp --dport 18101 -j "$input_chain"

  "$iptables_bin" -N "$output_chain" 2>/dev/null || true
  "$iptables_bin" -F "$output_chain"
  "$iptables_bin" -A "$output_chain" -m owner --uid-owner 0 -j ACCEPT
  "$iptables_bin" -A "$output_chain" -j REJECT
  while "$iptables_bin" -C OUTPUT -d "${bridge_address}/32" -p tcp --dport 18101 -j "$output_chain" 2>/dev/null; do
    "$iptables_bin" -D OUTPUT -d "${bridge_address}/32" -p tcp --dport 18101 -j "$output_chain"
  done
  "$iptables_bin" -I OUTPUT 1 -d "${bridge_address}/32" -p tcp --dport 18101 -j "$output_chain"
  getwork_relay_ingress_is_restricted
}

getwork_relay_ingress_is_restricted() {
  local bridge_address henu_subnet input_jump output_jump
  bridge_address="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}' 2>/dev/null)" ||
    return 1
  henu_subnet="$(docker network inspect henukit_default --format '{{(index .IPAM.Config 0).Subnet}}' 2>/dev/null)" ||
    return 1
  input_jump="-A INPUT -d ${bridge_address}/32 -p tcp -m tcp --dport 18101 -j HENUKIT-GETWORK-INGRESS"
  output_jump="-A OUTPUT -d ${bridge_address}/32 -p tcp -m tcp --dport 18101 -j HENUKIT-GETWORK-OUTPUT"
  [[ "$(iptables -S INPUT | awk '/^-A / { print; exit }')" == "$input_jump" ]] || return 1
  [[ "$(iptables -S OUTPUT | awk '/^-A / { print; exit }')" == "$output_jump" ]] || return 1
  [[ "$(iptables -S HENUKIT-GETWORK-INGRESS | grep -c '^-A ')" -eq 3 ]] || return 1
  iptables -C HENUKIT-GETWORK-INGRESS -s "$henu_subnet" -j ACCEPT || return 1
  iptables -C HENUKIT-GETWORK-INGRESS -s "${bridge_address}/32" -j ACCEPT || return 1
  iptables -C HENUKIT-GETWORK-INGRESS -j REJECT || return 1
  [[ "$(iptables -S HENUKIT-GETWORK-OUTPUT | grep -c '^-A ')" -eq 2 ]] || return 1
  iptables -C HENUKIT-GETWORK-OUTPUT -m owner --uid-owner 0 -j ACCEPT || return 1
  iptables -C HENUKIT-GETWORK-OUTPUT -j REJECT
}

getwork_relay_contract_is_live() (
  local release_sha="$1"
  local state expected_bridge token unauthorized tools_response_file sources_response_file
  if release_has_service "$release_sha" "getwork-mcp-relay"; then
    state=0
  else
    state=$?
  fi
  case "$state" in
    1) return 0 ;;
    0) ;;
    *) return 1 ;;
  esac
  expected_bridge="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}' 2>/dev/null)" ||
    return 1
  token="$(environment_value GETWORK_MCP_ACCESS_TOKEN)"
  [[ ${#token} -ge 32 && "$token" != *[[:space:]]* ]] || return 1
  unauthorized="$(curl --max-time 5 --noproxy '*' --silent --show-error --output /dev/null --write-out '%{http_code}' \
    --request POST --header 'Content-Type: application/json' \
    --data '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
    "http://${expected_bridge}:18101/mcp")" || return 1
  [[ "$unauthorized" == 401 ]] || return 1
  tools_response_file="$(mktemp "$state_root/.getwork-tools.XXXXXX")"
  sources_response_file="$(mktemp "$state_root/.getwork-sources.XXXXXX")"
  chmod 0600 "$tools_response_file" "$sources_response_file"
  trap 'rm -f -- "$tools_response_file" "$sources_response_file"' EXIT
  printf 'Authorization: Bearer %s\n' "$token" |
    curl --header @- --max-time 15 --max-filesize 1048576 --noproxy '*' \
    --fail --silent --show-error --output "$tools_response_file" \
    --request POST --header 'Content-Type: application/json' \
    --data '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
    "http://${expected_bridge}:18101/mcp" || return 1
  jq -e '.result.tools | map(.name) | sort == ["crawl_jobs","list_sources"]' \
    "$tools_response_file" >/dev/null || return 1
  printf 'Authorization: Bearer %s\n' "$token" |
    curl --header @- --max-time 15 --max-filesize 1048576 --noproxy '*' \
    --fail --silent --show-error --output "$sources_response_file" \
    --request POST --header 'Content-Type: application/json' \
    --data '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_sources","arguments":{}}}' \
    "http://${expected_bridge}:18101/mcp" || return 1
  jq -e '
    [.result.content[] | select(.type == "text") | .text | fromjson | .sources[].key] | sort
    == ["alibaba","baidu","beike","bytedance","ctrip","dewu","didi","jd","kuaishou","meituan","netease","pdd","tencent","tencentmusic","tongcheng","vipshop","xfusion","xiaohongshu"]
  ' "$sources_response_file" >/dev/null
)

getwork_relay_is_healthy() {
  local release_sha="$1"
  local state
  if release_has_service "$release_sha" "getwork-mcp-relay"; then
    state=0
  else
    state=$?
  fi
  case "$state" in
    0) container_is_healthy henukit-getwork-mcp-relay-1 ;;
    1) return 0 ;;
    *) return 1 ;;
  esac
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

legacy_runtime_is_absent() {
  local legacy_names
  legacy_names='^(henukit-)?(study-api|study-worker|quizcraft-api|quizcraft-web)(-|$)'
  ! docker ps -a --format '{{.Names}}' | grep -E "$legacy_names" >/dev/null
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
  legacy_runtime_is_absent || return 1
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
  getwork_relay_is_healthy "$release_sha" || return 1
  getwork_relay_contract_is_live "$release_sha" || return 1
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
  for ((attempt = 1; attempt <= active_release_attempts; attempt++)); do
    if verify_active_release "$release_sha"; then
      return 0
    fi
    if ((attempt < active_release_attempts)); then
      log "release $release_sha is not ready yet (attempt $attempt/$active_release_attempts)"
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
  complete_rollback_state_contract "$release_sha" ||
    die "could not complete rollback state contract for activated release $release_sha"
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

  if [[ "$refresh" != "yes" && ( -e "$marker" || -L "$marker" ) ]]; then
    trusted_root_file "$marker" "prepared backup marker"
    [[ -s "$marker" ]] || die "prepared backup marker is empty"
    [[ "$(file_mode "$marker")" == "600" || "$(file_mode "$marker")" == "400" ]] ||
      die "prepared backup marker must have mode 600 or 400"
    backup_file="$(tr -d '\r\n' < "$marker")"
    [[ "$backup_file" == "$backup_root/"* ]] || die "prepared backup path is outside HENUKIT_BACKUP_ROOT"
    [[ -s "$backup_file" && -s "$backup_file.sha256" && -s "$backup_file.meta" ]] ||
      die "prepared backup evidence is incomplete"
    trusted_root_file "$backup_file" "prepared Platform backup"
    trusted_root_file "$backup_file.sha256" "prepared Platform backup checksum"
    trusted_root_file "$backup_file.meta" "prepared backup metadata"
    account_backup_file="$(awk -F= '$1 == "account_portfolio_backup" { print substr($0, index($0, "=") + 1); exit }' "$backup_file.meta")"
    [[ "$account_backup_file" == "$backup_root/"* ]] || die "prepared Account Portfolio backup evidence is incomplete"
    [[ -s "$account_backup_file" && -s "$account_backup_file.sha256" ]] ||
      die "prepared Account Portfolio backup evidence is incomplete"
    trusted_root_file "$account_backup_file" "prepared Account Portfolio backup"
    trusted_root_file "$account_backup_file.sha256" "prepared Account Portfolio backup checksum"
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
  [[ -e "$approval" || -L "$approval" ]] || return 1
  trusted_root_file "$approval" "exact-SHA approval"
  approval_mode="$(file_mode "$approval")"
  approval_owner="$(file_owner "$approval")"
  [[ "$approval_mode" == "600" || "$approval_mode" == "400" ]] || return 1
  [[ "$approval_owner" == "0" ]] || return 1
  [[ "$(tr -d '\r\n' < "$approval")" == "$release_sha" ]]
}

validate_recovery_source_binding() {
  local release_sha="$1"
  local expected_identity="$2"
  local binding="$state_root/prepared/${release_sha}.artifact-source"
  local binding_mode
  trusted_root_file "$binding" "recovery artifact source binding"
  binding_mode="$(file_mode "$binding")"
  [[ "$binding_mode" == "600" || "$binding_mode" == "400" ]] ||
    die "recovery artifact source binding must have mode 600 or 400"
  [[ "$(tr -d '\r\n' < "$binding")" == "$expected_identity" ]] ||
    die "recovery approval is not bound to the selected artifact identity"
}

validate_recovery_baseline_binding() {
  local release_sha="$1"
  local expected_baseline="$2"
  local binding="$state_root/prepared/${release_sha}.recovery-baseline"
  local binding_mode
  trusted_root_file "$binding" "recovery baseline binding"
  binding_mode="$(file_mode "$binding")"
  [[ "$binding_mode" == "600" || "$binding_mode" == "400" ]] ||
    die "recovery baseline binding must have mode 600 or 400"
  [[ "$(tr -d '\r\n' < "$binding")" == "$expected_baseline" ]] ||
    die "recovery approval is not bound to baseline $expected_baseline"
}

consume_approval() {
  local release_sha="$1"
  local approval="$state_root/approvals/$release_sha"
  local consumed="$state_root/approvals/consumed/${release_sha}.$(date -u +%Y%m%dT%H%M%SZ).$$"
  release_is_approved "$release_sha" || die "exact-SHA approval disappeared before activation"
  mv "$approval" "$consumed"
  chmod 0400 "$consumed"
}

require_activation_disk_headroom() {
  local available_kib required_kib
  available_kib="$(df -Pk "$staging_root" | awk 'NR == 2 { print $4 }')"
  [[ "$available_kib" =~ ^[0-9]+$ ]] ||
    die "could not determine activation disk headroom for $staging_root"
  required_kib=$((minimum_activation_free_mib * 1024))
  ((available_kib >= required_kib)) ||
    die "activation requires at least ${minimum_activation_free_mib} MiB free on the artifact filesystem; approval remains unconsumed"
  log "activation disk headroom verified: $((available_kib / 1024)) MiB free"
}

github_branch_head() {
  gh api "repos/$repo/branches/$branch" --jq '.commit.sha'
}

rollback_contract_previous_sha=""
rollback_contract_candidate_sha=""
rollback_contract_compose_sha=""
rollback_contract_env_sha=""
rollback_contract_materials_sha=""
rollback_contract_materials_path_state=""
rollback_contract_deploy_webhook_state=""
rollback_contract_mode=""
pending_approved_release_sha=""
rollback_reconciliation_handled="0"

rollback_state_contract_matches() {
  local target="$1"
  local candidate_sha="$2"
  local previous_sha="$3"
  local path_state="$4"
  local deploy_state="$5"
  local contract_mode="$6"
  local compose_sha="$7"
  local env_sha="$8"
  local materials_sha="$9"
  [[ -f "$target" && ! -L "$target" ]] || return 1
  [[ "$(file_owner "$target")" == "$(id -u)" ]] || return 1
  [[ "$(file_mode "$target")" == "400" ]] || return 1
  [[ "$(grep -Fxc "candidate_sha=$candidate_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "previous_sha=$previous_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "materials_path_state=$path_state" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "deploy_webhook_state=$deploy_state" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "rollback_mode=$contract_mode" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "previous_compose_sha256=$compose_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "rollback_env_sha256=$env_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(grep -Fxc "materials_manifest_sha256=$materials_sha" "$target" || true)" == "1" ]] || return 1
  [[ "$(wc -l < "$target" | tr -d '[:space:]')" == "8" ]]
}

persist_rollback_state_contract() {
  local candidate_sha="$1"
  local previous_sha="$2"
  local path_state="$3"
  local deploy_state="$4"
  local contract_mode="$5"
  local compose_sha="$6"
  local env_sha="$7"
  local materials_sha="$8"
  local target="$state_root/rollback-contracts/pending/$candidate_sha"
  local incoming="$state_root/rollback-contracts/pending/.${candidate_sha}.$$"
  if [[ -e "$target" ]]; then
    rollback_state_contract_matches \
      "$target" "$candidate_sha" "$previous_sha" "$path_state" "$deploy_state" \
      "$contract_mode" "$compose_sha" "$env_sha" "$materials_sha"
    return
  fi
  umask 077
  {
    printf 'candidate_sha=%s\n' "$candidate_sha"
    printf 'previous_sha=%s\n' "$previous_sha"
    printf 'materials_path_state=%s\n' "$path_state"
    printf 'deploy_webhook_state=%s\n' "$deploy_state"
    printf 'rollback_mode=%s\n' "$contract_mode"
    printf 'previous_compose_sha256=%s\n' "$compose_sha"
    printf 'rollback_env_sha256=%s\n' "$env_sha"
    printf 'materials_manifest_sha256=%s\n' "$materials_sha"
  } > "$incoming"
  chmod 0400 "$incoming"
  mv "$incoming" "$target"
}

load_rollback_state_contract() {
  local candidate_sha="$1"
  local target="$state_root/rollback-contracts/pending/$candidate_sha"
  local previous_sha path_state deploy_state contract_mode compose_sha env_sha materials_sha
  previous_sha="$(awk -F= '$1 == "previous_sha" {print $2}' "$target" 2>/dev/null)"
  path_state="$(awk -F= '$1 == "materials_path_state" {print $2}' "$target" 2>/dev/null)"
  deploy_state="$(awk -F= '$1 == "deploy_webhook_state" {print $2}' "$target" 2>/dev/null)"
  contract_mode="$(awk -F= '$1 == "rollback_mode" {print $2}' "$target" 2>/dev/null)"
  compose_sha="$(awk -F= '$1 == "previous_compose_sha256" {print $2}' "$target" 2>/dev/null)"
  env_sha="$(awk -F= '$1 == "rollback_env_sha256" {print $2}' "$target" 2>/dev/null)"
  materials_sha="$(awk -F= '$1 == "materials_manifest_sha256" {print $2}' "$target" 2>/dev/null)"
  [[ "$previous_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$path_state" == "enabled" || "$path_state" == "disabled" ]] || return 1
  [[ "$deploy_state" == "active" || "$deploy_state" == "absent" ]] || return 1
  [[ "$contract_mode" == "normal" || "$contract_mode" == "degraded" ]] || return 1
  [[ "$compose_sha" =~ ^[0-9a-f]{64}$ && "$env_sha" =~ ^[0-9a-f]{64}$ &&
     "$materials_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  rollback_state_contract_matches \
    "$target" "$candidate_sha" "$previous_sha" "$path_state" "$deploy_state" \
    "$contract_mode" "$compose_sha" "$env_sha" "$materials_sha" || return 1
  rollback_contract_candidate_sha="$candidate_sha"
  rollback_contract_previous_sha="$previous_sha"
  rollback_contract_materials_path_state="$path_state"
  rollback_contract_deploy_webhook_state="$deploy_state"
  rollback_contract_mode="$contract_mode"
  rollback_contract_compose_sha="$compose_sha"
  rollback_contract_env_sha="$env_sha"
  rollback_contract_materials_sha="$materials_sha"
}

complete_rollback_state_contract() {
  local candidate_sha="$1"
  local pending="$state_root/rollback-contracts/pending/$candidate_sha"
  local completed timestamp sequence
  [[ -e "$pending" ]] || return 0
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  sequence=0
  while :; do
    completed="$state_root/rollback-contracts/completed/${candidate_sha}.${timestamp}.$$.$sequence"
    [[ -e "$completed" ]] || break
    sequence=$((sequence + 1))
  done
  mv "$pending" "$completed" || return 1
  if [[ "$pending_approved_release_sha" == "$candidate_sha" ]]; then
    pending_approved_release_sha=""
  fi
}

trusted_rollback_file() {
  local path="$1"
  [[ -f "$path" && -r "$path" && ! -L "$path" ]] || return 1
  [[ "$(file_owner "$path")" == "0" ]] || return 1
  (( (8#$(file_mode "$path") & 8#022) == 0 )) || return 1
  trusted_root_parent_chain "$path" "rollback file"
}

systemd_unit_load_state() {
  systemctl show -p LoadState --value "$1" 2>/dev/null
}

systemd_unit_active_state() {
  systemctl show -p ActiveState --value "$1" 2>/dev/null
}

materials_path_state() {
  systemctl is-enabled henukit-materials-webhook.path 2>/dev/null || true
}

deploy_webhook_state() {
  local load_state
  load_state="$(systemd_unit_load_state henukit-deploy-webhook.service)" || return 1
  if [[ "$load_state" == "not-found" ]]; then
    printf 'absent\n'
  elif [[ "$load_state" == "loaded" &&
          "$(systemd_unit_active_state henukit-deploy-webhook.service)" == "active" ]]; then
    printf 'active\n'
  else
    return 1
  fi
}

materials_control_plane_matches() {
  local expected_path_state="$1"
  local expected_deploy_webhook_state="$2"
  local unit load_state actual_path_state actual_deploy_webhook_state
  for unit in \
    henukit-materials-webhook.service \
    henukit-materials-runner.service \
    henukit-materials-webhook.path; do
    load_state="$(systemd_unit_load_state "$unit")" || return 1
    [[ "$load_state" == "loaded" ]] || return 1
  done
  [[ "$(systemd_unit_active_state henukit-materials-webhook.service)" == "active" ]] || return 1
  [[ "$(systemd_unit_active_state henukit-materials-runner.service)" == "inactive" ]] || return 1
  actual_path_state="$(materials_path_state)"
  [[ "$actual_path_state" == "$expected_path_state" ]] || return 1
  actual_deploy_webhook_state="$(deploy_webhook_state)" || return 1
  [[ "$actual_deploy_webhook_state" == "$expected_deploy_webhook_state" ]]
}

quiesce_materials_control_plane() {
  systemctl disable --now henukit-materials-webhook.path >/dev/null 2>&1 || return 1
  wait_for_materials_runner_inactive || return 1
  materials_control_plane_matches disabled "$rollback_contract_deploy_webhook_state"
}

wait_for_materials_runner_inactive() {
  local attempt
  for ((attempt = 1; attempt <= 30; attempt++)); do
    if [[ "$(systemd_unit_active_state henukit-materials-runner.service)" == "inactive" ]]; then
      return 0
    fi
    ((attempt < 30)) && sleep 2
  done
  return 1
}

restore_materials_control_plane() {
  systemctl daemon-reload || return 1
  wait_for_materials_runner_inactive || return 1
  systemctl restart henukit-materials-webhook.service || return 1
  if [[ "$rollback_contract_deploy_webhook_state" == "active" ]]; then
    [[ "$(systemd_unit_load_state henukit-deploy-webhook.service)" == "loaded" ]] || return 1
    systemctl restart henukit-deploy-webhook.service || return 1
  fi
  case "$rollback_contract_materials_path_state" in
    enabled) systemctl enable --now henukit-materials-webhook.path >/dev/null 2>&1 || return 1 ;;
    disabled) systemctl disable --now henukit-materials-webhook.path >/dev/null 2>&1 || return 1 ;;
    *) return 1 ;;
  esac
  wait_for_materials_runner_inactive || return 1
  materials_control_plane_matches \
    "$rollback_contract_materials_path_state" \
    "$rollback_contract_deploy_webhook_state"
}

capture_materials_operational_state() {
  local materials_path deploy_state
  materials_path="$(materials_path_state)"
  [[ "$materials_path" == "enabled" || "$materials_path" == "disabled" ]] || return 1
  deploy_state="$(deploy_webhook_state)" || return 1
  materials_control_plane_matches "$materials_path" "$deploy_state" || return 1
  rollback_contract_materials_path_state="$materials_path"
  rollback_contract_deploy_webhook_state="$deploy_state"
}

materials_manifest_is_valid() {
  local materials_dir="$1"
  (
    cd "$materials_dir"
    sha256sum --check --strict SHA256SUMS >/dev/null
  )
}

rollback_contract_is_ready() {
  local previous_sha="$1"
  local candidate_sha="$2"
  local previous_dir="$release_root/$previous_sha"
  local candidate_dir="$release_root/$candidate_sha"
  local previous_compose="$previous_dir/docker-compose.henukit.release.yml"
  local previous_materials="$previous_dir/materials-runtime/SHA256SUMS"
  local candidate_materials="$candidate_dir/materials-runtime/SHA256SUMS"
  trusted_rollback_file "$rollback_env_file" || return 1
  trusted_rollback_file "$previous_dir/RELEASE_SHA" || return 1
  trusted_rollback_file "$previous_compose" || return 1
  trusted_rollback_file "$previous_materials" || return 1
  trusted_rollback_file "$candidate_materials" || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA")" == "$previous_sha" ]] || return 1
  materials_manifest_is_valid "$previous_dir/materials-runtime" || return 1
  materials_manifest_is_valid "$candidate_dir/materials-runtime" || return 1
  cmp --silent "$previous_materials" "$candidate_materials" || return 1
}

capture_rollback_contract() {
  local previous_sha="$1"
  local candidate_sha="$2"
  local previous_dir="$release_root/$previous_sha"
  rollback_contract_is_ready "$previous_sha" "$candidate_sha" || return 1
  capture_materials_operational_state || return 1
  rollback_contract_previous_sha="$previous_sha"
  rollback_contract_candidate_sha="$candidate_sha"
  rollback_contract_compose_sha="$(sha256sum "$previous_dir/docker-compose.henukit.release.yml" | awk '{print $1}')"
  rollback_contract_env_sha="$(sha256sum "$rollback_env_file" | awk '{print $1}')"
  rollback_contract_materials_sha="$(sha256sum "$previous_dir/materials-runtime/SHA256SUMS" | awk '{print $1}')"
  rollback_contract_mode="normal"
  [[ "$rollback_contract_compose_sha" =~ ^[0-9a-f]{64}$ &&
     "$rollback_contract_env_sha" =~ ^[0-9a-f]{64}$ &&
     "$rollback_contract_materials_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  persist_rollback_state_contract \
    "$candidate_sha" "$previous_sha" \
    "$rollback_contract_materials_path_state" "$rollback_contract_deploy_webhook_state" \
    normal \
    "$rollback_contract_compose_sha" "$rollback_contract_env_sha" \
    "$rollback_contract_materials_sha"
}

degraded_rollback_contract_is_ready() {
  local previous_sha="$1"
  local candidate_sha="$2"
  local previous_dir="$release_root/$previous_sha"
  local candidate_dir="$release_root/$candidate_sha"
  [[ "$rollback_contract_mode" == "degraded" &&
     "$rollback_contract_previous_sha" == "$previous_sha" &&
     "$rollback_contract_candidate_sha" == "$candidate_sha" ]] || return 1
  trusted_rollback_file "$rollback_env_file" || return 1
  trusted_rollback_file "$previous_dir/RELEASE_SHA" || return 1
  trusted_rollback_file "$previous_dir/docker-compose.henukit.release.yml" || return 1
  trusted_rollback_file "$previous_dir/materials-runtime/SHA256SUMS" || return 1
  trusted_rollback_file "$candidate_dir/materials-runtime/SHA256SUMS" || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA")" == "$previous_sha" ]] || return 1
  materials_manifest_is_valid "$previous_dir/materials-runtime" || return 1
  materials_manifest_is_valid "$candidate_dir/materials-runtime" || return 1
  [[ "$(sha256sum "$previous_dir/docker-compose.henukit.release.yml" | awk '{print $1}')" == "$rollback_contract_compose_sha" ]] || return 1
  [[ "$(sha256sum "$rollback_env_file" | awk '{print $1}')" == "$rollback_contract_env_sha" ]] || return 1
  [[ "$(sha256sum "$previous_dir/materials-runtime/SHA256SUMS" | awk '{print $1}')" == "$rollback_contract_materials_sha" ]]
}

capture_degraded_rollback_contract() {
  local previous_sha="$1"
  local candidate_sha="$2"
  local previous_dir="$release_root/$previous_sha"
  rollback_contract_previous_sha="$previous_sha"
  rollback_contract_candidate_sha="$candidate_sha"
  rollback_contract_mode="degraded"
  capture_materials_operational_state || return 1
  rollback_contract_compose_sha="$(sha256sum "$previous_dir/docker-compose.henukit.release.yml" | awk '{print $1}')"
  rollback_contract_env_sha="$(sha256sum "$rollback_env_file" | awk '{print $1}')"
  rollback_contract_materials_sha="$(sha256sum "$previous_dir/materials-runtime/SHA256SUMS" | awk '{print $1}')"
  [[ "$rollback_contract_compose_sha" =~ ^[0-9a-f]{64}$ &&
     "$rollback_contract_env_sha" =~ ^[0-9a-f]{64}$ &&
     "$rollback_contract_materials_sha" =~ ^[0-9a-f]{64}$ ]] || return 1
  degraded_rollback_contract_is_ready "$previous_sha" "$candidate_sha" || return 1
  persist_rollback_state_contract \
    "$candidate_sha" "$previous_sha" \
    "$rollback_contract_materials_path_state" "$rollback_contract_deploy_webhook_state" \
    degraded \
    "$rollback_contract_compose_sha" "$rollback_contract_env_sha" \
    "$rollback_contract_materials_sha"
}

rollback_release() {
  local previous_sha="$1"
  local candidate_sha="$2"
  local previous_dir="$release_root/$previous_sha"
  local previous_compose="$previous_dir/docker-compose.henukit.release.yml"
  local portal_deployed_at
  local -a rollback_compose
  [[ "$previous_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA" 2>/dev/null)" == "$previous_sha" ]] ||
    return 1
  [[ "$rollback_contract_previous_sha" == "$previous_sha" &&
     "$rollback_contract_candidate_sha" == "$candidate_sha" &&
     "$rollback_contract_mode" == "normal" ]] || return 1
  rollback_contract_is_ready "$previous_sha" "$candidate_sha" || return 1
  [[ "$(sha256sum "$previous_compose" | awk '{print $1}')" == "$rollback_contract_compose_sha" ]] || return 1
  [[ "$(sha256sum "$rollback_env_file" | awk '{print $1}')" == "$rollback_contract_env_sha" ]] || return 1
  [[ "$(sha256sum "$previous_dir/materials-runtime/SHA256SUMS" | awk '{print $1}')" == "$rollback_contract_materials_sha" ]] || return 1
  if verify_active_release "$previous_sha"; then
    restore_materials_control_plane || return 1
    log "release $previous_sha remained active; rollback needs no runtime replacement"
    return 0
  fi
  log "rolling back to release $previous_sha"
  export RELEASE_SHA="$previous_sha"
  export PORTAL_VERSION="$previous_sha"
  portal_deployed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  [[ "$portal_deployed_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] || return 1
  export PORTAL_DEPLOYED_AT="$portal_deployed_at"
  rollback_compose=(docker compose --env-file "$rollback_env_file" -f "$previous_compose")
  "${rollback_compose[@]}" config --quiet || return 1
  "${rollback_compose[@]}" up -d --remove-orphans || return 1
  restore_materials_control_plane || return 1
  wait_for_active_release "$previous_sha"
}

rollback_release_is_ready() {
  local previous_sha="$1"
  local previous_dir="$release_root/$previous_sha"
  [[ "$previous_sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$(tr -d '[:space:]' < "$previous_dir/RELEASE_SHA" 2>/dev/null)" == "$previous_sha" ]] ||
    return 1
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
    restore_materials_control_plane ||
      die "$phase failed; degraded baseline returned but materials operational state did not restore"
    complete_rollback_state_contract "$release_sha" ||
      die "$phase failed; degraded baseline returned but its state contract did not complete"
    ensure_degraded_recovery_audit \
      "$release_sha" "$previous_sha" restored_known_degraded_baseline restored
    die "$phase failed; restored known degraded baseline $previous_sha; retry requires a new candidate SHA"
  fi
  rollback_release "$previous_sha" "$release_sha" ||
    die "$phase failed and rollback to $previous_sha also failed"
  complete_rollback_state_contract "$release_sha" ||
    die "$phase failed; rollback succeeded but its state contract did not complete"
  die "$phase failed; rolled back to $previous_sha"
}

deploy_release() {
  local run_id="$1"
  local release_sha="$2"
  local run_url="$3"
  local artifact_override="${4:-}"
  local run_attempt="${5:-}"
  local artifact_dir runtime_archive release_dir release_incoming
  local image helper previous_sha activation_status activation_record artifact_identity

  [[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "release source returned an invalid SHA"
  if [[ -n "$recovery_baseline_sha" && "$recovery_baseline_sha" == "$release_sha" ]]; then
    die "recovery baseline and candidate SHA must differ"
  fi
  if [[ -n "$pending_approved_release_sha" &&
        "$pending_approved_release_sha" != "$release_sha" ]]; then
    die "approved pending release $pending_approved_release_sha must finish or be withdrawn before release $release_sha"
  fi

  if [[ -r "$release_root/$release_sha/docker-compose.henukit.release.yml" ]]; then
    configure_getwork_relay_ingress "$release_sha" ||
      die "active release relay ingress could not be restricted to the HENUKit network"
  fi
  if active_release_matches "$release_sha"; then
    activation_record=""
    if [[ -f "$state_root/last-activated-sha" ]]; then
      activation_record="$(tr -d '[:space:]' < "$state_root/last-activated-sha")"
    fi
    if [[ "$activation_record" != "$release_sha" &&
          -e "$state_root/rollback-contracts/pending/$release_sha" ]]; then
      load_rollback_state_contract "$release_sha" ||
        die "active release has an invalid pending rollback state contract"
      restore_materials_control_plane ||
        die "active release could not restore its captured materials operational state"
    fi
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
        -e "$state_root/degraded-recoveries/${release_sha}.authorized" &&
        "$pending_approved_release_sha" != "$release_sha" ]]; then
    degraded_recovery_audit_matches \
      "$state_root/degraded-recoveries/${release_sha}.authorized" \
      "$release_sha" "$recovery_baseline_sha" authorized "*" ||
      die "existing degraded recovery authorization audit is invalid"
    validate_degraded_baseline_authority "$recovery_baseline_sha"
    degraded_baseline_matches "$recovery_baseline_sha" ||
      die "prior degraded recovery is neither active nor restored to its exact baseline"
    ensure_degraded_recovery_audit \
      "$release_sha" "$recovery_baseline_sha" restored_known_degraded_baseline restored
    die "prior degraded recovery attempt converged to the known degraded baseline; retry requires a new candidate SHA"
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
    tar --no-same-owner -xzf "$runtime_archive" -C "$release_incoming"
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
  if [[ -z "$artifact_override" ]]; then
    verify_workflow_continuation_gate_receipt "$artifact_dir" "$release_dir" "$release_sha"
  fi
  verify_account_boundary_manifest "$release_dir" "$release_sha"

  if ! release_is_approved "$release_sha"; then
    prepared_backup_file=""
    prepare_backup "$release_sha"
    log "release $release_sha prepared with verified backup $prepared_backup_file"
    log "release $release_sha awaits exact-SHA approval at $state_root/approvals/$release_sha"
    return
  fi

  if [[ -n "$recovery_baseline_sha" ]]; then
    if [[ -n "$artifact_override" ]]; then
      artifact_identity="local:$(sha256sum "$artifact_dir/henukit-release-${release_sha}.manifest" | awk '{print $1}')"
    else
      [[ "$run_attempt" =~ ^[1-9][0-9]*$ ]] || die "workflow run attempt is invalid"
      artifact_identity="actions:${run_id}:${run_attempt}"
    fi
    validate_recovery_source_binding "$release_sha" "$artifact_identity"
    validate_recovery_baseline_binding "$release_sha" "$recovery_baseline_sha"
    previous_sha="$recovery_baseline_sha"
    validate_degraded_baseline_authority "$previous_sha"
    degraded_baseline_matches "$previous_sha" ||
      die "declared degraded baseline does not match the current release link and exact image set"
    if rollback_release_is_ready "$previous_sha"; then
      die "declared recovery baseline is healthy; use the normal rollback-protected activation path"
    fi
    capture_degraded_rollback_contract "$previous_sha" "$release_sha" ||
      die "declared recovery baseline could not bind its materials rollback contract"
    log "authorized degraded-baseline recovery from exact release $previous_sha"
  else
    previous_sha="$(current_release_sha 2>/dev/null || true)"
    rollback_release_is_ready "$previous_sha" ||
      die "no healthy fixed-SHA rollback release is ready; refusing production activation"
    capture_rollback_contract "$previous_sha" "$release_sha" ||
      die "candidate and previous releases do not satisfy the exact rollback contract; refusing production activation"
  fi
  prepared_backup_file=""
  if [[ -n "$recovery_baseline_sha" &&
        "$pending_approved_release_sha" == "$release_sha" &&
        -e "$state_root/degraded-recoveries/${release_sha}.authorized" ]]; then
    prepare_backup "$release_sha"
  else
    prepare_backup "$release_sha" yes
  fi
  log "release $release_sha has a fresh verified pre-activation backup $prepared_backup_file"
  [[ "$(github_branch_head)" == "$release_sha" ]] ||
    die "GitHub branch head changed during preparation; refusing stale activation"
  if [[ -n "$recovery_baseline_sha" ]]; then
    ensure_degraded_recovery_audit \
      "$release_sha" "$previous_sha" authorized authorized "$prepared_backup_file"
  fi
  require_activation_disk_headroom
  configure_getwork_relay_ingress "$release_sha" ||
    die "candidate relay ingress could not be restricted to the HENUKit network"
  consume_approval "$release_sha"
  for image in "${load_images[@]}"; do
    log "loading ${image}:${release_sha}"
    gzip -dc "$artifact_dir/${image}-${release_sha}.docker.tar.gz" | docker load >/dev/null
  done

  if ! quiesce_materials_control_plane; then
    restore_materials_control_plane ||
      die "materials quiesce failed and the captured operational state could not be restored"
    die "materials quiesce failed; restored the captured operational state"
  fi

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
  if ! restore_materials_control_plane; then
    fail_candidate_activation "materials operational-state restoration" "$previous_sha" "$release_sha"
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

reconcile_pending_rollback_contract() {
  local contract contract_candidate candidate_sha activation_record pending_count candidate_ready
  rollback_reconciliation_handled="0"
  pending_count=0
  contract=""
  for contract_candidate in "$state_root/rollback-contracts/pending/"*; do
    [[ -e "$contract_candidate" ]] || continue
    contract="$contract_candidate"
    pending_count=$((pending_count + 1))
  done
  ((pending_count <= 1)) || die "multiple pending rollback contracts require operator reconciliation"
  ((pending_count == 1)) || return 0

  candidate_sha="$(basename "$contract")"
  [[ "$candidate_sha" =~ ^[0-9a-f]{40}$ ]] || die "pending rollback contract has an invalid candidate SHA"
  activation_record=""
  if [[ -f "$state_root/last-activated-sha" ]]; then
    activation_record="$(tr -d '[:space:]' < "$state_root/last-activated-sha")"
  fi
  if [[ "$activation_record" == "$candidate_sha" ]]; then
    complete_rollback_state_contract "$candidate_sha" ||
      die "activated release has an incomplete rollback contract"
    rollback_reconciliation_handled="1"
    return 0
  fi
  if release_is_approved "$candidate_sha"; then
    # The exact approval still exists, so activation has not consumed it yet.
    pending_approved_release_sha="$candidate_sha"
    return 0
  fi

  load_rollback_state_contract "$candidate_sha" ||
    die "interrupted release has an invalid rollback state contract"
  if [[ "$rollback_contract_mode" == "degraded" ]]; then
    degraded_recovery_audit_matches \
      "$state_root/degraded-recoveries/${candidate_sha}.authorized" \
      "$candidate_sha" "$rollback_contract_previous_sha" authorized "*" ||
      die "interrupted degraded recovery has no matching immutable authorization audit"
  fi
  if active_release_matches "$candidate_sha"; then
    candidate_ready=0
    if restore_materials_control_plane && wait_for_active_release "$candidate_sha"; then
      if ! release_uses_account_portfolio "$candidate_sha" ||
         grant_account_operator_permissions "$candidate_sha"; then
        candidate_ready=1
      fi
    fi
    if [[ "$candidate_ready" == "1" ]]; then
      if [[ "$rollback_contract_mode" == "degraded" ]]; then
        ensure_degraded_recovery_audit \
          "$candidate_sha" "$rollback_contract_previous_sha" activated activated
      fi
      record_activation "$candidate_sha"
      log "interrupted active release $candidate_sha converged from its persisted rollback contract"
      rollback_reconciliation_handled="1"
      return 0
    fi
    log "interrupted active release $candidate_sha failed convergence; restoring its persisted baseline"
  fi

  if [[ "$rollback_contract_mode" == "degraded" ]]; then
    validate_degraded_baseline_authority "$rollback_contract_previous_sha"
    degraded_rollback_contract_is_ready \
      "$rollback_contract_previous_sha" "$candidate_sha" ||
      die "interrupted degraded recovery contract no longer matches trusted disk state"
    restore_degraded_baseline "$rollback_contract_previous_sha" ||
      die "interrupted degraded recovery could not restore its exact baseline"
    restore_materials_control_plane ||
      die "interrupted degraded recovery could not restore materials operational state"
    ensure_degraded_recovery_audit \
      "$candidate_sha" "$rollback_contract_previous_sha" \
      restored_known_degraded_baseline restored
  else
    rollback_release "$rollback_contract_previous_sha" "$candidate_sha" ||
      die "interrupted release could not restore its persisted rollback contract"
  fi
  complete_rollback_state_contract "$candidate_sha" ||
    die "interrupted release rollback succeeded but its contract did not complete"
  if [[ "$rollback_contract_mode" == "degraded" ]]; then
    die "interrupted degraded recovery $candidate_sha reconciled to $rollback_contract_previous_sha; retry requires a new candidate SHA"
  fi
  die "interrupted release $candidate_sha reconciled to $rollback_contract_previous_sha; issue a new exact-SHA approval before retrying"
}

check_once() {
  local run_row run_id run_attempt release_sha run_status run_conclusion run_url branch_head
  reconcile_pending_rollback_contract
  [[ "$rollback_reconciliation_handled" == "0" ]] || return 0
  run_row="$(
    gh run list \
      --repo "$repo" \
      --workflow "$workflow" \
      --branch "$branch" \
      --event push \
      --limit 20 \
      --json databaseId,attempt,headSha,status,conclusion,url \
      --jq 'first(.[]) | [(.databaseId|tostring),(.attempt|tostring),.headSha,.status,.conclusion,.url] | @tsv'
  )"
  if [[ -z "$run_row" ]]; then
    log "no completed successful $workflow run found on $branch"
    return
  fi
  IFS=$'\t' read -r run_id run_attempt release_sha run_status run_conclusion run_url <<<"$run_row"
  [[ "$run_id" =~ ^[0-9]+$ ]] || die "GitHub returned an invalid workflow run id"
  [[ "$run_attempt" =~ ^[1-9][0-9]*$ ]] || die "GitHub returned an invalid workflow run attempt"
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
  deploy_release "$run_id" "$release_sha" "$run_url" "" "$run_attempt"
}

check_local_artifacts() {
  local branch_head
  reconcile_pending_rollback_contract
  [[ "$rollback_reconciliation_handled" == "0" ]] || return 0
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
