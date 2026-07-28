DELETE FROM role_permissions WHERE permission_code = 'account.membership.write';
DELETE FROM permission_codes WHERE code = 'account.membership.write';
