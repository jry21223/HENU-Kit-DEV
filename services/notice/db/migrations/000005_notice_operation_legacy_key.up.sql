SET lock_timeout = '5s';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_index AS index_state
        JOIN pg_class AS index_relation ON index_relation.oid = index_state.indexrelid
        JOIN pg_namespace AS index_schema ON index_schema.oid = index_relation.relnamespace
        WHERE index_schema.nspname = 'public'
          AND index_relation.relname IN ('notice_operations_actor_key_idx', 'notice_operations_legacy_key_idx')
          AND index_state.indisunique
          AND index_state.indisvalid
          AND index_state.indisready
        GROUP BY index_schema.nspname
        HAVING count(*) = 2
    ) THEN
        RAISE EXCEPTION 'replacement Notice operation indexes are not valid and ready';
    END IF;
END;
$$;

ALTER TABLE notice_operations
    DROP CONSTRAINT IF EXISTS notice_operations_pkey;
