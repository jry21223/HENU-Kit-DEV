ALTER TABLE library_public_releases
    ADD COLUMN IF NOT EXISTS manifest_sha256 text,
    ADD COLUMN IF NOT EXISTS oss_commit_sha256 text,
    ADD COLUMN IF NOT EXISTS catalog_sha256 text,
    ADD COLUMN IF NOT EXISTS index_sha256 text,
    ADD COLUMN IF NOT EXISTS slides_sha256 text,
    ADD COLUMN IF NOT EXISTS activation_digest text;

DO $$ BEGIN
    ALTER TABLE library_public_releases ADD CONSTRAINT library_public_releases_oss_commit_sha256_check CHECK (oss_commit_sha256 IS NULL OR oss_commit_sha256 ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_releases ADD CONSTRAINT library_public_releases_manifest_sha256_check CHECK (manifest_sha256 IS NULL OR manifest_sha256 ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_releases ADD CONSTRAINT library_public_releases_catalog_sha256_check CHECK (catalog_sha256 IS NULL OR catalog_sha256 ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_releases ADD CONSTRAINT library_public_releases_index_sha256_check CHECK (index_sha256 IS NULL OR index_sha256 ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_releases ADD CONSTRAINT library_public_releases_slides_sha256_check CHECK (slides_sha256 IS NULL OR slides_sha256 ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_releases ADD CONSTRAINT library_public_releases_activation_digest_check CHECK (activation_digest IS NULL OR activation_digest ~ '^[0-9a-f]{64}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER TABLE library_public_material_snapshots
    ADD COLUMN IF NOT EXISTS subject text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS material_type text NOT NULL DEFAULT 'note',
    ADD COLUMN IF NOT EXISTS public_path text NOT NULL DEFAULT '';

DO $$ BEGIN
    ALTER TABLE library_public_material_snapshots ADD CONSTRAINT library_public_material_snapshots_subject_check CHECK (char_length(subject) <= 160);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_material_snapshots ADD CONSTRAINT library_public_material_snapshots_role_check CHECK (char_length(role) <= 160);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_material_snapshots ADD CONSTRAINT library_public_material_snapshots_type_check CHECK (material_type IN ('note', 'exam', 'mock', 'path', 'lab', 'slides'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TABLE library_public_material_snapshots ADD CONSTRAINT library_public_material_snapshots_public_path_check CHECK (char_length(public_path) <= 1024);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS library_public_release_activation_events (
    id uuid PRIMARY KEY,
    release_id text NOT NULL REFERENCES library_public_releases(release_id),
    previous_release_id text REFERENCES library_public_releases(release_id),
    activation_digest text NOT NULL CHECK (activation_digest ~ '^[0-9a-f]{64}$'),
    activated_at timestamptz NOT NULL,
    material_count integer NOT NULL CHECK (material_count >= 0)
);

CREATE INDEX IF NOT EXISTS library_public_release_activation_events_release_idx
    ON library_public_release_activation_events (release_id, activated_at);

CREATE OR REPLACE FUNCTION library_public_release_activation_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'library public release activation history is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS library_public_release_activation_events_append_only ON library_public_release_activation_events;
CREATE TRIGGER library_public_release_activation_events_append_only
BEFORE UPDATE OR DELETE OR TRUNCATE ON library_public_release_activation_events
FOR EACH STATEMENT EXECUTE FUNCTION library_public_release_activation_append_only();
