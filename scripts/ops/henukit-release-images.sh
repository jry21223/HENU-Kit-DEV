#!/usr/bin/env bash
# Canonical HENU Kit release-image inventory. Keep runtime roles together with
# build metadata so CI, an amd64 local builder, and the production watcher
# cannot silently disagree about an artifact set.
set -Eeuo pipefail

program="henukit-release-images"

release_names=(
  console
  console-gateway
  platform-core
  platform-mail-worker
  platform-smtp-provider
  portal
  portal-summary
  portal-api
  account-portfolio
  notice
  notice-worker
  food
  food-mcp
  library
  career-opportunities
  career-mcp
  portal-gateway
  quizcraft
)
release_images=(
  henukit-console
  henukit-console-gateway
  henukit-platform-core
  henukit-platform-mail-worker
  henukit-platform-smtp-provider
  henukit-portal
  henukit-portal-summary
  henukit-portal-api
  henukit-account-portfolio
  henukit-notice
  henukit-notice-worker
  henukit-food
  henukit-food-mcp
  henukit-library
  henukit-career-opportunities
  henukit-career-mcp
  henukit-portal-gateway
  henukit-quizcraft
)
release_services=(
  console
  console-gateway
  platform-core
  platform-mail-worker
  platform-smtp-provider
  portal
  portal-summary
  portal-api
  account-portfolio
  notice
  notice-worker
  food
  food-mcp
  library
  career-opportunities
  career-mcp
  portal-gateway
  quizcraft
)
release_roles=(
  baseline
  baseline
  baseline
  baseline
  baseline
  baseline
  baseline
  baseline
  conditional
  conditional
  conditional
  conditional
  conditional
  conditional
  conditional
  conditional
  baseline
  conditional
)
release_contexts=(
  .
  services/console-gateway
  services/platform-core
  services/platform-core
  services/platform-core
  .
  services/portal-summary
  services/portal-api
  services/account-portfolio
  services/notice
  services/notice
  services/food
  services/food-mcp
  services/library
  services/career-opportunities
  services/career-mcp
  services/portal-gateway
  products/quizcraft/go-service
)
release_dockerfiles=(
  apps/console/Dockerfile
  services/console-gateway/Dockerfile
  services/platform-core/Dockerfile
  services/platform-core/Dockerfile.worker
  services/platform-core/Dockerfile.smtp-provider
  apps/portal/Dockerfile
  services/portal-summary/Dockerfile
  services/portal-api/Dockerfile
  services/account-portfolio/Dockerfile
  services/notice/Dockerfile
  services/notice/Dockerfile.worker
  services/food/Dockerfile
  services/food-mcp/Dockerfile
  services/library/Dockerfile
  services/career-opportunities/Dockerfile
  services/career-mcp/Dockerfile
  services/portal-gateway/Dockerfile
  products/quizcraft/go-service/Dockerfile
)
# The Portal bake flags below are deliberately 1: this inventory describes the
# #166 cutover release build only (see docs/operations/practice-wiring-matrix.md,
# ADR-0036). Every default in the repo stays 0 (fail-closed); a release built
# from this inventory MUST be deployed with the server-side gates
# (PORTAL_ENABLE_QUIZCRAFT_CATALOG / PORTAL_ENABLE_QUIZCRAFT_V2_READS /
# PORTAL_PRACTICE_COMMANDS_ENABLED) enabled in the same bundle, or the browser
# surfaces render honest 404/503s — never a mock/legacy fallback.
release_build_args=(
  $'VITE_BASE_PATH=/\nVITE_QUIZCRAFT_WORKSHOP_URL='
  ""
  ""
  ""
  ""
  $'NEXT_PUBLIC_PORTAL_GATEWAY_URL=\nNEXT_PUBLIC_PORTAL_GATEWAY_BASE_URL=/api\nNEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY=1\nNEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_CATALOG=1\nNEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS=1'
  ""
  ""
  ""
  ""
  ""
  ""
  ""
  ""
  ""
  ""
  ""
  ""
)

usage() {
  cat >&2 <<'EOF'
usage: henukit-release-images.sh <command> [arguments]

Commands:
  --check                         Validate the canonical inventory.
  --records                       Print name, image, compose service, role.
  --artifact-images               Print every required image archive name.
  --load-images                   Print every image the runtime must load.
  --baseline-images               Print images always expected to run.
  --conditional-services          Print service/image pairs expected when present.
  --github-matrix                 Emit a GitHub Actions matrix JSON document.
  --field <name> <field>          Print image, context, dockerfile, or build_args.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

record_count() {
  printf '%s\n' "${#release_names[@]}"
}

record_index() {
  local requested="$1"
  local index
  for ((index = 0; index < ${#release_names[@]}; index++)); do
    if [[ "${release_names[$index]}" == "$requested" ]]; then
      printf '%s\n' "$index"
      return 0
    fi
  done
  return 1
}

record_field() {
  local name="$1"
  local field="$2"
  local index
  index="$(record_index "$name")" || die "unknown release image name: $name"
  case "$field" in
    image) printf '%s' "${release_images[$index]}" ;;
    context) printf '%s' "${release_contexts[$index]}" ;;
    dockerfile) printf '%s' "${release_dockerfiles[$index]}" ;;
    build_args) printf '%s' "${release_build_args[$index]}" ;;
    service) printf '%s' "${release_services[$index]}" ;;
    role) printf '%s' "${release_roles[$index]}" ;;
    *) die "unsupported field: $field" ;;
  esac
}

