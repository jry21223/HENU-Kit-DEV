INSERT INTO permission_codes(code, description, status) VALUES
    ('account.orders.read', 'Read Account Portfolio membership order and refund state within Account Portfolio product Scope', 'active'),
    ('account.orders.close', 'Close an unpaid Account Portfolio membership order within Account Portfolio product Scope', 'active'),
    ('account.orders.refund', 'Refund a paid Account Portfolio membership order within Account Portfolio product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
