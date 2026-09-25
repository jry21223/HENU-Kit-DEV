#!/usr/bin/env bash
set -euo pipefail
: "${DATABASE_URL:?DATABASE_URL is required}"
: "${HENU_KIT_QQ_APP_ID:?HENU_KIT_QQ_APP_ID is required}"
: "${HENU_KIT_CLIENT_ID:?HENU_KIT_CLIENT_ID is required}"
: "${HENU_KIT_KEY_ID:?HENU_KIT_KEY_ID is required}"
: "${HENU_KIT_SECRET:?HENU_KIT_SECRET is required}"
[[ "$HENU_KIT_QQ_APP_ID" =~ ^[A-Za-z0-9_-]{1,128}$ ]]
[[ "$HENU_KIT_CLIENT_ID" =~ ^henu-bot-[A-Za-z0-9_-]{1,100}$ ]]
[[ "$HENU_KIT_KEY_ID" =~ ^[A-Za-z0-9_-]{1,100}$ ]]
[[ ${#HENU_KIT_SECRET} -ge 32 ]]
qq_secret_hash="$(printf '%s' "$HENU_KIT_SECRET" | openssl dgst -sha256 -binary | xxd -p -c 256)"
PGDATABASE="$DATABASE_URL" psql -v ON_ERROR_STOP=1 \
  -v app_id="$HENU_KIT_QQ_APP_ID" -v client_id="$HENU_KIT_CLIENT_ID" \
  -v key_id="$HENU_KIT_KEY_ID" -v secret_hash="$qq_secret_hash" <<'SQL'
BEGIN;
INSERT INTO oauth_clients(id,redirect_uris)
VALUES(:'client_id',ARRAY['https://invalid.invalid/qq-binding-service-only'])
ON CONFLICT(id) DO NOTHING;
-- Never remap an existing application to another service silently.
INSERT INTO qq_binding_apps(app_id,client_id) VALUES(:'app_id',:'client_id')
ON CONFLICT(app_id) DO UPDATE SET client_id=qq_binding_apps.client_id
WHERE qq_binding_apps.client_id=EXCLUDED.client_id;
SELECT EXISTS(SELECT 1 FROM qq_binding_apps WHERE app_id=:'app_id' AND client_id=:'client_id') AS matches \gset
\if :matches
\else
  ROLLBACK;
  \quit 1
\endif
UPDATE oauth_client_keys SET status='revoked' WHERE client_id=:'client_id' AND status='retiring' AND key_id<>:'key_id';
UPDATE oauth_client_keys SET status='retiring' WHERE client_id=:'client_id' AND status='active' AND key_id<>:'key_id';
INSERT INTO oauth_client_keys(client_id,key_id,secret_hash,status)
VALUES(:'client_id',:'key_id',decode(:'secret_hash','hex'),'active')
ON CONFLICT(client_id,key_id) DO UPDATE SET secret_hash=EXCLUDED.secret_hash,status='active';
COMMIT;
SQL
echo "QQ binding service credential provisioned; plaintext secret was not printed."
