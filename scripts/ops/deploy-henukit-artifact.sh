#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat >&2 <<'EOF'
usage: deploy-henukit-artifact.sh <runtime-dir> <env-file> [platform-core-migrations]

The image tarballs must already have been verified and loaded into Docker.
The optional value is a comma-separated list of filenames from
<runtime-dir>/migrations/platform-core, applied in the supplied order.
Numbered .up.sql migrations shipped under <runtime-dir>/migrations/notice
and <runtime-dir>/migrations/food are applied automatically to their owner
databases (created on demand on fresh hosts) before the release activates.
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

ensure_owner_database() {
  local owner="$1"
  "${compose[@]}" exec -T postgres sh -ceu '
    if ! psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atqc \
      "SELECT 1 FROM pg_database WHERE datname = '\''"$1"'\''" | grep -qx 1; then
      createdb -U "$POSTGRES_USER" "$1"
    fi
  ' sh "$owner"
}

# Notice and Food own their schemas but carry no embedded migration runner,
# so the release applies every numbered .up.sql migration shipped under
# <runtime>/migrations/<owner> through postgres before activating. Existing
# production databases are not recreated; createdb is only a fresh-host guard.
apply_owner_migrations() {
  local owner="$1"
  local migration_dir="$runtime_dir/migrations/$owner"
  [[ -d "$migration_dir" ]] || return 0
  ensure_owner_database "$owner"
  local migration_path
  for migration_path in "$migration_dir"/*.up.sql; do
    [[ -e "$migration_path" ]] || continue
    echo "Applying $owner migration $(basename "$migration_path") to the $owner database"
    "${compose[@]}" exec -T postgres sh -ceu 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1" -f -' sh "$owner" < "$migration_path"
  done
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
  IFS=',' read -r -a migration_names <<< "$migration_arg"
  for migration_name in "${migration_names[@]}"; do
    [[ "$migration_name" =~ ^[0-9]{6}_[a-z0-9_]+\.up\.sql$ ]] || die "migration must be a numbered .up.sql filename"
    migration_path="$migration_dir/$migration_name"
    [[ -f "$migration_path" && ! -L "$migration_path" ]] || die "migration file does not exist or is unsafe"
    echo "Applying Platform Core migration $migration_name to the platform database"
    "${compose[@]}" exec -T postgres sh -ceu 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d platform -f -' < "$migration_path"
  done
fi

apply_owner_migrations notice
apply_owner_migrations food

echo "Ensuring Account Portfolio database exists"
ensure_account_portfolio_database
echo "Activating HENU Kit release $release_sha"
"${compose[@]}" up -d --remove-orphans

legacy_names='^(henukit-)?(study-api|study-worker|quizcraft-api|quizcraft-web)(-|$)'
if docker ps -a --format '{{.Names}}' | grep -E "$legacy_names" >/dev/null; then
  die "legacy Study or standalone QuizCraft container still exists"
fi

echo "HENU Kit release $release_sha is active without legacy runtime containers"
