#!/bin/bash
# Canonical privileged materials deployment path:
# verified branch snapshot -> manifest mirror -> derived slides -> catalogue SQL
# -> public snapshot -> one transactional database import.
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
root="${HENUKIT_MATERIALS_ROOT:-/opt/henukit-materials}"
repository="${HENUKIT_MATERIALS_REPOSITORY:-jry21223/HENU-Final-Review}"
repo_ref="${HENUKIT_MATERIALS_REPO_REF:-main}"
expected_ref="refs/heads/$repo_ref"
event_sha=""
event_delivery=""
event_repository=""
event_ref=""
event_argument_count=0
manual_mode=0

die() {
  echo "henukit-materials-sync: $*" >&2
  exit 1
}

while (( "$#" )); do
  case "$1" in
    --manual) manual_mode=1; shift ;;
    --sha) [[ -n "${2-}" ]] || die "missing value for --sha"; event_sha="$2"; event_argument_count=$((event_argument_count + 1)); shift 2 ;;
    --delivery) [[ -n "${2-}" ]] || die "missing value for --delivery"; event_delivery="$2"; event_argument_count=$((event_argument_count + 1)); shift 2 ;;
    --repository) [[ -n "${2-}" ]] || die "missing value for --repository"; event_repository="$2"; event_argument_count=$((event_argument_count + 1)); shift 2 ;;
    --ref) [[ -n "${2-}" ]] || die "missing value for --ref"; event_ref="$2"; event_argument_count=$((event_argument_count + 1)); shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done

if (( manual_mode )); then
  (( event_argument_count == 0 )) || die "--manual cannot be combined with webhook event arguments"
  event_delivery="manual-$(date +%s)-$$"
else
  (( event_argument_count == 4 )) || die "use --manual or provide one complete webhook event tuple"
  [[ "$event_sha" =~ ^[0-9a-f]{40}$ ]] || die "runner SHA must be a full lowercase Git SHA"
  [[ "$event_delivery" =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,127}$ ]] || die "runner delivery ID is invalid"
  [[ "$event_repository" == "$repository" ]] || die "runner repository does not match $repository"
  [[ "$event_ref" == "$expected_ref" ]] || die "runner ref does not match $expected_ref"
fi

for command in node python3 git flock; do
  command -v "$command" >/dev/null || die "$command is required"
done

mkdir -p "$root"
exec 9>"$root/.sync.lock"
flock 9

staging="$root/.staging"
public_root="$root/public"
snapshots="$public_root/.snapshots"
current_link="$public_root/current"
transaction="$root/.sync-transaction"
database_committed=0
database_mode=""
database_name=""
database_release_sha=""
postgres_user=""
compose=()
direct_pg_host=""
direct_pg_port=""
direct_pg_user=""
direct_pg_password=""
direct_pg_database=""
direct_pg_sslmode=""

direct_psql() {
  PGHOST="$direct_pg_host" \
  PGPORT="$direct_pg_port" \
  PGUSER="$direct_pg_user" \
  PGPASSWORD="$direct_pg_password" \
  PGDATABASE="$direct_pg_database" \
  PGSSLMODE="$direct_pg_sslmode" \
    psql -X "$@"
}

