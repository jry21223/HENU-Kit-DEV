CREATE TABLE verification_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_lookup_hash bytea NOT NULL CHECK (octet_length(email_lookup_hash) = 32),
    purpose text NOT NULL CHECK (purpose IN ('login', 'bind_email', 'security')),
    request_key text NOT NULL UNIQUE CHECK (length(request_key) BETWEEN 8 AND 200),
    request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    code_nonce bytea NOT NULL CHECK (octet_length(code_nonce) = 16),
    code_hash bytea NOT NULL CHECK (octet_length(code_hash) = 32),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    consumed_request_key text UNIQUE CHECK (consumed_request_key IS NULL OR length(consumed_request_key) BETWEEN 8 AND 200),
    consumed_request_fingerprint bytea CHECK (consumed_request_fingerprint IS NULL OR octet_length(consumed_request_fingerprint) = 32),
    revoked_at timestamptz,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT verification_codes_lifecycle CHECK (used_at IS NULL OR revoked_at IS NULL),
    CONSTRAINT verification_codes_consumption_shape CHECK (
        (used_at IS NULL AND consumed_request_key IS NULL AND consumed_request_fingerprint IS NULL)
        OR (used_at IS NOT NULL AND consumed_request_key IS NOT NULL AND consumed_request_fingerprint IS NOT NULL)
    )
);

CREATE INDEX verification_codes_lookup_idx
ON verification_codes (email_lookup_hash, purpose, created_at DESC);

CREATE TABLE mail_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    verification_code_id uuid NOT NULL UNIQUE REFERENCES verification_codes(id) ON DELETE RESTRICT,
    dedupe_key text NOT NULL UNIQUE,
    request_id text NOT NULL CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    kind text NOT NULL CHECK (kind = 'verification_code'),
    priority text NOT NULL CHECK (priority = 'critical'),
    recipient_ciphertext bytea NOT NULL,
    payload_ciphertext bytea NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'retry_due', 'accepted', 'delivered', 'failed')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 20),
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    provider_message_id text,
    accepted_at timestamptz,
    delivered_at timestamptz,
    failed_at timestamptz,
    last_error_code text CHECK (last_error_code IS NULL OR last_error_code ~ '^[A-Z0-9_]+$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mail_outbox_lock_shape CHECK (
        (status = 'processing' AND locked_at IS NOT NULL AND locked_by IS NOT NULL)
        OR (status <> 'processing' AND locked_at IS NULL AND locked_by IS NULL)
    ),
    CONSTRAINT mail_outbox_delivery_shape CHECK (
        (status IN ('pending', 'processing', 'retry_due') AND accepted_at IS NULL AND delivered_at IS NULL AND failed_at IS NULL)
        OR (status = 'accepted' AND accepted_at IS NOT NULL AND delivered_at IS NULL AND failed_at IS NULL)
        OR (status = 'delivered' AND accepted_at IS NOT NULL AND delivered_at IS NOT NULL AND failed_at IS NULL)
        OR (status = 'failed' AND delivered_at IS NULL AND failed_at IS NOT NULL)
    )
);

CREATE INDEX mail_outbox_claim_idx
ON mail_outbox (priority, available_at, created_at)
WHERE status IN ('pending', 'retry_due', 'processing');

CREATE UNIQUE INDEX mail_outbox_provider_message_idx
ON mail_outbox (provider_message_id)
WHERE provider_message_id IS NOT NULL;
