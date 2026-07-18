CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_verified boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_clients (
    id text PRIMARY KEY,
    redirect_uris text[] NOT NULL CHECK (cardinality(redirect_uris) > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_client_keys (
    client_id text NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    key_id text NOT NULL,
    secret_hash bytea NOT NULL CHECK (octet_length(secret_hash) = 32),
    status text NOT NULL CHECK (status IN ('active', 'retiring', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (client_id, key_id)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('core', 'client_exchange')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    client_id text REFERENCES oauth_clients(id) ON DELETE CASCADE,
    parent_session_id uuid REFERENCES sessions(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sessions_kind_fields CHECK (
        (kind = 'core' AND client_id IS NULL AND parent_session_id IS NULL)
        OR
        (kind = 'client_exchange' AND client_id IS NOT NULL AND parent_session_id IS NOT NULL)
    )
);

CREATE INDEX sessions_active_user_idx ON sessions (user_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE authorization_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash bytea NOT NULL UNIQUE CHECK (octet_length(code_hash) = 32),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    core_session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    code_challenge text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX authorization_codes_expiry_idx ON authorization_codes (expires_at) WHERE used_at IS NULL;

CREATE TABLE oauth_exchange_idempotency (
    client_id text NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 200),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    response_ciphertext bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (client_id, idempotency_key)
);

CREATE INDEX oauth_exchange_idempotency_expiry_idx ON oauth_exchange_idempotency (expires_at);
