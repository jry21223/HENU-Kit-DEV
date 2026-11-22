INSERT INTO permission_codes (code, description, status) VALUES
    ('career.read', 'Read sanitized Career Work Radar monitoring metrics and task history within Career product Scope', 'active'),
    ('career.manage', 'Retry Career digest deliveries and manage operational task state within Career product Scope', 'active'),
    ('campus.read', 'Read Campus Help & Marketplace listings and moderation queue within Campus product Scope', 'active'),
    ('campus.moderate', 'Hide, restore, or close Campus listings and messages within Campus product Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';
