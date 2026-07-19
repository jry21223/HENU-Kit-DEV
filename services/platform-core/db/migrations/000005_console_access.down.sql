DO $$
BEGIN
    IF to_regclass('public.permission_codes') IS NOT NULL THEN
        DELETE FROM role_permissions WHERE permission_code = 'console.overview.read';
        DELETE FROM permission_codes WHERE code = 'console.overview.read';
    END IF;
END;
$$;
