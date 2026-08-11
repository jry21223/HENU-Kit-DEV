BEGIN;

-- Serialize repeated/operator-concurrent attempts at this exact Study expand.
SELECT pg_advisory_xact_lock(306, 2);

DO $migration$
BEGIN
  IF to_regclass('public.materials') IS NULL THEN
    RAISE EXCEPTION 'Study baseline table public.materials is required before migration 0002';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'storage_key'
  ) OR NOT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'deleted_at'
  ) THEN
    RAISE EXCEPTION 'Study baseline public.materials is missing storage_key or deleted_at';
  END IF;
  IF EXISTS (
    SELECT storage_key
    FROM public.materials
    WHERE deleted_at IS NULL
    GROUP BY storage_key
    HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION 'active Study materials contain duplicate storage_key values';
  END IF;
END;
$migration$;

ALTER TABLE public.materials ADD COLUMN IF NOT EXISTS sha256 text;
ALTER TABLE public.materials ADD COLUMN IF NOT EXISTS slides jsonb;
CREATE UNIQUE INDEX IF NOT EXISTS materials_storage_key_active_idx
  ON public.materials (storage_key)
  WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS public.henukit_materials_sync_state (
  singleton smallint PRIMARY KEY CHECK (singleton = 1),
  synced_sha text NOT NULL,
  delivery text NOT NULL,
  updated_at timestamptz NOT NULL
);

COMMIT;
