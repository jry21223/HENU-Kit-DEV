CREATE TABLE IF NOT EXISTS portal_campus_item_images (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL REFERENCES portal_campus_items(id) ON DELETE CASCADE,
    position INT NOT NULL CHECK (position BETWEEN 0 AND 5),
    content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    byte_size INT NOT NULL CHECK (byte_size > 0 AND byte_size <= 4194304),
    sha256 TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    bytes BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The unique index also supports lookups by item_id and ordered position.
    UNIQUE (item_id, position)
);
