DELETE FROM role_permissions WHERE permission_code IN ('account.tickets.read', 'account.tickets.reply', 'account.tickets.transition');
DELETE FROM permission_codes WHERE code IN ('account.tickets.read', 'account.tickets.reply', 'account.tickets.transition');
