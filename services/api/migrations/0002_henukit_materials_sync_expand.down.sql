BEGIN;

SELECT pg_advisory_xact_lock(306, 2);

DROP TABLE IF EXISTS public.henukit_materials_sync_state;

-- This expand is intentionally irreversible beyond the private sync marker.
-- sha256, slides and the partial storage-key index may already contain Study
-- production data or serve older compatible runtimes, so rollback retains them.

COMMIT;
