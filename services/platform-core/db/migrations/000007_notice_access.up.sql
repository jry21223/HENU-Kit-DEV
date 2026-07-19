INSERT INTO permission_codes (code, description) VALUES
    ('notice.read', 'Read Notice lifecycle data within Notice product Scope'),
    ('notice.manage', 'Create Notice sources and immutable versions within Notice product Scope'),
    ('notice.review', 'Review Notice versions within Notice product Scope'),
    ('notice.distribute', 'Distribute approved Notice versions within Notice product Scope')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
