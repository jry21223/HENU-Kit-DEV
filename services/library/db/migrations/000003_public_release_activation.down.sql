DROP TRIGGER IF EXISTS library_public_release_activation_events_append_only ON library_public_release_activation_events;
DROP FUNCTION IF EXISTS library_public_release_activation_append_only();
DROP TABLE IF EXISTS library_public_release_activation_events;

ALTER TABLE library_public_material_snapshots
    DROP CONSTRAINT IF EXISTS library_public_material_snapshots_public_path_check,
    DROP CONSTRAINT IF EXISTS library_public_material_snapshots_type_check,
    DROP CONSTRAINT IF EXISTS library_public_material_snapshots_role_check,
    DROP CONSTRAINT IF EXISTS library_public_material_snapshots_subject_check,
    DROP COLUMN IF EXISTS public_path,
    DROP COLUMN IF EXISTS material_type,
    DROP COLUMN IF EXISTS role,
    DROP COLUMN IF EXISTS subject;

ALTER TABLE library_public_releases
    DROP CONSTRAINT IF EXISTS library_public_releases_activation_digest_check,
    DROP CONSTRAINT IF EXISTS library_public_releases_oss_commit_sha256_check,
    DROP CONSTRAINT IF EXISTS library_public_releases_slides_sha256_check,
    DROP CONSTRAINT IF EXISTS library_public_releases_index_sha256_check,
    DROP CONSTRAINT IF EXISTS library_public_releases_catalog_sha256_check,
    DROP CONSTRAINT IF EXISTS library_public_releases_manifest_sha256_check,
    DROP COLUMN IF EXISTS activation_digest,
    DROP COLUMN IF EXISTS oss_commit_sha256,
    DROP COLUMN IF EXISTS slides_sha256,
    DROP COLUMN IF EXISTS index_sha256,
    DROP COLUMN IF EXISTS catalog_sha256,
    DROP COLUMN IF EXISTS manifest_sha256;
