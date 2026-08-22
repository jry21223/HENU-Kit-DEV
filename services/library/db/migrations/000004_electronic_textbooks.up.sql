ALTER TABLE library_public_material_snapshots
    DROP CONSTRAINT IF EXISTS library_public_material_snapshots_type_check;

ALTER TABLE library_public_material_snapshots
    ADD CONSTRAINT library_public_material_snapshots_type_check
    CHECK (material_type IN ('note', 'exam', 'mock', 'path', 'lab', 'slides', 'textbook'));
