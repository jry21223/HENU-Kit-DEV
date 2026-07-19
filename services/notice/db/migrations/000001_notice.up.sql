CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS notice_sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE CHECK (code ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    canonical_url text NOT NULL CHECK (canonical_url ~ '^https://'),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notice_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES notice_sources(id),
    version integer NOT NULL CHECK (version > 0),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 100000),
    source_url text NOT NULL CHECK (source_url ~ '^https://'),
    content_hash text NOT NULL CHECK (content_hash ~ '^[a-f0-9]{64}$'),
    source_published_at timestamptz,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, version),
    UNIQUE (source_url, content_hash)
);

CREATE OR REPLACE FUNCTION reject_notice_version_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'notice versions are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS notice_versions_immutable_update ON notice_versions;
CREATE TRIGGER notice_versions_immutable_update
BEFORE UPDATE OR DELETE OR TRUNCATE ON notice_versions
FOR EACH STATEMENT EXECUTE FUNCTION reject_notice_version_mutation();

CREATE TABLE IF NOT EXISTS notice_lifecycles (
    notice_version_id uuid PRIMARY KEY REFERENCES notice_versions(id),
    state text NOT NULL DEFAULT 'pending_review' CHECK (state IN ('pending_review', 'approved', 'rejected', 'distributed')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notice_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notice_version_id uuid NOT NULL REFERENCES notice_versions(id),
    decision text NOT NULL CHECK (decision IN ('approved', 'rejected')),
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 1000),
    actor_user_id uuid NOT NULL,
    request_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notice_distributions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notice_version_id uuid NOT NULL REFERENCES notice_versions(id),
    channel text NOT NULL CHECK (channel IN ('in_app', 'email')),
    audience_kind text NOT NULL CHECK (audience_kind IN ('all_students', 'college', 'role')),
    audience_value text,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'delivered', 'failed')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    claimed_at timestamptz,
    completed_at timestamptz,
    last_error text,
    actor_user_id uuid NOT NULL,
    request_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((audience_kind = 'all_students' AND audience_value IS NULL) OR (audience_kind <> 'all_students' AND audience_value IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS notice_operations (
    client_id text NOT NULL,
    method text NOT NULL,
    normalized_route text NOT NULL,
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 200),
    operation text NOT NULL CHECK (operation IN ('source_create', 'version_create', 'review', 'distribution')),
    request_hash text NOT NULL CHECK (request_hash ~ '^[a-f0-9]{64}$'),
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '24 hours'),
    PRIMARY KEY (client_id, method, normalized_route, idempotency_key)
);

CREATE TABLE IF NOT EXISTS notice_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid NOT NULL,
    permission_code text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    request_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION reject_notice_audit_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'notice audit events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS notice_audit_append_only ON notice_audit_events;
CREATE TRIGGER notice_audit_append_only
BEFORE UPDATE OR DELETE OR TRUNCATE ON notice_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_notice_audit_mutation();

CREATE INDEX IF NOT EXISTS notice_versions_source_created_idx ON notice_versions (source_id, created_at DESC);
CREATE INDEX IF NOT EXISTS notice_reviews_version_created_idx ON notice_reviews (notice_version_id, created_at DESC);
CREATE INDEX IF NOT EXISTS notice_distributions_version_created_idx ON notice_distributions (notice_version_id, created_at DESC);
CREATE INDEX IF NOT EXISTS notice_distributions_queue_idx ON notice_distributions (created_at) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS notice_operations_expiry_idx ON notice_operations (expires_at);
