ALTER TABLE portal_campus_items
    ADD COLUMN IF NOT EXISTS author_user_id uuid,
    ADD COLUMN IF NOT EXISTS accepted_by_user_id uuid,
    ADD COLUMN IF NOT EXISTS accepted_by_name text,
    ADD COLUMN IF NOT EXISTS accepted_at timestamptz,
    ADD COLUMN IF NOT EXISTS moderation_note text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1 CHECK (version >= 1);

ALTER TABLE portal_campus_messages
    ADD COLUMN IF NOT EXISTS author_user_id uuid,
    ADD COLUMN IF NOT EXISTS hidden boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS portal_campus_contacts (
    user_id uuid PRIMARY KEY,
    qq text NOT NULL DEFAULT '',
    wechat text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    note text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS portal_campus_exchanges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id text NOT NULL REFERENCES portal_campus_items(id) ON DELETE CASCADE,
    requester_user_id uuid NOT NULL,
    requester_name text NOT NULL DEFAULT '',
    owner_user_id uuid,
    status text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'accepted', 'declined', 'cancelled')),
    requester_channels jsonb NOT NULL DEFAULT '[]'::jsonb,
    owner_channels jsonb NOT NULL DEFAULT '[]'::jsonb,
    requester_snapshot jsonb,
    owner_snapshot jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (item_id, requester_user_id)
);

CREATE INDEX IF NOT EXISTS portal_campus_exchanges_item_idx ON portal_campus_exchanges (item_id, created_at DESC);

CREATE TABLE IF NOT EXISTS portal_campus_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id text,
    actor_user_id uuid NOT NULL,
    action text NOT NULL,
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
