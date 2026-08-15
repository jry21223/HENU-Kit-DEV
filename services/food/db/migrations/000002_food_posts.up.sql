-- Food Post: user-created public food reviews (issue #385).
--
-- A post is public the moment it is created: `hidden` is always false and the
-- row never enters a draft or review state. Images live in Postgres (HENU Kit
-- has no object storage), bounded to 2 MiB per photo and 6 photos per post.

CREATE TABLE IF NOT EXISTS food_posts (
    id uuid PRIMARY KEY,
    venue_name text NOT NULL CHECK (char_length(venue_name) BETWEEN 1 AND 160),
    campus text NOT NULL CHECK (campus IN ('minglun','jinming','longzihu')),
    tier text NOT NULL CHECK (tier IN ('夯','顶级','人上人','NPC','拉完了')),
    review_text text NOT NULL CHECK (char_length(review_text) BETWEEN 2 AND 2000),
    price_reference text NOT NULL DEFAULT '' CHECK (char_length(price_reference) <= 200),
    hours_reference text NOT NULL DEFAULT '' CHECK (char_length(hours_reference) <= 200),
    author_user_id uuid NOT NULL,
    author_display_name text NOT NULL CHECK (char_length(author_display_name) BETWEEN 1 AND 120),
    hidden boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_food_posts_campus_created ON food_posts (campus, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_food_posts_author_created ON food_posts (author_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS food_post_dishes (
    id uuid PRIMARY KEY,
    post_id uuid NOT NULL REFERENCES food_posts(id) ON DELETE CASCADE,
    position int NOT NULL CHECK (position BETWEEN 0 AND 5),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    price text NOT NULL DEFAULT '' CHECK (char_length(price) <= 40),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 200),
    UNIQUE (post_id, position)
);
CREATE INDEX IF NOT EXISTS idx_food_post_dishes_post ON food_post_dishes (post_id, position);

CREATE TABLE IF NOT EXISTS food_post_images (
    id uuid PRIMARY KEY,
    post_id uuid NOT NULL REFERENCES food_posts(id) ON DELETE CASCADE,
    position int NOT NULL CHECK (position BETWEEN 0 AND 5),
    content_type text NOT NULL CHECK (content_type IN ('image/jpeg','image/png','image/webp')),
    byte_size int NOT NULL CHECK (byte_size > 0 AND byte_size <= 2097152),
    sha256 text NOT NULL CHECK (char_length(sha256) = 64),
    bytes bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (post_id, position)
);
CREATE INDEX IF NOT EXISTS idx_food_post_images_post ON food_post_images (post_id, position);

CREATE TABLE IF NOT EXISTS food_post_operations (
    id uuid PRIMARY KEY,
    client_id text NOT NULL,
    actor_user_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    request_id text NOT NULL,
    post_id uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('succeeded','failed')),
    error_code text,
    response jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (client_id, actor_user_id, idempotency_key)
);
