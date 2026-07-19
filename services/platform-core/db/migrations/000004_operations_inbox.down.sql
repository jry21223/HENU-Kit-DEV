DROP TRIGGER IF EXISTS operations_inbox_audit_events_immutable ON operations_inbox_audit_events;
DROP FUNCTION IF EXISTS reject_operations_inbox_audit_mutation();
DROP TABLE IF EXISTS operations_inbox_audit_events;
DROP TABLE IF EXISTS operations_inbox_idempotency;
DROP TABLE IF EXISTS operations_inbox_items;
DO $$
BEGIN
    IF to_regclass('public.permission_codes') IS NOT NULL THEN
        DELETE FROM role_permissions WHERE permission_code IN ('platform.operations_inbox.read', 'platform.operations_inbox.write');
        DELETE FROM permission_codes WHERE code IN ('platform.operations_inbox.read', 'platform.operations_inbox.write');
    END IF;
END;
$$;