json_quote() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '"%s"' "$value"
}

validate_inventory() {
  local expected_count index other name image service role context dockerfile build_args
  expected_count="${#release_names[@]}"
  [[ "$expected_count" -eq "${#release_images[@]}" &&
     "$expected_count" -eq "${#release_services[@]}" &&
     "$expected_count" -eq "${#release_roles[@]}" &&
     "$expected_count" -eq "${#release_contexts[@]}" &&
     "$expected_count" -eq "${#release_dockerfiles[@]}" &&
     "$expected_count" -eq "${#release_build_args[@]}" ]] ||
    die "all inventory columns must have the same length"
  [[ "$expected_count" -gt 0 ]] || die "inventory must not be empty"

  for ((index = 0; index < expected_count; index++)); do
    name="${release_names[$index]}"
    image="${release_images[$index]}"
    service="${release_services[$index]}"
    role="${release_roles[$index]}"
    context="${release_contexts[$index]}"
    dockerfile="${release_dockerfiles[$index]}"
    build_args="${release_build_args[$index]}"
    [[ "$name" =~ ^[a-z0-9][a-z0-9-]*$ ]] || die "invalid image record name: $name"
    [[ "$image" =~ ^henukit-[a-z0-9][a-z0-9-]*$ ]] || die "invalid image name: $image"
    [[ "$service" =~ ^[a-z0-9][a-z0-9-]*$ ]] || die "invalid Compose service: $service"
    [[ "$role" == "baseline" || "$role" == "conditional" ]] || die "invalid runtime role: $role"
    [[ "$context" == "." || ( "$context" != /* && "$context" != *".."* ) ]] || die "invalid build context: $context"
    [[ "$dockerfile" != /* && "$dockerfile" != *".."* && "$dockerfile" == *"/Dockerfile"* ]] ||
      die "invalid Dockerfile path: $dockerfile"
    [[ "$build_args" != *$'\r'* ]] || die "build arguments must use LF line endings"
    for ((other = 0; other < index; other++)); do
      [[ "$name" != "${release_names[$other]}" ]] || die "duplicate record name: $name"
      [[ "$image" != "${release_images[$other]}" ]] || die "duplicate image: $image"
      [[ "$service" != "${release_services[$other]}" ]] || die "duplicate Compose service: $service"
    done
  done
}

emit_records() {
  local index
  for ((index = 0; index < ${#release_names[@]}; index++)); do
    printf '%s\t%s\t%s\t%s\n' \
      "${release_names[$index]}" \
      "${release_images[$index]}" \
      "${release_services[$index]}" \
      "${release_roles[$index]}"
  done
}

emit_images() {
  local role_filter="${1:-}"
  local index
  for ((index = 0; index < ${#release_names[@]}; index++)); do
    if [[ -z "$role_filter" || "${release_roles[$index]}" == "$role_filter" ]]; then
      printf '%s\n' "${release_images[$index]}"
    fi
  done
}

emit_conditional_services() {
  local index
  for ((index = 0; index < ${#release_names[@]}; index++)); do
    if [[ "${release_roles[$index]}" == "conditional" ]]; then
      printf '%s\t%s\n' "${release_services[$index]}" "${release_images[$index]}"
    fi
  done
}

emit_github_matrix() {
  local index separator=""
  printf '{"include":['
  for ((index = 0; index < ${#release_names[@]}; index++)); do
    printf '%s{"name":' "$separator"
    json_quote "${release_names[$index]}"
    printf ',"image":'
    json_quote "${release_images[$index]}"
    printf ',"context":'
    json_quote "${release_contexts[$index]}"
    printf ',"dockerfile":'
    json_quote "${release_dockerfiles[$index]}"
    printf ',"build_args":'
    json_quote "${release_build_args[$index]}"
    printf '}'
    separator=,
  done
  printf ']}\n'
}

validate_inventory

case "${1:-}" in
  --check)
    [[ $# -eq 1 ]] || { usage; exit 64; }
    ;;
  --records)
    [[ $# -eq 1 ]] || { usage; exit 64; }
    emit_records
    ;;
  --artifact-images|--load-images)
    [[ $# -eq 1 ]] || { usage; exit 64; }
    emit_images
    ;;
  --baseline-images)
    [[ $# -eq 1 ]] || { usage; exit 64; }
    emit_images baseline
    ;;
  --conditional-services)
    [[ $# -eq 1 ]] || { usage; exit 64; }
    emit_conditional_services
    ;;
  --github-matrix)
    [[ $# -eq 1 ]] || { usage; exit 64; }
    emit_github_matrix
    ;;
  --field)
    [[ $# -eq 3 ]] || { usage; exit 64; }
    record_field "$2" "$3"
    ;;
  *)
    usage
    exit 64
    ;;
esac
