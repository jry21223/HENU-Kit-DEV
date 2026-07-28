INSERT INTO permission_codes(code, description, status) VALUES
    ('account.membership.write', 'Grant or revoke Account Portfolio lifetime membership within Account Portfolio product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
