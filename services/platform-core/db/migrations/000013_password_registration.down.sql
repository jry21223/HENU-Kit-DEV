BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM password_credentials)
       OR EXISTS (SELECT 1 FROM verification_codes WHERE purpose = 'register') THEN
        RAISE EXCEPTION '000013 rollback would discard registered password accounts; restore a pre-migration backup or roll forward';
    END IF;
END;
$$;

ALTER TABLE verification_codes
    DROP CONSTRAINT IF EXISTS verification_codes_login_session_shape,
    DROP CONSTRAINT IF EXISTS verification_codes_purpose_check,
    ADD CONSTRAINT verification_codes_purpose_check CHECK (
        purpose IN ('login', 'bind_email', 'security')
    ),
    ADD CONSTRAINT verification_codes_login_session_shape CHECK (
        (login_session_id IS NULL AND login_session_token_ciphertext IS NULL)
        OR
        (purpose = 'login' AND login_session_id IS NOT NULL AND octet_length(login_session_token_ciphertext) = 0)
    );

DROP TABLE password_credentials;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_display_name_shape,
    DROP COLUMN IF EXISTS display_name;

COMMIT;
