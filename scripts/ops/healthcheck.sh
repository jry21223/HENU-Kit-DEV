#!/usr/bin/env sh
set -eu

API_HEALTH_URL="${API_HEALTH_URL:-http://localhost:8080/healthz}"
WEB_URL="${WEB_URL:-http://localhost:3000/health}"
ADMIN_URL="${ADMIN_URL:-http://localhost:5173}"

check_url() {
  name="$1"
  url="$2"
  code="$(curl -fsS -o /dev/null -w '%{http_code}' "$url")"
  case "$code" in
    200|204|301|302)
      echo "$name ok ($code) $url"
      ;;
    *)
      echo "$name unhealthy ($code) $url" >&2
      exit 1
      ;;
  esac
}

check_url "api" "$API_HEALTH_URL"
check_url "web" "$WEB_URL"
check_url "admin" "$ADMIN_URL"
