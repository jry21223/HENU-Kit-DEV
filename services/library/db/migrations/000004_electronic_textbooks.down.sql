BEGIN;

-- Snapshot rows are immutable release evidence. This migration can only narrow
-- the constraint when every stored row already satisfies the older domain; it
-- must never rewrite a textbook or a later canonical type to make rollback fit.
DO $migration$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM library_public_material_snapshots
        WHERE material_type NOT IN ('note', 'exam', 'mock', 'path', 'lab', 'slides')
    ) THEN
        RAISE EXCEPTION
            'cannot remove textbook material type while immutable snapshots use newer material types';
    END IF;

    ALTER TABLE library_public_material_snapshots
        DROP CONSTRAINT IF EXISTS library_public_material_snapshots_type_check;
    ALTER TABLE library_public_material_snapshots
        ADD CONSTRAINT library_public_material_snapshots_type_check
        CHECK (material_type IN ('note', 'exam', 'mock', 'path', 'lab', 'slides'));
END
$migration$;

COMMIT;
