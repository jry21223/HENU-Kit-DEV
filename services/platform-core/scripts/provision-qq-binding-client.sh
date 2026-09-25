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
# Keep the database URI and its password out of psql's process arguments.
# The optional host address is for a verified Postgres bridge IP when the URI
# hostname only resolves inside Compose. Keep PGHOST for TLS hostname checks.
export DATABASE_URL HENU_KIT_DATABASE_HOSTADDR="${HENU_KIT_DATABASE_HOSTADDR:-}"
python3 -c '
import ipaddress
import os
import sys
from urllib.parse import parse_qsl, unquote, urlsplit

try:
    uri = urlsplit(os.environ["DATABASE_URL"])
    if uri.scheme not in {"postgres", "postgresql"} or not uri.hostname:
        raise ValueError("invalid PostgreSQL URI")
    if not uri.username or not uri.path.startswith("/") or not uri.path[1:] or uri.fragment:
        raise ValueError("incomplete PostgreSQL URI")
    options = parse_qsl(uri.query, keep_blank_values=True, strict_parsing=True)
    if len(options) != len({key for key, _ in options}) or any(
        key != "sslmode" for key, _ in options
    ):
        raise ValueError("unsupported PostgreSQL URI option")
    address = os.environ.get("HENU_KIT_DATABASE_HOSTADDR", "")
    if address:
        ipaddress.ip_address(address)
    connection = {
        "PGHOST": uri.hostname,
        "PGPORT": str(uri.port or 5432),
        "PGDATABASE": unquote(uri.path[1:]),
        "PGUSER": unquote(uri.username),
        "PGPASSWORD": unquote(uri.password) if uri.password is not None else "",
    }
    if address:
        connection["PGHOSTADDR"] = address
    if options:
        connection["PGSSLMODE"] = options[0][1]
except (KeyError, ValueError) as exc:
    print("Invalid QQ binding database connection configuration", file=sys.stderr)
    sys.exit(2)

# Inherited libpq options must not redirect a privileged provisioning call.
environment = {
    key: value for key, value in os.environ.items()
    if not key.startswith("PG") and key not in {
        "DATABASE_URL", "HENU_KIT_DATABASE_HOSTADDR", "HENU_KIT_SECRET"
    }
}
environment.update(connection)
os.execvpe("psql", ["psql", "-X", "-w", "-v", "ON_ERROR_STOP=1", *sys.argv[1:]], environment)
' \
  -v app_id="$HENU_KIT_QQ_APP_ID" -v client_id="$HENU_KIT_CLIENT_ID" \
  -v key_id="$HENU_KIT_KEY_ID" -v secret_hash="$qq_secret_hash" <<'SQL'
BEGIN;
INSERT INTO oauth_clients(id,redirect_uris)
VALUES(:'client_id',ARRAY['https://invalid.invalid/qq-binding-service-only'])
ON CONFLICT(id) DO NOTHING;
SELECT 1 AS client_locked FROM oauth_clients WHERE id=:'client_id' FOR UPDATE \gset
SELECT redirect_uris=ARRAY['https://invalid.invalid/qq-binding-service-only'] AS redirects_match
FROM oauth_clients WHERE id=:'client_id' \gset
\if :redirects_match
\else
  ROLLBACK;
  DO $$ BEGIN RAISE EXCEPTION 'QQ binding client ID belongs to another OAuth flow'; END $$;
\endif
-- Never remap an existing application to another service silently.
INSERT INTO qq_binding_apps(app_id,client_id) VALUES(:'app_id',:'client_id')
ON CONFLICT(app_id) DO UPDATE SET client_id=qq_binding_apps.client_id
WHERE qq_binding_apps.client_id=EXCLUDED.client_id;
SELECT EXISTS(SELECT 1 FROM qq_binding_apps WHERE app_id=:'app_id' AND client_id=:'client_id') AS matches \gset
\if :matches
\else
  ROLLBACK;
  DO $$ BEGIN RAISE EXCEPTION 'QQ app/client mapping conflict'; END $$;
\endif
SELECT EXISTS(SELECT 1 FROM oauth_client_keys WHERE client_id=:'client_id' AND key_id=:'key_id') AS key_exists,
       EXISTS(SELECT 1 FROM oauth_client_keys WHERE client_id=:'client_id' AND key_id=:'key_id'
              AND secret_hash=decode(:'secret_hash','hex') AND status='active') AS exact_replay \gset
\if :key_exists
  \if :exact_replay
    COMMIT;
  \else
    ROLLBACK;
    DO $$ BEGIN RAISE EXCEPTION 'QQ binding key ID already exists with different material or state'; END $$;
  \endif
\else
  UPDATE oauth_client_keys SET status='revoked' WHERE client_id=:'client_id' AND status='retiring' AND key_id<>:'key_id';
  UPDATE oauth_client_keys SET status='retiring' WHERE client_id=:'client_id' AND status='active' AND key_id<>:'key_id';
  INSERT INTO oauth_client_keys(client_id,key_id,secret_hash,status)
  VALUES(:'client_id',:'key_id',decode(:'secret_hash','hex'),'active');
  COMMIT;
\endif
SQL
echo "QQ binding service credential provisioned; plaintext secret was not printed."
