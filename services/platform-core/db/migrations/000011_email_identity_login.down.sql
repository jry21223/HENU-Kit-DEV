DROP TRIGGER operator_bootstrap_audit_events_immutable ON operator_bootstrap_audit_events;
DROP FUNCTION reject_operator_bootstrap_audit_mutation();
DROP TABLE operator_bootstrap_audit_events;

ALTER TABLE verification_codes
    DROP CONSTRAINT verification_codes_login_session_shape,
    DROP COLUMN login_session_token_ciphertext,
    DROP COLUMN login_session_id;

DROP TABLE email_identities;
