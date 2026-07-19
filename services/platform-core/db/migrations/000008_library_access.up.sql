INSERT INTO permission_codes (code, description) VALUES
    ('library.read', 'Read bounded Library data within Library product Scope'),
    ('library.manage', 'Manage courses and materials within Library product Scope'),
    ('library.review', 'Review Library submissions and corrections within Library product Scope')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
