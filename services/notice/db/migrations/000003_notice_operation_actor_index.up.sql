CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS notice_operations_actor_key_idx
    ON notice_operations (client_id, actor_user_id, method, normalized_route, idempotency_key)
    WHERE actor_user_id IS NOT NULL;
