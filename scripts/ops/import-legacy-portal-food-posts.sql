-- One-way import of the seven frozen legacy portal_food_posts rows into the
-- Food service's food_posts table (the ADR-0032 "unify the seven legacy rows"
-- follow-up). The legacy table stays frozen and is never written.
--
-- Mapping: shop_name -> venue_name, legacy tier tag -> tier, excerpt ->
-- review_text, "人均N" from the title -> price_reference, legacy author ->
-- author_display_name. Legacy rows have no platform account, so each legacy
-- author maps to a deterministic UUID derived from their display name; posts
-- are public (hidden = false) and keep their original created_at.
--
-- Idempotent: ids are deterministic md5-derived UUIDs, so re-running the
-- script never duplicates a row.
--
-- Run against the food database:
--   psql -U henukit -d food -v ON_ERROR_STOP=1 -f import-legacy-portal-food-posts.sql
-- Then migrate the five production-proven legacy photos with the separately
-- fail-closed import-legacy-portal-food-images.mjs tool. It verifies that
-- these deterministic post IDs exist before it writes any image row.

INSERT INTO food_posts (
  id, venue_name, campus, tier, review_text, price_reference,
  author_user_id, author_display_name, hidden, created_at, updated_at
)
VALUES
  (md5('portal-food-post:survey-01')::uuid, '老成都麻辣烫', 'minglun', '顶级',
   '苹果园东路，汤味足、蒜泥香，价格实惠，评级顶级。', '人均18',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-02')::uuid, '姐妹酸辣粉', 'minglun', '人上人',
   '老河大西门夜市，好吃实惠，评级人上人。', '人均11',
   md5('portal-food-post-author:沐阳')::uuid, '沐阳', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-03')::uuid, '川奇麻辣香锅', 'jinming', '人上人',
   '东环路北段学府苑，推荐微辣，评级人上人。', '人均23',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-04')::uuid, '李萍饭店', 'minglun', '人上人',
   '明伦西门顺河公寓旁，适合舍友聚餐，评级人上人。', '人均40',
   md5('portal-food-post-author:沐阳')::uuid, '沐阳', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-05')::uuid, '学五香扒饭', 'jinming', '夯',
   '学五食堂窗口，评级夯。', '人均12',
   md5('portal-food-post-author:流年')::uuid, '流年', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-06')::uuid, '袁记水饺', 'jinming', '夯',
   '东环路学府苑，连锁出品稳定，评级夯。', '人均15',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-07')::uuid, '金牌烧鹅', 'jinming', '夯',
   '劳动路北段千禧广场，几乎每天吃不腻，评级夯。', '人均18',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now())
ON CONFLICT (id) DO NOTHING;
