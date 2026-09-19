CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS notice_operations_legacy_key_idx
    ON notice_operations (client_id, method, normalized_route, idempotency_key)
    WHERE actor_user_id IS NULL;
