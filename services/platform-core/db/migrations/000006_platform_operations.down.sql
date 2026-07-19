DO $$
BEGIN
    IF to_regclass('public.permission_codes') IS NOT NULL THEN
        DELETE FROM role_permissions WHERE permission_code IN ('platform.operations.read', 'platform.operations.write');
        DELETE FROM permission_codes WHERE code IN ('platform.operations.read', 'platform.operations.write');
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS platform_operations_audit_events_immutable ON platform_operations_audit_events;
DROP FUNCTION IF EXISTS reject_platform_operations_audit_mutation();
DROP TABLE IF EXISTS platform_operations_audit_events;
DROP TABLE IF EXISTS platform_operations_idempotency;
