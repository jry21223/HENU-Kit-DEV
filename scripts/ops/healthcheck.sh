#!/usr/bin/env sh
set -eu

API_HEALTH_URL="${API_HEALTH_URL:-http://localhost:8080/readyz}"
WORKER_READY_URL="${WORKER_READY_URL:-}"
WEB_URL="${WEB_URL:-http://localhost:3000/health}"
ADMIN_URL="${ADMIN_URL:-http://localhost:5173}"
CHECK_SECURITY_HEADERS="${CHECK_SECURITY_HEADERS:-false}"

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

check_security_headers() {
  name="$1"
  url="$2"
  headers="$(curl -fsS -D - -o /dev/null "$url")"
  printf '%s\n' "$headers" | grep -qi '^strict-transport-security:' || {
    echo "$name missing Strict-Transport-Security: $url" >&2
    exit 1
  }
  printf '%s\n' "$headers" | grep -qi '^content-security-policy:' || {
    echo "$name missing Content-Security-Policy: $url" >&2
    exit 1
  }
  printf '%s\n' "$headers" | grep -qi '^x-content-type-options: *nosniff' || {
    echo "$name missing X-Content-Type-Options nosniff: $url" >&2
    exit 1
  }
  echo "$name security headers ok $url"
}

check_url "api" "$API_HEALTH_URL"
if [ -n "$WORKER_READY_URL" ]; then
  check_url "worker" "$WORKER_READY_URL"
fi
check_url "web" "$WEB_URL"
check_url "admin" "$ADMIN_URL"

if [ "$CHECK_SECURITY_HEADERS" = "true" ]; then
  check_security_headers "web" "$WEB_URL"
  check_security_headers "admin" "$ADMIN_URL"
fi
