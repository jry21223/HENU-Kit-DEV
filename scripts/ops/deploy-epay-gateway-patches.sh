#!/usr/bin/env bash
# Build and test an EasyPay gateway candidate before any interruption. In
# execute mode the tested directory is swapped in as one unit and private
# environment/order state is copied from the stopped original directory.
set -Eeuo pipefail

program="deploy-epay-gateway-patches"
candidate=""

usage() {
  cat >&2 <<'EOF'
usage: deploy-epay-gateway-patches.sh <gateway-dir> <patch-dir> --check|--execute

--check builds an isolated candidate, applies 0001 -> 0002 -> 0003, and runs
npm test without touching the service. --execute performs the same checks,
then stops the service and atomically activates that already-tested candidate.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

cleanup() {
  if [[ -n "${candidate:-}" && -d "$candidate" && "$candidate" == */.epay-gateway-candidate.* ]]; then
    rm -rf -- "$candidate"
  fi
}
trap cleanup EXIT

if [[ $# -ne 3 || ( "$3" != "--check" && "$3" != "--execute" ) ]]; then
  usage
  exit 64
fi

gateway_dir="$(cd "$1" 2>/dev/null && pwd -P)" || die "gateway directory does not exist"
patch_dir="$(cd "$2" 2>/dev/null && pwd -P)" || die "patch directory does not exist"
mode="$3"
gateway_parent="$(dirname "$gateway_dir")"
gateway_name="$(basename "$gateway_dir")"
service="${EPAY_SERVICE:-epay-gateway.service}"
health_url="${EPAY_HEALTH_URL:-http://127.0.0.1:9219/health}"
backup_root="${EPAY_BACKUP_ROOT:-$gateway_parent/epay-gateway-backups}"

[[ "$gateway_dir" != "/" && "$gateway_name" != "." && "$gateway_name" != ".." ]] ||
  die "gateway directory is unsafe"
[[ "$patch_dir" != "/" ]] || die "patch directory is unsafe"
[[ "$health_url" == http://127.0.0.1:*/* ]] || die "EPAY_HEALTH_URL must use loopback HTTP"
for command in npm patch sha256sum; do
  command -v "$command" >/dev/null 2>&1 || die "$command is required"
done

patches=(
  "$patch_dir/0001-henukit-query-and-notify-outbox.patch"
  "$patch_dir/0002-henukit-close-refund-and-response-verification.patch"
  "$patch_dir/0003-henukit-private-checkout-handle.patch"
)
for patch_file in "${patches[@]}"; do
  [[ -f "$patch_file" && -r "$patch_file" && ! -L "$patch_file" ]] ||
    die "required patch is unavailable: $(basename "$patch_file")"
done
for required in package.json package-lock.json config.js db.js server.js lib test; do
  [[ -e "$gateway_dir/$required" && ! -L "$gateway_dir/$required" ]] ||
    die "gateway baseline is missing $required"
done

expected_manifest=""
for patch_file in "${patches[@]}"; do
  expected_manifest+="$(sha256sum "$patch_file" | awk '{print $1}')  $(basename "$patch_file")"$'\n'
done

candidate="$(mktemp -d "$gateway_parent/.epay-gateway-candidate.XXXXXX")"
chmod 0700 "$candidate"
for source in package.json package-lock.json config.js db.js server.js lib test; do
  cp -a "$gateway_dir/$source" "$candidate/"
done

if [[ -f "$gateway_dir/.henukit-patches.sha256" ]] &&
   [[ "$(cat "$gateway_dir/.henukit-patches.sha256")" == "${expected_manifest%$'\n'}" ]]; then
  printf '%s: exact patch set is already present; revalidating tests\n' "$program"
else
  (
    cd "$candidate"
    for patch_file in "${patches[@]}"; do
      patch --batch -p1 < "$patch_file"
    done
  )
fi
printf '%s' "$expected_manifest" > "$candidate/.henukit-patches.sha256"
chmod 0400 "$candidate/.henukit-patches.sha256"
(
  cd "$candidate"
  npm ci --ignore-scripts
  npm test
)
printf '%s: candidate verification passed\n' "$program"

if [[ "$mode" == "--check" ]]; then
  exit 0
fi

gateway_env_value() {
  local key="$1"
  local env_file="$gateway_dir/.env"
  local count value
  [[ -f "$env_file" && -r "$env_file" && ! -L "$env_file" ]] || return 1
  count="$(grep -Ec "^[[:space:]]*${key}[[:space:]]*=" "$env_file" || true)"
  [[ "$count" == "1" ]] || return 1
  value="$(grep -E "^[[:space:]]*${key}[[:space:]]*=" "$env_file")"
  value="${value#*=}"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

henukit_enabled="$(gateway_env_value HENUKIT_EPAY_ENABLED || true)"
henukit_pid="$(gateway_env_value HENUKIT_EPAY_PID || true)"
henukit_key="$(gateway_env_value HENUKIT_EPAY_KEY || true)"
henukit_notify_urls="$(gateway_env_value HENUKIT_EPAY_NOTIFY_URLS || true)"
henukit_return_origins="$(gateway_env_value HENUKIT_EPAY_RETURN_ORIGINS || true)"
[[ "$henukit_enabled" == "0" || "$henukit_enabled" == "1" ]] || die "HENU EasyPay tenant enabled flag must be 0 or 1"
[[ "$henukit_pid" =~ ^[A-Za-z0-9_-]{1,64}$ ]] || die "HENU EasyPay tenant PID is missing or invalid"
[[ ${#henukit_key} -ge 16 && "$henukit_key" != *[[:space:]]* ]] || die "HENU EasyPay tenant key is missing or invalid"
[[ ! "$henukit_key" =~ (replace|example|changeme|test-secret) ]] || die "HENU EasyPay tenant key is a deployment placeholder"
[[ ",$henukit_notify_urls," == *,https://henukit.cn/api/v1/payment-providers/easypay/notifications,* ]] ||
  die "HENU EasyPay tenant callback allowlist is incomplete"
[[ ",$henukit_return_origins," == *,https://henukit.cn,* ]] ||
  die "HENU EasyPay tenant return-origin allowlist is incomplete"

awk '
  BEGIN { changed=0 }
  /^[[:space:]]*HENUKIT_EPAY_ENABLED[[:space:]]*=/ { print "HENUKIT_EPAY_ENABLED=1"; changed++; next }
  { print }
  END { if (changed != 1) exit 42 }
' "$gateway_dir/.env" > "$candidate/.env" || die "could not prepare the enabled HENU tenant environment"
chmod 0600 "$candidate/.env"

command -v systemctl >/dev/null 2>&1 || die "systemctl is required for execution"
command -v curl >/dev/null 2>&1 || die "curl is required for execution"
systemctl is-active "$service" >/dev/null || die "$service is not active"

service_becomes_healthy() {
  local attempt
  for attempt in {1..30}; do
    systemctl is-active "$service" >/dev/null || return 1
    if curl --fail --silent --max-time 2 "$health_url" >/dev/null 2>&1; then
      return 0
    fi
    [[ "$attempt" -eq 30 ]] || sleep 1
  done
  return 1
}

install -d -m 0700 "$backup_root"
backup_dir="$backup_root/$(date -u +%Y%m%dT%H%M%SZ)-$$"
[[ ! -e "$backup_dir" ]] || die "backup target already exists"
install -d -m 0700 "$backup_dir"

systemctl stop "$service"
mv "$gateway_dir" "$backup_dir/original"
mv "$candidate" "$gateway_dir"
candidate=""
for private_path in epay.db data; do
  if [[ -e "$backup_dir/original/$private_path" ]]; then
    cp -a "$backup_dir/original/$private_path" "$gateway_dir/"
  fi
done

activation_failed=0
if ! systemctl start "$service" || ! service_becomes_healthy; then
  activation_failed=1
fi
if [[ "$activation_failed" -ne 0 ]]; then
  systemctl stop "$service" >/dev/null 2>&1 || true
  mv "$gateway_dir" "$backup_dir/failed-candidate"
  mv "$backup_dir/original" "$gateway_dir"
  systemctl start "$service" || die "candidate failed and rollback service could not start"
  service_becomes_healthy || die "candidate failed and original gateway rollback did not become healthy"
  die "candidate failed health verification; original gateway restored"
fi

printf '%s: gateway patches activated; rollback directory: %s/original\n' "$program" "$backup_dir"
