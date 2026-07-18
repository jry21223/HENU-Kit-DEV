-- name: GetActiveCoreSessionByTokenHash :one
SELECT s.id, s.user_id, s.expires_at, u.email_verified, u.status, u.created_at AS user_created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.kind = 'core'
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
FOR UPDATE OF s, u;

-- name: GetActiveCoreSessionForExchange :one
SELECT s.id
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1
  AND s.kind = 'core'
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND u.status = 'active'
FOR UPDATE OF s, u;

-- name: TouchCoreSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1 AND kind = 'core';

-- name: GetOAuthClient :one
SELECT id, redirect_uris FROM oauth_clients WHERE id = $1;

-- name: GetOAuthClientKey :one
SELECT client_id, key_id, secret_hash, status
FROM oauth_client_keys
WHERE client_id = $1 AND key_id = $2 AND status IN ('active', 'retiring');

-- name: CreateAuthorizationCode :one
INSERT INTO authorization_codes (
    code_hash, user_id, client_id, core_session_id, redirect_uri, code_challenge, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetAuthorizationCodeForUpdate :one
SELECT id, user_id, client_id, core_session_id, redirect_uri, code_challenge, expires_at, used_at
FROM authorization_codes
WHERE code_hash = $1
FOR UPDATE;

-- name: MarkAuthorizationCodeUsed :execrows
UPDATE authorization_codes SET used_at = now() WHERE id = $1 AND used_at IS NULL;

-- name: CreateExchangeSession :one
INSERT INTO sessions (
    user_id, kind, token_hash, client_id, parent_session_id, expires_at
) VALUES ($1, 'client_exchange', $2, $3, $4, $5)
RETURNING id, expires_at;

-- name: GetPlatformUser :one
SELECT id, email_verified, status, created_at FROM users WHERE id = $1;

-- name: GetOAuthExchangeIdempotency :one
SELECT request_hash, response_ciphertext, expires_at
FROM oauth_exchange_idempotency
WHERE client_id = $1 AND idempotency_key = $2 AND expires_at > now();

-- name: CreateOAuthExchangeIdempotency :exec
INSERT INTO oauth_exchange_idempotency (
    client_id, idempotency_key, request_hash, response_ciphertext, expires_at
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (client_id, idempotency_key) DO UPDATE SET
    request_hash = EXCLUDED.request_hash,
    response_ciphertext = EXCLUDED.response_ciphertext,
    expires_at = EXCLUDED.expires_at,
    created_at = now()
WHERE oauth_exchange_idempotency.expires_at <= now();
