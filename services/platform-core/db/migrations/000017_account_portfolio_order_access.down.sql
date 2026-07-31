DELETE FROM role_permissions WHERE permission_code IN ('account.orders.read', 'account.orders.close', 'account.orders.refund');
DELETE FROM permission_codes WHERE code IN ('account.orders.read', 'account.orders.close', 'account.orders.refund');
