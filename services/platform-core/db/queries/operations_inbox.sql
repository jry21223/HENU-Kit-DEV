-- name: GetOperationsInboxIdempotency :one
SELECT request_hash, item_id, response_version, response_payload
FROM operations_inbox_idempotency
WHERE service_id = $1 AND actor_user_id = $2 AND operation = $3 AND idempotency_key = $4;

-- name: GetOperationsInboxOperationStatus :one
SELECT response_payload
FROM operations_inbox_idempotency
WHERE service_id = $1 AND actor_user_id = $2 AND operation = $3 AND idempotency_key = $4;

-- name: CreateOperationsInboxItem :one
INSERT INTO operations_inbox_items (
    source_product_code, source_resource_type, source_resource_id, source_resource_url,
    owner_user_id, priority, sla_due_at, status, created_by, updated_by
) VALUES (
    sqlc.arg(source_product_code), sqlc.arg(source_resource_type), sqlc.arg(source_resource_id), sqlc.narg(source_resource_url),
    sqlc.narg(owner_user_id), sqlc.arg(priority), sqlc.narg(sla_due_at), sqlc.arg(status), sqlc.arg(actor_user_id), sqlc.arg(actor_user_id)
)
RETURNING *;

-- name: GetOperationsInboxItem :one
SELECT * FROM operations_inbox_items WHERE id = $1;

-- name: ListOperationsInboxItems :many
SELECT * FROM operations_inbox_items
WHERE source_product_code = sqlc.arg(source_product_code)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY updated_at DESC, id
LIMIT sqlc.arg(page_size)
OFFSET sqlc.arg(page_offset);

-- name: UpdateOperationsInboxItem :one
UPDATE operations_inbox_items SET
    owner_user_id = CASE WHEN sqlc.arg(set_owner)::boolean THEN sqlc.narg(owner_user_id) ELSE owner_user_id END,
    priority = COALESCE(sqlc.narg(priority)::text, priority),
    sla_due_at = CASE WHEN sqlc.arg(set_sla)::boolean THEN sqlc.narg(sla_due_at) ELSE sla_due_at END,
    status = COALESCE(sqlc.narg(status)::text, status),
    version = version + 1,
    updated_by = sqlc.arg(actor_user_id),
    updated_at = now()
WHERE id = sqlc.arg(id) AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: CreateOperationsInboxIdempotency :exec
INSERT INTO operations_inbox_idempotency (
    service_id, actor_user_id, operation, idempotency_key, request_hash, item_id, response_version, response_payload
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: CreateOperationsInboxAudit :exec
INSERT INTO operations_inbox_audit_events (
    item_id, actor_user_id, request_id, action, from_version, to_version, item_snapshot
) VALUES ($1, $2, $3, $4, $5, $6, $7);
