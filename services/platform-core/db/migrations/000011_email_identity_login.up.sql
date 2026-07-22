CREATE TABLE IF NOT EXISTS email_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email_lookup_hash bytea NOT NULL UNIQUE CHECK (octet_length(email_lookup_hash) = 32),
    email_ciphertext bytea NOT NULL CHECK (octet_length(email_ciphertext) > 28),
    verified_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS email_identities_user_idx ON email_identities (user_id);

ALTER TABLE verification_codes
    ADD COLUMN IF NOT EXISTS login_session_id uuid REFERENCES sessions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS login_session_token_ciphertext bytea;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'verification_codes_login_session_shape'
          AND conrelid = 'verification_codes'::regclass
    ) THEN
        ALTER TABLE verification_codes
            ADD CONSTRAINT verification_codes_login_session_shape CHECK (
                (login_session_id IS NULL AND login_session_token_ciphertext IS NULL)
                OR
                (purpose = 'login' AND login_session_id IS NOT NULL AND octet_length(login_session_token_ciphertext) > 28)
            );
    END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS operator_bootstrap_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    target_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    actor_unix_user text NOT NULL CHECK (length(actor_unix_user) BETWEEN 1 AND 120),
    request_id text NOT NULL CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    reason text NOT NULL CHECK (length(reason) BETWEEN 8 AND 500),
    permission_codes text[] NOT NULL CHECK (cardinality(permission_codes) > 0),
    scope_summary jsonb NOT NULL CHECK (jsonb_typeof(scope_summary) = 'array'),
    changed boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION reject_operator_bootstrap_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'operator bootstrap audit events are append-only';
END;
$$;

DROP TRIGGER IF EXISTS operator_bootstrap_audit_events_immutable ON operator_bootstrap_audit_events;
CREATE TRIGGER operator_bootstrap_audit_events_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON operator_bootstrap_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_operator_bootstrap_audit_mutation();
