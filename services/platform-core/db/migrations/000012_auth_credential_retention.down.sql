BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM verification_codes WHERE sensitive_cleared_at IS NOT NULL)
       OR EXISTS (SELECT 1 FROM mail_outbox WHERE payload_cleared_at IS NOT NULL) THEN
        RAISE EXCEPTION '000012 rollback is unavailable after auth retention cleanup; restore the pre-migration backup or roll forward';
    END IF;
END;
$$;

ALTER TABLE oauth_exchange_idempotency
    DROP CONSTRAINT IF EXISTS oauth_exchange_response_discarded;
DROP TRIGGER IF EXISTS oauth_exchange_credential_discard ON oauth_exchange_idempotency;
DROP FUNCTION IF EXISTS discard_oauth_exchange_credential();

ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS mail_outbox_payload_retention_shape,
    ALTER COLUMN payload_ciphertext SET NOT NULL,
    DROP COLUMN IF EXISTS payload_cleared_at;

ALTER TABLE verification_codes
    DROP CONSTRAINT IF EXISTS verification_codes_login_session_shape,
    DROP CONSTRAINT IF EXISTS verification_codes_secret_retention_shape;
DROP TRIGGER IF EXISTS verification_session_credential_discard ON verification_codes;
DROP FUNCTION IF EXISTS discard_verification_session_credential();

UPDATE verification_codes
SET login_session_id = NULL,
    login_session_token_ciphertext = NULL
WHERE login_session_id IS NOT NULL;

ALTER TABLE verification_codes
    ALTER COLUMN request_key SET NOT NULL,
    ALTER COLUMN request_fingerprint SET NOT NULL,
    ALTER COLUMN code_nonce SET NOT NULL,
    ALTER COLUMN code_hash SET NOT NULL,
    ADD CONSTRAINT verification_codes_consumption_shape CHECK (
        (used_at IS NULL AND consumed_request_key IS NULL AND consumed_request_fingerprint IS NULL)
        OR (used_at IS NOT NULL AND consumed_request_key IS NOT NULL AND consumed_request_fingerprint IS NOT NULL)
    ),
    ADD CONSTRAINT verification_codes_login_session_shape CHECK (
        (login_session_id IS NULL AND login_session_token_ciphertext IS NULL)
        OR
        (purpose = 'login' AND login_session_id IS NOT NULL AND octet_length(login_session_token_ciphertext) > 28)
    ),
    DROP COLUMN IF EXISTS sensitive_cleared_at;

COMMIT;
