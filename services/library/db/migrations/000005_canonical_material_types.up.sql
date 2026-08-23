BEGIN;

ALTER TABLE library_public_material_snapshots
    DROP CONSTRAINT IF EXISTS library_public_material_snapshots_type_check;

ALTER TABLE library_public_material_snapshots
    ADD CONSTRAINT library_public_material_snapshots_type_check
    CHECK (material_type IN (
        'note', 'exam', 'mock', 'path', 'lab', 'slides', 'textbook',
        'handout', 'exercise', 'answer'
    ));

-- Historical snapshots are immutable release evidence. Do not rewrite their
-- role/type rows without also changing the release activation identity.
-- Newly activated releases use only the seven canonical public types.

COMMIT;
