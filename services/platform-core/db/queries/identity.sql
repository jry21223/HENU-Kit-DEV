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
SELECT id, email_verified, status, created_at, display_name FROM users WHERE id = $1;

-- name: ListUserDisplayNames :many
SELECT id, display_name FROM users WHERE id = ANY($1::uuid[]);

-- name: GetExchangeSessionAuthorizationContext :one
SELECT s.id, s.user_id, s.client_id,
       s.revoked_at AS session_revoked_at, s.expires_at AS session_expires_at,
       parent.revoked_at AS parent_revoked_at, parent.expires_at AS parent_expires_at,
       u.status AS user_status
FROM sessions s
JOIN sessions parent ON parent.id = s.parent_session_id
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.kind = 'client_exchange'
  AND parent.kind = 'core';

-- name: GetAuthorizationGrant :one
SELECT s.user_id, g.id AS grant_id,
       CAST(GREATEST(u.authorization_revision, r.revision, g.revision) AS bigint) AS authorization_revision
FROM sessions s
JOIN sessions parent ON parent.id = s.parent_session_id
JOIN users u ON u.id = s.user_id
JOIN user_role_grants g ON g.user_id = u.id AND g.status = 'active'
JOIN authorization_roles r ON r.id = g.role_id AND r.status = 'active'
JOIN role_permissions rp ON rp.role_id = r.id
JOIN permission_codes p ON p.code = rp.permission_code AND p.status = 'active'
WHERE s.token_hash = sqlc.arg(token_hash)
  AND s.kind = 'client_exchange'
  AND s.client_id = sqlc.arg(client_id)
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND parent.kind = 'core'
  AND parent.revoked_at IS NULL
  AND parent.expires_at > now()
  AND u.status = 'active'
  AND rp.permission_code = sqlc.arg(permission_code)
  AND (
      (g.scope_kind = 'platform')
      OR
      (g.scope_kind = 'product'
       AND sqlc.arg(scope_kind)::text IN ('product', 'resource')
       AND g.product_code = sqlc.arg(product_code)::text)
      OR
      (g.scope_kind = 'resource'
       AND sqlc.arg(scope_kind)::text = 'resource'
       AND g.product_code = sqlc.arg(product_code)::text
       AND g.resource_type = sqlc.arg(resource_type)::text
       AND g.resource_id = sqlc.arg(resource_id)::text)
  )
ORDER BY CASE g.scope_kind WHEN 'resource' THEN 1 WHEN 'product' THEN 2 ELSE 3 END, g.created_at
LIMIT 1;

-- name: CreateAuthorizationAuditEvent :exec
INSERT INTO authorization_audit_events (
    actor_user_id, session_id, request_id, service_id, permission_code,
    target_kind, target_product_code, target_resource_type, target_resource_id,
    decision, reason_code, grant_id, authorization_revision
) VALUES (
    sqlc.arg(actor_user_id), sqlc.arg(session_id), sqlc.arg(request_id), sqlc.arg(service_id), sqlc.arg(permission_code),
    sqlc.arg(target_kind), sqlc.narg(target_product_code), sqlc.narg(target_resource_type), sqlc.narg(target_resource_id),
    sqlc.arg(decision), sqlc.arg(reason_code), sqlc.narg(grant_id), sqlc.narg(authorization_revision)
);

-- name: GetOAuthExchangeIdempotency :one
SELECT request_hash, expires_at
FROM oauth_exchange_idempotency
WHERE client_id = $1 AND idempotency_key = $2 AND expires_at > now();

-- name: CreateOAuthExchangeIdempotency :exec
INSERT INTO oauth_exchange_idempotency (
    client_id, idempotency_key, request_hash, response_ciphertext, expires_at
) VALUES ($1, $2, $3, ''::bytea, $4)
ON CONFLICT (client_id, idempotency_key) DO UPDATE SET
    request_hash = EXCLUDED.request_hash,
    response_ciphertext = ''::bytea,
    expires_at = EXCLUDED.expires_at,
    created_at = now()
WHERE oauth_exchange_idempotency.expires_at <= now();
