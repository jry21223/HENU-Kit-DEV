-- The student role itself is canonical (docs/architecture/ACCESS_CONTROL.md)
-- and may pre-date this migration, so only the notice permission and its
-- role grant are removed.
DELETE FROM role_permissions WHERE permission_code = 'portal.notice.read';
DELETE FROM permission_codes WHERE code = 'portal.notice.read';
