#!/usr/bin/env bash
# Deprecated wrapper. The verified one-command local build-and-deploy flow is
# scripts/ops/deploy-henukit-local.sh (see docs/operations/henukit-local-deploy.md).
# This wrapper exists only so older references keep working; it forwards to the
# verified script.
set -Eeuo pipefail

program="deploy-henukit-one-shot"

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
verified="$repo_root/scripts/ops/deploy-henukit-local.sh"

[[ -x "$verified" ]] || die "verified wrapper missing: $verified (expected in the repository)"
printf '%s: forwarding to %s (the verified one-command flow)\n' "$program" "$(basename "$verified")" >&2
exec "$verified" "$@"
