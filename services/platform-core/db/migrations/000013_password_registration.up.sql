BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS display_name text,
    DROP CONSTRAINT IF EXISTS users_display_name_shape,
    ADD CONSTRAINT users_display_name_shape CHECK (
        display_name IS NULL OR char_length(display_name) BETWEEN 1 AND 80
    );

CREATE TABLE password_credentials (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    verifier text NOT NULL CHECK (verifier LIKE '$argon2id$%'),
    policy_version integer NOT NULL CHECK (policy_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE verification_codes
    DROP CONSTRAINT IF EXISTS verification_codes_purpose_check,
    DROP CONSTRAINT IF EXISTS verification_codes_login_session_shape,
    ADD CONSTRAINT verification_codes_purpose_check CHECK (
        purpose IN ('register', 'login', 'bind_email', 'security')
    ),
    ADD CONSTRAINT verification_codes_login_session_shape CHECK (
        (login_session_id IS NULL AND login_session_token_ciphertext IS NULL)
        OR
        (purpose IN ('register', 'login') AND login_session_id IS NOT NULL AND octet_length(login_session_token_ciphertext) = 0)
    );

COMMIT;
