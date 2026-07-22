DROP TRIGGER operator_bootstrap_audit_events_immutable ON operator_bootstrap_audit_events;
DROP FUNCTION reject_operator_bootstrap_audit_mutation();
DROP TABLE operator_bootstrap_audit_events;

ALTER TABLE verification_codes
    DROP CONSTRAINT IF EXISTS verification_codes_login_session_shape,
    DROP CONSTRAINT IF EXISTS verification_codes_secret_retention_shape;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM verification_codes WHERE sensitive_cleared_at IS NOT NULL) THEN
        RAISE EXCEPTION '000011 rollback is unavailable after auth retention cleanup; restore the pre-migration backup or roll forward';
    END IF;
END;
$$;

ALTER TABLE verification_codes
    ALTER COLUMN request_key SET NOT NULL,
    ALTER COLUMN request_fingerprint SET NOT NULL,
    ALTER COLUMN code_nonce SET NOT NULL,
    ALTER COLUMN code_hash SET NOT NULL;

ALTER TABLE verification_codes
    ADD CONSTRAINT verification_codes_consumption_shape CHECK (
        (used_at IS NULL AND consumed_request_key IS NULL AND consumed_request_fingerprint IS NULL)
        OR (used_at IS NOT NULL AND consumed_request_key IS NOT NULL AND consumed_request_fingerprint IS NOT NULL)
    ),
    DROP COLUMN IF EXISTS sensitive_cleared_at,
    DROP COLUMN IF EXISTS login_session_id;

DROP TABLE email_identities;
