INSERT INTO food_submissions (
    id, venue_name, item_name, description, status, version, submitted_at, updated_at
) VALUES (
    '11111111-1111-4111-8111-111111111111', '北苑餐厅', '胡辣汤',
    'Gateway 到 Food Owner 的集成测试投稿', 'pending', 1,
    '2026-08-09T00:00:00Z', '2026-08-09T00:00:00Z'
), (
    '22222222-2222-4222-8222-222222222222', '南苑餐厅', '烩面',
    'Gateway 到 Food Owner 的拒绝流程集成测试投稿', 'pending', 1,
    '2026-08-09T00:00:00Z', '2026-08-09T00:00:00Z'
)
ON CONFLICT (id) DO UPDATE SET
    status = 'pending', version = 1, campus = EXCLUDED.campus, updated_at = EXCLUDED.updated_at;
