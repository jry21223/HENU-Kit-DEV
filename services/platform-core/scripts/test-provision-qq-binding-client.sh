#!/usr/bin/env bash
set -euo pipefail

# Run only against the disposable Platform Core CI database after migration 000021.
: "${PLATFORM_CORE_TEST_DATABASE_URL:?test database URL is required}"
: "${PGHOST:?test PGHOST is required}"
: "${PGPORT:?test PGPORT is required}"
: "${PGUSER:?test PGUSER is required}"
: "${PGDATABASE:?test PGDATABASE is required}"
: "${PGPASSWORD:?test PGPASSWORD is required}"
if [[ "$PGDATABASE" != platform_core_test ||
      ! "$PGHOST" =~ ^(localhost|127\.0\.0\.1)$ ||
      "$PLATFORM_CORE_TEST_DATABASE_URL" != */platform_core_test\?* ]]; then
  echo 'Refusing to run provisioning integration checks outside a local test database' >&2
  exit 1
fi

script="${PROVISION_SCRIPT:-$(dirname "$0")/provision-qq-binding-client.sh}"
export DATABASE_URL="$PLATFORM_CORE_TEST_DATABASE_URL"
export HENU_KIT_DATABASE_HOSTADDR="$PGHOST"
export HENU_KIT_QQ_APP_ID=qq-provision-test-app
export HENU_KIT_CLIENT_ID=henu-bot-qq-provision-test
export HENU_KIT_KEY_ID=key-a
export HENU_KIT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

sql() {
  env -i PATH="$PATH" PGHOST="$PGHOST" PGPORT="$PGPORT" PGUSER="$PGUSER" \
    PGDATABASE="$PGDATABASE" PGPASSWORD="$PGPASSWORD" \
    psql -X -w -v ON_ERROR_STOP=1 -Atqc "$1"
}
provision() {
  local output
  output="$(mktemp)"
  if PGHOSTADDR=192.0.2.1 PGSERVICE=nonexistent bash "$script" >"$output" 2>&1; then
    rm "$output"
    return 0
  fi
  echo "provision failed for test key $HENU_KIT_KEY_ID" >&2
  cat "$output" >&2
  rm "$output"
  return 1
}
expect_failure() {
  if PGHOSTADDR=192.0.2.1 PGSERVICE=nonexistent bash "$script" >/dev/null 2>&1; then
    echo "provisioning unexpectedly succeeded" >&2
    exit 1
  fi
}
keys() {
  sql "SELECT string_agg(key_id || ':' || status, ',' ORDER BY key_id) FROM oauth_client_keys WHERE client_id='henu-bot-qq-provision-test'"
}
cleanup() {
  sql "DELETE FROM oauth_clients WHERE id IN ('henu-bot-qq-provision-test','henu-bot-qq-provision-conflict','henu-bot-qq-provision-other')" >/dev/null
}
trap cleanup EXIT
cleanup

provision
test "$(sql "SELECT client_id FROM qq_binding_apps WHERE app_id='qq-provision-test-app'")" = "$HENU_KIT_CLIENT_ID"
test "$(keys)" = 'key-a:active'

provision
test "$(keys)" = 'key-a:active'

export HENU_KIT_KEY_ID=key-b
export HENU_KIT_SECRET=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
provision
test "$(keys)" = 'key-a:retiring,key-b:active'

# An exact retry must not shorten the previous key's overlap window.
provision
test "$(keys)" = 'key-a:retiring,key-b:active'

export HENU_KIT_SECRET=cccccccccccccccccccccccccccccccc
expect_failure
test "$(keys)" = 'key-a:retiring,key-b:active'

export HENU_KIT_KEY_ID=key-c
provision
test "$(keys)" = 'key-a:revoked,key-b:retiring,key-c:active'

export HENU_KIT_KEY_ID=key-a
export HENU_KIT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expect_failure
test "$(keys)" = 'key-a:revoked,key-b:retiring,key-c:active'

# Neither an existing OAuth client nor an existing QQ app may be silently remapped.
sql "INSERT INTO oauth_clients(id,redirect_uris) VALUES('henu-bot-qq-provision-conflict',ARRAY['https://example.invalid/callback'])"
export HENU_KIT_QQ_APP_ID=qq-provision-conflict-app
export HENU_KIT_CLIENT_ID=henu-bot-qq-provision-conflict
expect_failure
test "$(sql "SELECT count(*) FROM qq_binding_apps WHERE app_id='qq-provision-conflict-app'")" = 0
test "$(sql "SELECT count(*) FROM oauth_client_keys WHERE client_id='henu-bot-qq-provision-conflict'")" = 0

sql "INSERT INTO oauth_clients(id,redirect_uris) VALUES('henu-bot-qq-provision-other',ARRAY['https://invalid.invalid/qq-binding-service-only']); INSERT INTO qq_binding_apps(app_id,client_id) VALUES('qq-provision-conflict-app','henu-bot-qq-provision-other')"
expect_failure
test "$(sql "SELECT client_id FROM qq_binding_apps WHERE app_id='qq-provision-conflict-app'")" = henu-bot-qq-provision-other
test "$(sql "SELECT count(*) FROM oauth_client_keys WHERE client_id='henu-bot-qq-provision-conflict'")" = 0

echo 'QQ binding provisioning integration checks passed'
