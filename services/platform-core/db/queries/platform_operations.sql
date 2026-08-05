-- name: ListPlatformOperationAccounts :many
SELECT id, display_name, email_verified, status, authorization_revision, created_at
FROM users
ORDER BY created_at DESC, id
LIMIT 20;

-- name: ListPlatformOperationAccountGrants :many
SELECT grants.user_id, roles.code AS role_code, grants.scope_kind,
       grants.product_code, grants.resource_type, grants.resource_id
FROM user_role_grants AS grants
JOIN authorization_roles AS roles ON roles.id = grants.role_id
WHERE grants.status = 'active'
  AND grants.user_id IN (
      SELECT id FROM users ORDER BY created_at DESC, id LIMIT 20
  )
ORDER BY grants.user_id, roles.code, grants.scope_kind,
         grants.product_code NULLS FIRST, grants.resource_type NULLS FIRST,
         grants.resource_id NULLS FIRST;

-- name: GetPlatformOperationAccountByEmailLookupHash :one
SELECT users.id, users.display_name, users.status
FROM email_identities
JOIN users ON users.id = email_identities.user_id
WHERE email_identities.email_lookup_hash = $1;

-- name: GetPlatformOperationSession :one
SELECT id, user_id, revoked_at, expires_at
FROM sessions
WHERE id = $1;

-- name: RevokePlatformOperationSession :one
UPDATE sessions
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL AND expires_at > now()
RETURNING id;

-- name: UpdatePlatformOperationUser :one
UPDATE users
SET status = sqlc.arg(status), authorization_revision = authorization_revision + 1
WHERE id = sqlc.arg(user_id) AND authorization_revision = sqlc.arg(expected_revision)
RETURNING authorization_revision;

-- name: GetPlatformOperationUser :one
SELECT id, authorization_revision
FROM users
WHERE id = $1;

-- name: RevokePlatformOperationUserGrants :exec
UPDATE user_role_grants
SET status = 'revoked', revision = revision + 1, revoked_at = now(), updated_at = now()
WHERE user_id = $1 AND status = 'active';

-- name: GetPlatformOperationRoleByCode :one
SELECT id
FROM authorization_roles
WHERE code = $1 AND status = 'active';

-- name: CreatePlatformOperationUserGrant :exec
INSERT INTO user_role_grants (
    user_id, role_id, scope_kind, product_code, resource_type, resource_id
) VALUES (
    sqlc.arg(user_id), sqlc.arg(role_id), sqlc.arg(scope_kind),
    sqlc.narg(product_code), sqlc.narg(resource_type), sqlc.narg(resource_id)
);

-- name: GetPlatformOperationIdempotency :one
SELECT request_hash, response_payload
FROM platform_operations_idempotency
WHERE service_id = $1 AND actor_user_id = $2 AND operation = $3 AND idempotency_key = $4;

-- name: CreatePlatformOperationIdempotency :exec
INSERT INTO platform_operations_idempotency (
    service_id, actor_user_id, operation, idempotency_key, request_hash, response_payload
) VALUES ($1, $2, $3, $4, $5, $6);

-- name: CreatePlatformOperationAudit :exec
INSERT INTO platform_operations_audit_events (
    actor_user_id, request_id, operation, resource_kind, resource_id, result_payload
) VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListPlatformOperationSessions :many
SELECT sessions.id, sessions.user_id, users.display_name, sessions.kind,
       sessions.client_id, sessions.last_seen_at, sessions.expires_at, sessions.revoked_at
FROM sessions
LEFT JOIN users ON users.id = sessions.user_id
ORDER BY sessions.created_at DESC, sessions.id
LIMIT 20;

-- name: CountPlatformOperationMailStatuses :one
SELECT
    count(*) FILTER (WHERE status = 'pending')::bigint AS pending,
    count(*) FILTER (WHERE status = 'processing')::bigint AS processing,
    count(*) FILTER (WHERE status = 'retry_due')::bigint AS retry_due,
    count(*) FILTER (WHERE status = 'accepted')::bigint AS accepted,
    count(*) FILTER (WHERE status = 'delivered')::bigint AS delivered,
    count(*) FILTER (WHERE status = 'failed')::bigint AS failed,
    (SELECT count(*)::bigint FROM mail_dead_letters WHERE requeued_at IS NULL) AS dead_letters
FROM mail_outbox;

-- name: ListPlatformOperationInboxItems :many
SELECT id, source_product_code, source_resource_type, source_resource_id,
       source_resource_url, owner_user_id, priority, sla_due_at, status,
       version, created_at, updated_at
FROM operations_inbox_items
ORDER BY updated_at DESC, id
LIMIT 20;

-- name: ListPlatformOperationAuditEvents :many
SELECT events.request_id, events.actor_user_id, users.display_name,
       events.permission_code, events.target_kind,
       events.target_product_code, events.target_resource_type,
       events.target_resource_id, events.decision, events.reason_code,
       events.created_at
FROM (
    SELECT request_id, actor_user_id, permission_code, target_kind,
           target_product_code, target_resource_type, target_resource_id,
           decision, reason_code, created_at
    FROM authorization_audit_events
    UNION ALL
    SELECT request_id, actor_user_id, 'platform.operations.write'::text,
           'resource'::text, NULL::text, resource_kind,
           resource_id::text, 'allowed'::text, operation || '_succeeded', created_at
    FROM platform_operations_audit_events
) AS events
LEFT JOIN users ON users.id = events.actor_user_id
ORDER BY events.created_at DESC, events.request_id
LIMIT 20;
