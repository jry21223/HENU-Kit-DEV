DELETE FROM role_permissions WHERE permission_code IN ('notice.read', 'notice.manage', 'notice.review', 'notice.distribute');
DELETE FROM permission_codes WHERE code IN ('notice.read', 'notice.manage', 'notice.review', 'notice.distribute');
