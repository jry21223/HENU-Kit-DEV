CREATE TABLE IF NOT EXISTS library_public_releases (
    release_id text PRIMARY KEY CHECK (release_id ~ '^[a-f0-9]{40}-[a-f0-9]{16}$'),
    receipt_sha256 text NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
    state text NOT NULL CHECK (state IN ('active', 'retired')),
    activated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS library_public_releases_one_active_idx
    ON library_public_releases ((state)) WHERE state = 'active';

CREATE TABLE IF NOT EXISTS library_public_material_snapshots (
    release_id text NOT NULL REFERENCES library_public_releases(release_id),
    material_id uuid NOT NULL,
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    file_name text NOT NULL CHECK (char_length(file_name) BETWEEN 1 AND 255),
    access_level text NOT NULL CHECK (access_level IN ('public_free', 'authenticated', 'restricted')),
    status text NOT NULL CHECK (status IN ('published', 'withdrawn')),
    object_key text NOT NULL CHECK (char_length(object_key) BETWEEN 1 AND 1024 AND object_key LIKE 'releases/' || release_id || '/%'),
    object_version_id text NOT NULL CHECK (char_length(object_version_id) BETWEEN 1 AND 1024),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    byte_size bigint NOT NULL CHECK (byte_size >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (release_id, material_id),
    UNIQUE (release_id, object_key)
);

CREATE TABLE IF NOT EXISTS library_download_start_events (
    id uuid PRIMARY KEY,
    material_id uuid NOT NULL,
    release_id text NOT NULL,
    receipt_sha256 text NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
    object_version_id text NOT NULL CHECK (char_length(object_version_id) BETWEEN 1 AND 1024),
    request_id text NOT NULL CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    issued_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > issued_at AND expires_at <= issued_at + interval '60 seconds'),
    FOREIGN KEY (release_id, material_id)
        REFERENCES library_public_material_snapshots(release_id, material_id)
);

CREATE INDEX IF NOT EXISTS library_download_start_events_material_idx
    ON library_download_start_events (material_id, issued_at);

CREATE INDEX IF NOT EXISTS library_download_start_events_issued_idx
    ON library_download_start_events (issued_at);

CREATE OR REPLACE FUNCTION library_download_start_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'library download-start ledger is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS library_download_start_events_append_only ON library_download_start_events;
CREATE TRIGGER library_download_start_events_append_only
BEFORE UPDATE OR DELETE OR TRUNCATE ON library_download_start_events
FOR EACH STATEMENT EXECUTE FUNCTION library_download_start_append_only();
