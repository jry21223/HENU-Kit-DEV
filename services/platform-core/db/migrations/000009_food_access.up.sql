INSERT INTO permission_codes(code, description, status) VALUES
    ('food.read', 'Read Food operations within Food product Scope', 'active'),
    ('food.review', 'Review Food submissions within Food product Scope', 'active'),
    ('food.anomaly', 'Resolve Food anomaly tickets within Food product Scope', 'active'),
    ('food.tier_adjust', 'Confirm Food tier adjustments within Food product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description=EXCLUDED.description, status='active';
