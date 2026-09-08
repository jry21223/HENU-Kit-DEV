BEGIN;

CREATE TEMPORARY TABLE library_expected_material_type_constraint_000005 (
    material_type TEXT NOT NULL
) ON COMMIT DROP;

ALTER TABLE library_expected_material_type_constraint_000005
    ADD CONSTRAINT library_expected_material_type_constraint_000005_check
    CHECK (material_type IN (
        'note', 'exam', 'mock', 'path', 'lab', 'slides', 'textbook',
        'handout', 'exercise', 'answer'
    ));

DO $migration$
DECLARE
    current_definition TEXT;
    expected_definition TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid)
    INTO current_definition
    FROM pg_constraint
    WHERE conrelid = 'library_public_material_snapshots'::regclass
      AND conname = 'library_public_material_snapshots_type_check';

    SELECT pg_get_constraintdef(oid)
    INTO STRICT expected_definition
    FROM pg_constraint
    WHERE conrelid = 'library_expected_material_type_constraint_000005'::regclass
      AND conname = 'library_expected_material_type_constraint_000005_check';

    IF current_definition IS DISTINCT FROM expected_definition THEN
        ALTER TABLE library_public_material_snapshots
            DROP CONSTRAINT IF EXISTS library_public_material_snapshots_type_check;
        ALTER TABLE library_public_material_snapshots
            ADD CONSTRAINT library_public_material_snapshots_type_check
            CHECK (material_type IN (
                'note', 'exam', 'mock', 'path', 'lab', 'slides', 'textbook',
                'handout', 'exercise', 'answer'
            ));
    END IF;
END
$migration$;

-- Historical snapshots are immutable release evidence. Do not rewrite their
-- role/type rows without also changing the release activation identity.
-- Newly activated releases use only the seven canonical public types.

COMMIT;
