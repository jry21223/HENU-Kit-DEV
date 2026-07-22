BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.verification_codes') IS NULL
       OR to_regclass('public.mail_outbox') IS NULL
       OR to_regclass('public.oauth_exchange_idempotency') IS NULL
       OR NOT EXISTS (
           SELECT 1 FROM information_schema.columns
           WHERE table_schema = 'public' AND table_name = 'verification_codes'
             AND column_name = 'login_session_token_ciphertext'
       ) THEN
        RAISE EXCEPTION '000012 requires migrations 000001, 000003, and 000011';
    END IF;
END;
$$;

ALTER TABLE verification_codes
    ADD COLUMN IF NOT EXISTS sensitive_cleared_at timestamptz,
    DROP CONSTRAINT IF EXISTS verification_codes_login_session_shape,
    DROP CONSTRAINT IF EXISTS verification_codes_consumption_shape,
    DROP CONSTRAINT IF EXISTS verification_codes_secret_retention_shape,
    ALTER COLUMN request_key DROP NOT NULL,
    ALTER COLUMN request_fingerprint DROP NOT NULL,
    ALTER COLUMN code_nonce DROP NOT NULL,
    ALTER COLUMN code_hash DROP NOT NULL;

UPDATE verification_codes
SET login_session_token_ciphertext = ''::bytea
WHERE login_session_token_ciphertext IS NOT NULL;

CREATE OR REPLACE FUNCTION discard_verification_session_credential() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.login_session_token_ciphertext IS NOT NULL THEN
        NEW.login_session_token_ciphertext := ''::bytea;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS verification_session_credential_discard ON verification_codes;
CREATE TRIGGER verification_session_credential_discard
BEFORE INSERT OR UPDATE OF login_session_token_ciphertext ON verification_codes
FOR EACH ROW EXECUTE FUNCTION discard_verification_session_credential();

ALTER TABLE verification_codes
    ADD CONSTRAINT verification_codes_login_session_shape CHECK (
        (login_session_id IS NULL AND login_session_token_ciphertext IS NULL)
        OR
        (purpose = 'login' AND login_session_id IS NOT NULL AND octet_length(login_session_token_ciphertext) = 0)
    ),
    ADD CONSTRAINT verification_codes_secret_retention_shape CHECK (
        (
            sensitive_cleared_at IS NULL
            AND request_key IS NOT NULL
            AND request_fingerprint IS NOT NULL
            AND code_nonce IS NOT NULL
            AND code_hash IS NOT NULL
            AND (
                (used_at IS NULL AND consumed_request_key IS NULL AND consumed_request_fingerprint IS NULL)
                OR
                (used_at IS NOT NULL AND consumed_request_key IS NOT NULL AND consumed_request_fingerprint IS NOT NULL)
            )
        )
        OR
        (
            sensitive_cleared_at IS NOT NULL
            AND request_key IS NULL
            AND request_fingerprint IS NULL
            AND code_nonce IS NULL
            AND code_hash IS NULL
            AND consumed_request_key IS NULL
            AND consumed_request_fingerprint IS NULL
        )
    );

ALTER TABLE mail_outbox
    ADD COLUMN IF NOT EXISTS payload_cleared_at timestamptz,
    DROP CONSTRAINT IF EXISTS mail_outbox_payload_retention_shape,
    ALTER COLUMN payload_ciphertext DROP NOT NULL;

ALTER TABLE mail_outbox
    ADD CONSTRAINT mail_outbox_payload_retention_shape CHECK (
        (payload_cleared_at IS NULL AND payload_ciphertext IS NOT NULL)
        OR (payload_cleared_at IS NOT NULL AND payload_ciphertext IS NULL)
    );

UPDATE oauth_exchange_idempotency
SET response_ciphertext = ''::bytea
WHERE octet_length(response_ciphertext) <> 0;

CREATE OR REPLACE FUNCTION discard_oauth_exchange_credential() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.response_ciphertext := ''::bytea;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS oauth_exchange_credential_discard ON oauth_exchange_idempotency;
CREATE TRIGGER oauth_exchange_credential_discard
BEFORE INSERT OR UPDATE OF response_ciphertext ON oauth_exchange_idempotency
FOR EACH ROW EXECUTE FUNCTION discard_oauth_exchange_credential();

ALTER TABLE oauth_exchange_idempotency
    DROP CONSTRAINT IF EXISTS oauth_exchange_response_discarded,
    ADD CONSTRAINT oauth_exchange_response_discarded CHECK (octet_length(response_ciphertext) = 0);

COMMIT;
