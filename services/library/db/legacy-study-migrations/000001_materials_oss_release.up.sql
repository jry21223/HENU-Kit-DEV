BEGIN;

DO $$
BEGIN
    IF to_regclass('public.materials') IS NULL THEN
        RAISE EXCEPTION 'public.materials is missing';
    END IF;
END
$$;

LOCK TABLE public.materials IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM public.materials
        WHERE deleted_at IS NULL
        GROUP BY storage_key
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'non-deleted materials contain duplicate storage_key values';
    END IF;
END
$$;

ALTER TABLE public.materials
    ADD COLUMN IF NOT EXISTS sha256 text,
    ADD COLUMN IF NOT EXISTS slides jsonb;

CREATE UNIQUE INDEX IF NOT EXISTS materials_storage_key_active_idx
    ON public.materials (storage_key)
    WHERE deleted_at IS NULL;

DO $$
DECLARE
    schema_ready boolean;
BEGIN
    SELECT
        EXISTS (
            SELECT 1
            FROM pg_attribute
            WHERE attrelid = 'public.materials'::regclass
              AND attname = 'sha256'
              AND atttypid = 'text'::regtype
              AND NOT attisdropped
        )
        AND EXISTS (
            SELECT 1
            FROM pg_attribute
            WHERE attrelid = 'public.materials'::regclass
              AND attname = 'slides'
              AND atttypid = 'jsonb'::regtype
              AND NOT attisdropped
        )
        AND EXISTS (
            SELECT 1
            FROM pg_index material_index
            JOIN pg_class index_relation ON index_relation.oid = material_index.indexrelid
            JOIN pg_namespace index_namespace ON index_namespace.oid = index_relation.relnamespace
            WHERE index_namespace.nspname = 'public'
              AND index_relation.relname = 'materials_storage_key_active_idx'
              AND material_index.indrelid = 'public.materials'::regclass
              AND material_index.indisunique
              AND material_index.indisvalid
              AND material_index.indisready
              AND material_index.indislive
              AND material_index.indnkeyatts = 1
              AND material_index.indexprs IS NULL
              AND pg_get_expr(material_index.indpred, material_index.indrelid) = '(deleted_at IS NULL)'
              AND (
                  SELECT array_agg(attribute.attname ORDER BY key_position.ordinality)
                  FROM unnest(material_index.indkey) WITH ORDINALITY AS key_position(attribute_number, ordinality)
                  JOIN pg_attribute attribute
                    ON attribute.attrelid = material_index.indrelid
                   AND attribute.attnum = key_position.attribute_number
              ) = ARRAY['storage_key']::name[]
        )
    INTO schema_ready;
    IF NOT schema_ready THEN
        RAISE EXCEPTION 'reviewed materials OSS schema verification failed';
    END IF;
END
$$;

COMMIT;
