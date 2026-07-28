#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat >&2 <<'EOF'
usage: deploy-henukit-artifact.sh <runtime-dir> <env-file> [platform-core-migration]

The image tarballs must already have been verified and loaded into Docker.
The optional migration is a filename from <runtime-dir>/migrations/platform-core.
EOF
}

die() {
  echo "deploy-henukit-artifact: $*" >&2
  exit 1
}

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage
  exit 64
fi

runtime_dir="$1"
env_file="$2"
migration_arg="${3:-}"

[[ -d "$runtime_dir" ]] || die "runtime directory does not exist"
[[ -r "$env_file" ]] || die "environment file is not readable"
[[ -r "$runtime_dir/RELEASE_SHA" ]] || die "runtime artifact has no RELEASE_SHA"
[[ -r "$runtime_dir/docker-compose.henukit.release.yml" ]] || die "runtime artifact has no release Compose file"

release_sha="$(tr -d '[:space:]' < "$runtime_dir/RELEASE_SHA")"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "RELEASE_SHA is not a full lowercase Git SHA"

export RELEASE_SHA="$release_sha"
compose=(docker compose --env-file "$env_file" -f "$runtime_dir/docker-compose.henukit.release.yml")
"${compose[@]}" config --quiet

# init-henukit-dbs.sh runs only when a PostgreSQL volume is first created.
# Releases against an existing volume must provision this independent database
# explicitly before Account Portfolio starts its embedded schema migration.
ensure_account_portfolio_database() {
  "${compose[@]}" exec -T postgres sh -ceu '
    if ! psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc \
      "SELECT 1 FROM pg_database WHERE datname = '\''account_portfolio'\''" | grep -qx 1; then
      createdb -U "$POSTGRES_USER" account_portfolio
    fi
  '
}

ensure_postgres_ready() {
  local attempt
  "${compose[@]}" up -d postgres
  for attempt in $(seq 1 30); do
    if "${compose[@]}" exec -T postgres sh -ceu 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null'; then
      return 0
    fi
    sleep 1
  done
  die "PostgreSQL did not become ready for Account Portfolio database provisioning"
}

# Platform Core migrations and the Account Portfolio database bootstrap both
# execute through postgres. A fresh host has no running container yet, so this
# readiness gate must run before either operation.
echo "Ensuring PostgreSQL is ready"
ensure_postgres_ready

if [[ -n "$migration_arg" ]]; then
  migration_dir="$(cd "$runtime_dir/migrations/platform-core" 2>/dev/null && pwd -P)" || die "migration directory is missing"
  if [[ "$migration_arg" = /* ]]; then
    migration_path="$(cd "$(dirname "$migration_arg")" 2>/dev/null && pwd -P)/$(basename "$migration_arg")" || die "migration path is invalid"
  else
    migration_path="$migration_dir/$migration_arg"
  fi
  [[ "$migration_path" == "$migration_dir/"* ]] || die "migration must come from the runtime artifact"
  [[ "$migration_path" =~ /[0-9]{6}_[a-z0-9_]+\.up\.sql$ ]] || die "migration must be a numbered .up.sql file"
  [[ -f "$migration_path" ]] || die "migration file does not exist"

  echo "Applying Platform Core migration $(basename "$migration_path") to the platform database"
  "${compose[@]}" exec -T postgres sh -ceu 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d platform -f -' < "$migration_path"
fi

echo "Ensuring Account Portfolio database exists"
ensure_account_portfolio_database
echo "Activating HENU Kit release $release_sha"
"${compose[@]}" up -d --remove-orphans

legacy_names='^(henukit-)?(study-api|study-worker|quizcraft-api|quizcraft-web)(-|$)'
if docker ps -a --format '{{.Names}}' | grep -E "$legacy_names" >/dev/null; then
  die "legacy Study or standalone QuizCraft container still exists"
fi

echo "HENU Kit release $release_sha is active without legacy runtime containers"
