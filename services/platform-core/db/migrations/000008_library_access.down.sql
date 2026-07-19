DELETE FROM role_permissions WHERE permission_code IN ('library.read', 'library.manage', 'library.review');
DELETE FROM permission_codes WHERE code IN ('library.read', 'library.manage', 'library.review');