validate_root_policy_file() {
  local path="${1:?}"
  local secret="${2:-0}"
  local owner
  local mode
  [[ -f "$path" && ! -L "$path" ]] || die "policy file must be a regular file: $path"
  owner="$(stat -c '%u' "$path")"
  mode="$(stat -c '%a' "$path")"
  [[ "$owner" == 0 ]] || die "policy file must be root-owned: $path"
  (( (8#$mode & 8#022) == 0 )) || die "policy file must not be writable by group or other: $path"
  if (( secret )) && (( (8#$mode & 8#077) != 0 )); then
    die "secret policy file must have mode 0600: $path"
  fi
}

validate_root_controlled_path() {
  local current="${1:?}"
  local owner
  local mode
  [[ "$current" == /* ]] || die "root-controlled path must be absolute: $current"
  while :; do
    [[ -e "$current" && ! -L "$current" ]] || die "root-controlled path component is missing or symbolic: $current"
    owner="$(stat -c '%u' "$current")"
    mode="$(stat -c '%a' "$current")"
    [[ "$owner" == 0 ]] || die "root-controlled path component must be root-owned: $current"
    (( (8#$mode & 8#022) == 0 )) || {
      die "root-controlled path component must not be writable by group or other: $current"
    }
    [[ "$current" == / ]] && break
    current="$(dirname -- "$current")"
  done
}

configure_database() {
  if [[ -n "${HENUKIT_MATERIALS_DATABASE_URL:-}" ]]; then
    command -v psql >/dev/null || die "HENUKIT_MATERIALS_DATABASE_URL set but psql is missing"
    local -a connection=()
    readarray -d '' -t connection < <(
      python3 -c '
import sys
from urllib.parse import parse_qs, unquote, urlparse
parsed = urlparse(sys.stdin.read().strip())
if parsed.scheme not in {"postgres", "postgresql"}:
    raise SystemExit("unsupported PostgreSQL URL scheme")
query = parse_qs(parsed.query, keep_blank_values=True)
unsupported = set(query) - {"sslmode"}
if unsupported:
    raise SystemExit("unsupported PostgreSQL URL parameter")
values = [
    parsed.hostname or "",
    str(parsed.port or 5432),
    unquote(parsed.username or ""),
    unquote(parsed.password or ""),
    unquote(parsed.path.lstrip("/")),
    query.get("sslmode", ["prefer"])[-1],
]
sys.stdout.write("\0".join(values) + "\0")
' <<< "$HENUKIT_MATERIALS_DATABASE_URL"
    )
    (( ${#connection[@]} == 6 )) || die "HENUKIT_MATERIALS_DATABASE_URL is invalid or uses unsupported parameters"
    direct_pg_host="${connection[0]}"
    direct_pg_port="${connection[1]}"
    direct_pg_user="${connection[2]}"
    direct_pg_password="${connection[3]}"
    direct_pg_database="${connection[4]}"
    direct_pg_sslmode="${connection[5]}"
    [[ -n "$direct_pg_host" && "$direct_pg_port" =~ ^[0-9]{1,5}$ && -n "$direct_pg_user" ]] || die "direct PostgreSQL host, port and user are required"
    [[ "$direct_pg_database" =~ ^[A-Za-z_][A-Za-z0-9_-]{0,62}$ ]] || die "direct PostgreSQL database name is invalid"
    case "$direct_pg_sslmode" in disable|allow|prefer|require|verify-ca|verify-full) ;; *) die "direct PostgreSQL sslmode is invalid" ;; esac
    database_mode=direct
    return 0
  fi

  command -v docker >/dev/null || die "docker is required for the production database path"
  local active_sha_file="${HENUKIT_MATERIALS_ACTIVE_SHA_FILE:-/var/lib/henukit-actions-watch/last-activated-sha}"
  [[ -f "$active_sha_file" && ! -L "$active_sha_file" ]] || die "active release SHA marker not found: $active_sha_file"
  validate_root_policy_file "$active_sha_file" 0
  database_release_sha="$(tr -d '[:space:]' < "$active_sha_file")"
  [[ "$database_release_sha" =~ ^[0-9a-f]{40}$ ]] || die "active release SHA marker is invalid"

  local compose_dir="${HENUKIT_MATERIALS_RELEASE_DIR:-/opt/henukit-releases/$database_release_sha}"
  compose_dir="${compose_dir%/}"
  local compose_file="$compose_dir/docker-compose.henukit.release.yml"
  local release_marker="$compose_dir/RELEASE_SHA"
  local env_file="${HENUKIT_MATERIALS_ENV_FILE:-/etc/henukit-deploy/materials-production.env}"
  [[ -f "$compose_file" && ! -L "$compose_file" ]] || die "active release compose not found: $compose_file"
  [[ -f "$release_marker" && ! -L "$release_marker" ]] || die "active release marker not found: $release_marker"
  [[ "$(tr -d '[:space:]' < "$release_marker")" == "$database_release_sha" ]] || {
    die "release directory does not match the watcher active SHA"
  }
  [[ -f "$env_file" && ! -L "$env_file" ]] || die "production env file not found: $env_file"
  validate_root_policy_file "$compose_file" 0
  validate_root_policy_file "$release_marker" 0
  validate_root_policy_file "$env_file" 1
  validate_root_controlled_path "$env_file"

  compose=(docker compose --env-file "$env_file" -f "$compose_file")
  local database_url
  database_url="$(grep -E '^STUDY_DATABASE_URL=' "$env_file" | tail -1 | cut -d= -f2- || true)"
  [[ -n "$database_url" ]] || die "STUDY_DATABASE_URL not found in $env_file"
  database_name="$(python3 -c 'import sys, urllib.parse; value=sys.stdin.read().strip(); parsed=urllib.parse.urlparse(value); print(parsed.path.lstrip("/"))' <<< "$database_url")"
  [[ "$database_name" =~ ^[A-Za-z_][A-Za-z0-9_-]{0,62}$ ]] || die "cannot extract a safe database name from STUDY_DATABASE_URL"
  postgres_user="${POSTGRES_USER:-}"
  if [[ -z "$postgres_user" ]]; then
    postgres_user="$(grep -E '^POSTGRES_USER=' "$env_file" | tail -1 | cut -d= -f2- || true)"
  fi
  [[ "$postgres_user" =~ ^[A-Za-z_][A-Za-z0-9_-]{0,62}$ ]] || die "POSTGRES_USER is missing or invalid in $env_file"
  database_mode=compose
}

database_query() {
  local query="${1:?}"
  if [[ "$database_mode" == direct ]]; then
    direct_psql -v ON_ERROR_STOP=1 -tAc "$query"
  else
    RELEASE_SHA="$database_release_sha" "${compose[@]}" exec -T postgres \
      psql -X -U "$postgres_user" -d "$database_name" -v ON_ERROR_STOP=1 -tAc "$query"
  fi
}

# Return 0 for an exact in-transaction marker, 1 for a reachable database that
# has no matching marker, and 2 when the database cannot answer. Callers must
# preserve the journal on 2 because COMMIT outcome is then ambiguous.
database_marker_matches() {
  local expected_sha="${1:?}"
  local expected_delivery="${2:?}"
  local relation
  local marker
  if ! relation="$(database_query "SELECT to_regclass('public.henukit_materials_sync_state');" 2>/dev/null)"; then
    return 2
  fi
  relation="$(tr -d '[:space:]' <<< "$relation")"
  [[ -n "$relation" ]] || return 1
  if ! marker="$(database_query "SELECT synced_sha || ':' || delivery FROM public.henukit_materials_sync_state WHERE singleton = 1;" 2>/dev/null)"; then
    return 2
  fi
  marker="$(tr -d '[:space:]' <<< "$marker")"
  [[ "$marker" == "$expected_sha:$expected_delivery" ]]
}

validate_database_schema() {
  local status
  local schema_query
  read -r -d '' schema_query <<'SQL' || true
SELECT CASE WHEN
  to_regclass('public.materials') IS NOT NULL
  AND to_regclass('public.materials_storage_key_active_idx') IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'sha256'
  )
  AND EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'slides'
  )
  AND EXISTS (
    SELECT 1
    FROM pg_catalog.pg_class marker
    JOIN pg_catalog.pg_namespace marker_namespace ON marker_namespace.oid = marker.relnamespace
    WHERE marker_namespace.nspname = 'public'
      AND marker.relname = 'henukit_materials_sync_state'
      AND marker.relkind = 'r'
      AND EXISTS (
        SELECT 1 FROM pg_catalog.pg_attribute column_row
        WHERE column_row.attrelid = marker.oid AND column_row.attname = 'singleton'
          AND column_row.attnum > 0 AND NOT column_row.attisdropped
          AND column_row.atttypid = 'pg_catalog.int2'::regtype AND column_row.attnotnull
      )
      AND EXISTS (
        SELECT 1 FROM pg_catalog.pg_attribute column_row
        WHERE column_row.attrelid = marker.oid AND column_row.attname = 'synced_sha'
          AND column_row.attnum > 0 AND NOT column_row.attisdropped
          AND column_row.atttypid = 'pg_catalog.text'::regtype AND column_row.attnotnull
      )
      AND EXISTS (
        SELECT 1 FROM pg_catalog.pg_attribute column_row
        WHERE column_row.attrelid = marker.oid AND column_row.attname = 'delivery'
          AND column_row.attnum > 0 AND NOT column_row.attisdropped
          AND column_row.atttypid = 'pg_catalog.text'::regtype AND column_row.attnotnull
      )
      AND EXISTS (
        SELECT 1 FROM pg_catalog.pg_attribute column_row
        WHERE column_row.attrelid = marker.oid AND column_row.attname = 'updated_at'
          AND column_row.attnum > 0 AND NOT column_row.attisdropped
          AND column_row.atttypid = 'pg_catalog.timestamptz'::regtype AND column_row.attnotnull
      )
      AND EXISTS (
        SELECT 1
        FROM pg_catalog.pg_constraint constraint_row
        JOIN pg_catalog.pg_attribute singleton_column
          ON singleton_column.attrelid = constraint_row.conrelid
          AND singleton_column.attname = 'singleton'
          AND singleton_column.attnum > 0
          AND NOT singleton_column.attisdropped
        WHERE constraint_row.conrelid = marker.oid
          AND constraint_row.contype = 'p'
          AND constraint_row.convalidated
          AND constraint_row.conkey = ARRAY[singleton_column.attnum]::smallint[]
      )
      AND EXISTS (
        SELECT 1
        FROM pg_catalog.pg_constraint constraint_row
        WHERE constraint_row.conrelid = marker.oid
          AND constraint_row.contype = 'c'
          AND constraint_row.convalidated
          AND pg_catalog.pg_get_constraintdef(constraint_row.oid, true) = 'CHECK (singleton = 1)'
      )
  )
THEN 'ready' ELSE 'missing' END;
SQL
  if ! status="$(database_query "$schema_query" 2>/dev/null)"; then
    die "cannot verify the Study materials schema"
  fi
  status="$(tr -d '[:space:]' <<< "$status")"
  [[ "$status" == ready ]] || die "Study materials expand migration 0002 is required before synchronization"
}

database_import() {
  local sql_file="${1:?}"
  if [[ "$database_mode" == direct ]]; then
    echo "henukit-materials-sync: importing via direct database URL"
    direct_psql -v ON_ERROR_STOP=1 -f "$sql_file"
    return
  fi

  echo "henukit-materials-sync: importing into database $database_name"
  RELEASE_SHA="$database_release_sha" "${compose[@]}" exec -T postgres \
    psql -X -U "$postgres_user" -d "$database_name" -v ON_ERROR_STOP=1 -f - < "$sql_file"
}

validate_snapshot_target() {
  local target="${1-}"
  [[ -z "$target" || "$target" =~ ^\.snapshots/[A-Za-z0-9][A-Za-z0-9._-]{0,255}$ ]] || {
    die "invalid materials snapshot target: $target"
  }
}

delete_snapshot_target() {
  local target="${1:?}"
  validate_snapshot_target "$target"
  [[ "$target" == .snapshots/* ]] || die "refusing to delete a non-snapshot target"
  rm -rf -- "${public_root:?}/$target"
}

current_target() {
  local target
  if [[ ! -L "$current_link" ]]; then
    [[ ! -e "$current_link" ]] || die "$current_link must be a symbolic link"
    return 0
  fi
  target="$(readlink "$current_link")"
  validate_snapshot_target "$target"
  [[ -d "$public_root/$target/files" ]] || die "current materials snapshot is incomplete: $target"
  printf '%s\n' "$target"
}

published_snapshot_matches_event() {
  local expected_sha="${1:?}"
  local expected_delivery="${2:?}"
  local target
  target="$(current_target)"
  [[ -n "$target" ]] || return 1
  [[ -f "$public_root/$target/SYNCED_SHA" && ! -L "$public_root/$target/SYNCED_SHA" ]] || return 1
  [[ -f "$public_root/$target/DELIVERY" && ! -L "$public_root/$target/DELIVERY" ]] || return 1
  [[ "$(tr -d '[:space:]' < "$public_root/$target/SYNCED_SHA")" == "$expected_sha" ]] || return 1
  [[ "$(tr -d '[:space:]' < "$public_root/$target/DELIVERY")" == "$expected_delivery" ]]
}

switch_current() {
  local target="${1-}"
  local temp_link="$public_root/.current.$$"
  validate_snapshot_target "$target"
  rm -f -- "$temp_link"
  if [[ -z "$target" ]]; then
    rm -f -- "$current_link"
    return 0
  fi
  [[ -d "$public_root/$target/files" ]] || die "cannot activate incomplete snapshot: $target"
  ln -s "$target" "$temp_link"
  mv -Tf -- "$temp_link" "$current_link"
}

ensure_serving_layout() {
  local legacy_name
  local legacy_target
  local legacy_temp
  local current
  local path
  local -a legacy_entries=()

  mkdir -p "$public_root"
  [[ -d "$public_root" && ! -L "$public_root" ]] || die "$public_root must be a real directory"
  mkdir -p "$snapshots"
  chmod 0755 "$public_root" "$snapshots"
  [[ -d "$snapshots" && ! -L "$snapshots" ]] || die "$snapshots must be a real directory"

  # A prior failed first run may have created the canonical convenience links
  # before `current` existed. They are not legacy content and must not be copied
  # into a synthetic snapshot on retry.
  if [[ -L "$root/slides" && "$(readlink "$root/slides")" == public/current/slides && ! -e "$root/slides" ]]; then
    rm -f -- "$root/slides"
  fi
  if [[ -L "$root/SYNCED_SHA" && "$(readlink "$root/SYNCED_SHA")" == public/current/SYNCED_SHA && ! -e "$root/SYNCED_SHA" ]]; then
    rm -f -- "$root/SYNCED_SHA"
  fi

  if [[ ! -L "$current_link" ]]; then
    [[ ! -e "$current_link" ]] || die "$current_link exists but is not a symbolic link"
    # No snapshot is active, so an unfinished copy from before the atomic link
    # switch is unreachable and safe to discard before retrying the migration.
    for path in "$snapshots"/.legacy-*.tmp "$snapshots"/legacy-*; do
      [[ -d "$path" && -f "$path/.legacy-migration" ]] || continue
      rm -rf -- "$path"
    done
    while IFS= read -r -d '' path; do
      legacy_entries+=("$path")
    done < <(
      find "$public_root" -mindepth 1 -maxdepth 1 \
        ! -name .snapshots ! -name current -print0
    )
    if (( ${#legacy_entries[@]} > 0 )) || [[ -e "$root/slides" || -L "$root/slides" || -e "$root/SYNCED_SHA" || -L "$root/SYNCED_SHA" ]]; then
      legacy_name="legacy-$(date +%s)-$$"
      legacy_target=".snapshots/$legacy_name"
      legacy_temp="$snapshots/.${legacy_name}.tmp"
      mkdir -p "$legacy_temp/files" "$legacy_temp/slides"
      chmod 0755 "$legacy_temp" "$legacy_temp/files" "$legacy_temp/slides"
      for path in "${legacy_entries[@]}"; do
        [[ "$(basename "$path")" != .* ]] || die "legacy public layout contains a dotfile"
        if find "$path" -type l -print -quit | grep -q .; then
          die "legacy public layout contains a symbolic link"
        fi
        if find "$path" -mindepth 1 -name '.*' -print -quit | grep -q .; then
          die "legacy public layout contains a nested dotfile"
        fi
        cp -a -- "$path" "$legacy_temp/files/"
        printf '%s\0' "$(basename "$path")" >> "$legacy_temp/.legacy-sources"
      done
      if [[ -e "$root/slides" || -L "$root/slides" ]]; then
        rm -rf "$legacy_temp/slides"
        cp -a -- "$root/slides" "$legacy_temp/slides"
        : > "$legacy_temp/.legacy-slides"
      fi
      if [[ -e "$root/SYNCED_SHA" || -L "$root/SYNCED_SHA" ]]; then
        cp -a -- "$root/SYNCED_SHA" "$legacy_temp/SYNCED_SHA"
        : > "$legacy_temp/.legacy-synced-sha"
      fi
      # The privileged helper runs with umask 077. Preserve file contents and
      # modes, but make every directory in the served legacy snapshot
      # explicitly traversable by the read-only Nginx worker.
      find "$legacy_temp/files" "$legacy_temp/slides" -type d -exec chmod 0755 {} +
      : > "$legacy_temp/.legacy-migration"
      mv -- "$legacy_temp" "$snapshots/$legacy_name"
      switch_current "$legacy_target"
    fi
  fi

  current="$(current_target)"
  if [[ -n "$current" && -f "$public_root/$current/.legacy-migration" ]]; then
    if [[ -f "$public_root/$current/.legacy-sources" ]]; then
      while IFS= read -r -d '' path; do
        [[ -n "$path" && "$path" == "$(basename "$path")" && "$path" != . && "$path" != .. ]] || {
          die "invalid legacy migration source marker"
        }
        rm -rf -- "${public_root:?}/$path"
      done < "$public_root/$current/.legacy-sources"
    fi
    [[ ! -f "$public_root/$current/.legacy-slides" ]] || rm -rf -- "$root/slides"
    [[ ! -f "$public_root/$current/.legacy-synced-sha" ]] || rm -rf -- "$root/SYNCED_SHA"
    rm -f -- \
      "$public_root/$current/.legacy-sources" \
      "$public_root/$current/.legacy-slides" \
      "$public_root/$current/.legacy-synced-sha" \
      "$public_root/$current/.legacy-migration"
  fi
  if [[ -e "$root/slides" && ! -L "$root/slides" ]]; then
    die "$root/slides must be a symbolic link"
  fi
  if [[ -e "$root/SYNCED_SHA" && ! -L "$root/SYNCED_SHA" ]]; then
    die "$root/SYNCED_SHA must be a symbolic link"
  fi
  ln -sfn public/current/slides "$root/slides"
  ln -sfn public/current/SYNCED_SHA "$root/SYNCED_SHA"
}

write_transaction_phase() {
  local phase="${1:?}"
  local temp="$transaction/.phase.$$"
  [[ "$phase" == "preparing" || "$phase" == "published" || "$phase" == "committed" ]] || {
    die "invalid transaction phase: $phase"
  }
  printf '%s\n' "$phase" > "$temp"
  mv -f -- "$temp" "$transaction/phase"
}

begin_transaction() {
  local previous_target="${1-}"
  local new_target="${2:?}"
  local new_sha="${3:?}"
  local delivery="${4:?}"
  local temp="$root/.sync-transaction.$$"
  validate_snapshot_target "$previous_target"
  validate_snapshot_target "$new_target"
  [[ "$new_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid transaction SHA"
  [[ "$delivery" =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,127}$ ]] || die "invalid transaction delivery"
  [[ ! -e "$transaction" ]] || die "materials transaction already exists"
  rm -rf -- "$temp"
  mkdir -m 0700 "$temp"
  printf '%s\n' "$previous_target" > "$temp/previous"
  printf '%s\n' "$new_target" > "$temp/new"
  printf '%s\n' "$new_sha" > "$temp/sha"
  printf '%s\n' "$delivery" > "$temp/delivery"
  printf '%s\n' preparing > "$temp/phase"
  mv -- "$temp" "$transaction"
}

read_transaction() {
  [[ -d "$transaction" ]] || die "materials transaction metadata is corrupt"
  [[ -f "$transaction/phase" && -f "$transaction/previous" && -f "$transaction/new" && -f "$transaction/sha" && -f "$transaction/delivery" ]] || {
    die "materials transaction metadata is incomplete"
  }
  transaction_phase="$(cat "$transaction/phase")"
  transaction_previous="$(cat "$transaction/previous")"
  transaction_new="$(cat "$transaction/new")"
  transaction_sha="$(cat "$transaction/sha")"
  transaction_delivery="$(cat "$transaction/delivery")"
  [[ "$transaction_phase" == "preparing" || "$transaction_phase" == "published" || "$transaction_phase" == "committed" ]] || {
    die "invalid materials transaction phase: $transaction_phase"
  }
  validate_snapshot_target "$transaction_previous"
  validate_snapshot_target "$transaction_new"
  [[ "$transaction_sha" =~ ^[0-9a-f]{40}$ ]] || die "invalid materials transaction SHA"
  [[ "$transaction_delivery" =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,127}$ ]] || die "invalid materials transaction delivery"
}

rollback_transaction() {
  [[ -e "$transaction" ]] || return 0
  read_transaction
  echo "henukit-materials-sync: restoring snapshot interrupted in phase $transaction_phase" >&2
  switch_current "$transaction_previous"
  delete_snapshot_target "$transaction_new"
  rm -rf -- "$transaction"
}

finalize_transaction() {
  [[ -e "$transaction" ]] || return 0
  read_transaction
  switch_current "$transaction_new"
  if [[ -n "$transaction_previous" && "$transaction_previous" != "$transaction_new" ]]; then
    delete_snapshot_target "$transaction_previous"
  fi
  rm -rf -- "$transaction"
}

recover_interrupted_transaction() {
  [[ -e "$transaction" ]] || return 0
  read_transaction
  if [[ "$transaction_phase" == "committed" ]]; then
    finalize_transaction
  elif [[ "$transaction_phase" == "published" ]]; then
    if database_marker_matches "$transaction_sha" "$transaction_delivery"; then
      echo "henukit-materials-sync: database marker confirms interrupted COMMIT" >&2
      write_transaction_phase committed
      finalize_transaction
    else
      marker_status=$?
      if (( marker_status == 2 )); then
        die "database unavailable; preserving ambiguous published transaction"
      fi
      rollback_transaction
    fi
  else
    rollback_transaction
  fi
}

configure_database
validate_database_schema
ensure_serving_layout
recover_interrupted_transaction
if [[ -n "$event_sha" ]]; then
  if database_marker_matches "$event_sha" "$event_delivery"; then
    published_snapshot_matches_event "$event_sha" "$event_delivery" || {
      die "database marker matches event but published snapshot metadata is inconsistent"
    }
    echo "henukit-materials-sync: event already committed and published" >&2
    exit 0
  else
    marker_status=$?
    if (( marker_status == 2 )); then
      die "cannot verify whether this webhook event already committed"
    fi
  fi
fi
rm -rf -- "$staging"

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if (( status != 0 && database_committed == 0 )) && [[ -e "$transaction" ]]; then
    read_transaction || status=1
    if database_marker_matches "$transaction_sha" "$transaction_delivery"; then
      echo "henukit-materials-sync: database marker confirms COMMIT after client failure" >&2
      database_committed=1
      write_transaction_phase committed || status=1
      finalize_transaction || status=1
    else
      marker_status=$?
      if (( marker_status == 1 )); then
        echo "henukit-materials-sync: run failed before COMMIT; restoring previous public snapshot" >&2
        rollback_transaction || status=1
      else
        echo "henukit-materials-sync: database unavailable; preserving ambiguous published transaction" >&2
        status=1
      fi
    fi
  elif (( database_committed == 1 )) && [[ -e "$transaction" ]]; then
    write_transaction_phase committed || status=1
    finalize_transaction || status=1
  fi
  rm -rf -- "$staging"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

export HENUKIT_MATERIALS_ROOT="$root"
export HENUKIT_MATERIALS_STAGING_ROOT="$staging"
if [[ -n "$event_sha" ]]; then
  export HENUKIT_MATERIALS_EXPECTED_SHA="$event_sha"
fi

"$script_dir/sync-henukit-materials.sh"

manifest="$root/repo/manifest.json"
[[ -f "$manifest" ]] || die "manifest.json missing after snapshot preparation: $manifest"

mkdir -p "$staging/slides"
python3 "$script_dir/convert-henukit-slides.py" \
  --mirror "$staging/public" \
  --out "$staging/slides" \
  --manifest "$manifest"
find "$staging/slides" -type f -exec chmod 0444 {} +
find "$staging/slides" -type d -exec chmod 0755 {} +
chmod 0444 "$staging/SYNCED_SHA"
printf '%s\n' "$event_delivery" > "$staging/DELIVERY"
chmod 0444 "$staging/DELIVERY"
echo "henukit-materials-sync: slides conversion complete"

sql_file="$staging/import.sql"
node "$script_dir/import-henukit-materials.mjs" \
  --manifest "$manifest" \
  --slides-dir "$staging/slides" \
  --sync-sha "$(cat "$staging/SYNCED_SHA")" \
  --delivery "$event_delivery" \
  > "$sql_file"
[[ -s "$sql_file" ]] || die "catalogue importer produced empty SQL"

synced_sha="$(cat "$staging/SYNCED_SHA")"
snapshot_suffix="$event_delivery"
snapshot_name="$synced_sha-$snapshot_suffix-$$"
new_target=".snapshots/$snapshot_name"
new_snapshot="$public_root/$new_target"
previous_target="$(current_target)"
begin_transaction "$previous_target" "$new_target" "$synced_sha" "$event_delivery"
mkdir -m 0755 "$new_snapshot"
mv "$staging/public" "$new_snapshot/files"
mv "$staging/slides" "$new_snapshot/slides"
mv "$staging/SYNCED_SHA" "$new_snapshot/SYNCED_SHA"
mv "$staging/DELIVERY" "$new_snapshot/DELIVERY"
switch_current "$new_target"
write_transaction_phase published

database_import "$sql_file"

database_committed=1
write_transaction_phase committed
finalize_transaction
echo "henukit-materials-sync: published and indexed $(cat "$root/SYNCED_SHA")"
