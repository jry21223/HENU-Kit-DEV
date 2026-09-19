-- The legacy application cannot safely interpret actor-scoped keys. Clear this
-- transient 24-hour cache before restoring its global uniqueness contract.
TRUNCATE TABLE notice_operations;

ALTER TABLE notice_operations
    ADD CONSTRAINT notice_operations_pkey
    PRIMARY KEY (client_id, method, normalized_route, idempotency_key);
