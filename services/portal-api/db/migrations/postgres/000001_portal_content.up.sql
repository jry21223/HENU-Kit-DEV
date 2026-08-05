-- Portal content tables for PostgreSQL (compose uses postgres://portal DB)
CREATE TABLE IF NOT EXISTS portal_food_posts (
    id TEXT PRIMARY KEY,
    campus TEXT NOT NULL CHECK (campus IN ('minglun', 'jinming', 'longzihu')),
    title TEXT NOT NULL,
    excerpt TEXT NOT NULL DEFAULT '',
    blocks JSONB NOT NULL DEFAULT '[]'::jsonb,
    author TEXT NOT NULL DEFAULT '',
    likes INT NOT NULL DEFAULT 0,
    stars DOUBLE PRECISION NOT NULL DEFAULT 0,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    shop_name TEXT NOT NULL DEFAULT '',
    shop_lat DOUBLE PRECISION NOT NULL DEFAULT 0,
    shop_lng DOUBLE PRECISION NOT NULL DEFAULT 0,
    time TEXT NOT NULL DEFAULT '',
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    images JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_portal_food_posts_campus ON portal_food_posts (campus);
CREATE INDEX IF NOT EXISTS idx_portal_food_posts_hidden ON portal_food_posts (hidden);

CREATE TABLE IF NOT EXISTS portal_food_comments (
    id TEXT PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES portal_food_posts(id) ON DELETE CASCADE,
    author TEXT NOT NULL DEFAULT '',
    time TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_portal_food_comments_post ON portal_food_comments (post_id);

CREATE TABLE IF NOT EXISTS portal_campus_items (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('help', 'sell')),
    category TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    "desc" TEXT NOT NULL DEFAULT '',
    price DOUBLE PRECISION NOT NULL DEFAULT 0,
    seller TEXT NOT NULL DEFAULT '',
    credit INT NOT NULL DEFAULT 80,
    deals_done INT NOT NULL DEFAULT 0,
    wants INT NOT NULL DEFAULT 0,
    place TEXT NOT NULL DEFAULT '',
    deadline TEXT,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'ongoing', 'done', 'hidden')),
    time TEXT NOT NULL DEFAULT '',
    images JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_portal_campus_items_type ON portal_campus_items (type);
CREATE INDEX IF NOT EXISTS idx_portal_campus_items_status ON portal_campus_items (status);

CREATE TABLE IF NOT EXISTS portal_campus_messages (
    id TEXT PRIMARY KEY,
    item_id TEXT NOT NULL REFERENCES portal_campus_items(id) ON DELETE CASCADE,
    author TEXT NOT NULL DEFAULT '',
    time TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_portal_campus_messages_item ON portal_campus_messages (item_id);
