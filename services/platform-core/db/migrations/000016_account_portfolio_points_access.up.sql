INSERT INTO permission_codes(code, description, status) VALUES
    ('account.points.adjust', 'Adjust Account Portfolio points within Account Portfolio product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
