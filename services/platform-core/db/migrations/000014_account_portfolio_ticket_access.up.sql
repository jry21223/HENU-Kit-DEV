INSERT INTO permission_codes(code, description, status) VALUES
    ('account.tickets.read', 'Read the Account Portfolio support-ticket queue within Account Portfolio product Scope', 'active'),
    ('account.tickets.reply', 'Reply to Account Portfolio support tickets within Account Portfolio product Scope', 'active'),
    ('account.tickets.transition', 'Transition Account Portfolio support tickets within Account Portfolio product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
