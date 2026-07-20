#!/usr/bin/env bash
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
: "${QUIZCRAFT_PLATFORM_CLIENT_SECRET:?QUIZCRAFT_PLATFORM_CLIENT_SECRET is required}"
: "${QUIZCRAFT_PLATFORM_KEY_ID:?QUIZCRAFT_PLATFORM_KEY_ID is required}"
: "${QUIZCRAFT_PUBLIC_URL:?QUIZCRAFT_PUBLIC_URL is required}"

if [ "${#QUIZCRAFT_PLATFORM_CLIENT_SECRET}" -lt 32 ]; then
  echo "QUIZCRAFT_PLATFORM_CLIENT_SECRET must contain at least 32 bytes" >&2
  exit 1
fi
if [[ ! "$QUIZCRAFT_PLATFORM_KEY_ID" =~ ^[A-Za-z0-9._-]{1,120}$ ]]; then
  echo "QUIZCRAFT_PLATFORM_KEY_ID has an invalid format" >&2
  exit 1
fi
if [[ ! "$QUIZCRAFT_PUBLIC_URL" =~ ^https://[^/?#]+/?$ ]]; then
  echo "QUIZCRAFT_PUBLIC_URL must be an HTTPS origin" >&2
  exit 1
fi

redirect_uri="${QUIZCRAFT_PUBLIC_URL%/}/auth/callback"
secret_hash_hex="$(printf '%s' "$QUIZCRAFT_PLATFORM_CLIENT_SECRET" | openssl dgst -sha256 -binary | xxd -p -c 256)"

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
  -v client_id="quizcraft" \
  -v key_id="$QUIZCRAFT_PLATFORM_KEY_ID" \
  -v redirect_uri="$redirect_uri" \
  -v secret_hash_hex="$secret_hash_hex" <<'SQL'
BEGIN;
INSERT INTO oauth_clients(id, redirect_uris)
VALUES (:'client_id', ARRAY[:'redirect_uri'])
ON CONFLICT (id) DO UPDATE SET redirect_uris=EXCLUDED.redirect_uris;

UPDATE oauth_client_keys
SET status='revoked'
WHERE client_id=:'client_id' AND status='retiring' AND key_id<>:'key_id';

UPDATE oauth_client_keys
SET status='retiring'
WHERE client_id=:'client_id' AND status='active' AND key_id<>:'key_id';

INSERT INTO oauth_client_keys(client_id, key_id, secret_hash, status)
VALUES (:'client_id', :'key_id', decode(:'secret_hash_hex', 'hex'), 'active')
ON CONFLICT (client_id, key_id)
DO UPDATE SET secret_hash=EXCLUDED.secret_hash, status='active';
COMMIT;
SQL

echo "Provisioned QuizCraft OAuth/HMAC client for $redirect_uri with key $QUIZCRAFT_PLATFORM_KEY_ID"
