DELETE FROM role_permissions WHERE permission_code IN ('food.read', 'food.review', 'food.anomaly', 'food.tier_adjust');
DELETE FROM permission_codes WHERE code IN ('food.read', 'food.review', 'food.anomaly', 'food.tier_adjust');
