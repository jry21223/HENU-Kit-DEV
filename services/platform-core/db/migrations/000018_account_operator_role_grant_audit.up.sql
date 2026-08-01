CREATE TABLE IF NOT EXISTS account_operator_role_grant_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id uuid NOT NULL REFERENCES authorization_roles(id) ON DELETE RESTRICT,
    role_code text NOT NULL,
    actor text NOT NULL CHECK (length(actor) BETWEEN 1 AND 120),
    request_id text NOT NULL UNIQUE CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    reason text NOT NULL CHECK (length(reason) BETWEEN 8 AND 500),
    permission_codes text[] NOT NULL CHECK (cardinality(permission_codes) = 8),
    changed boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION reject_account_operator_role_grant_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'account operator role grant audit events are append-only';
END;
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'account_operator_role_grant_audit_events_immutable'
          AND tgrelid = 'account_operator_role_grant_audit_events'::regclass
    ) THEN
        CREATE TRIGGER account_operator_role_grant_audit_events_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON account_operator_role_grant_audit_events
        FOR EACH STATEMENT EXECUTE FUNCTION reject_account_operator_role_grant_audit_mutation();
    END IF;
END;
$$;
