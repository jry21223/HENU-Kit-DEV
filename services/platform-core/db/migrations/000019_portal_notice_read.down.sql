-- Portal notice grants are durable authorization decisions. A schema rollback
-- must not silently revoke grants or reactivate a previously revoked grant.
SELECT 1;
