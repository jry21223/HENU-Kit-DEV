-- Photos attached to a Food post.
--
-- portal_food_posts.images holds absolute URLs, which only works for photos
-- already hosted somewhere. HENU Kit has no object storage, so student-supplied
-- photos had nowhere to live and posts shipped without them. The bytes live in
-- Postgres instead: the volume is small (a survey batch is a handful of photos
-- of a few hundred KB), and a table keeps every photo inside the same backup
-- and restore path as the post that references it.
--
-- Rows here take precedence over the images column, which stays for posts whose
-- photos are already hosted elsewhere.
CREATE TABLE IF NOT EXISTS portal_food_post_images (
    id TEXT PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES portal_food_posts(id) ON DELETE CASCADE,
    -- Display order within the post; 0 is the card thumbnail.
    position INT NOT NULL CHECK (position >= 0),
    content_type TEXT NOT NULL CHECK (
        content_type IN ('image/jpeg', 'image/png', 'image/webp')
    ),
    byte_size INT NOT NULL CHECK (byte_size > 0),
    -- Serves as the ETag, so a reader revalidates rather than refetching.
    sha256 TEXT NOT NULL CHECK (char_length(sha256) = 64),
    bytes BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (post_id, position)
);

CREATE INDEX IF NOT EXISTS idx_portal_food_post_images_post
    ON portal_food_post_images (post_id, position);
