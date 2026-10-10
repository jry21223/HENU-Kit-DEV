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
   '在苹果园东路。汤底够味，蒜泥很香，十几块就能吃饱。', '人均18',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-02')::uuid, '姐妹酸辣粉', 'minglun', '人上人',
   '老河大西门夜市那家，便宜又好吃，逛夜市顺路来一碗。', '人均11',
   md5('portal-food-post-author:沐阳')::uuid, '沐阳', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-03')::uuid, '川奇麻辣香锅', 'jinming', '人上人',
   '在东环路北段学府苑。第一次去建议点微辣。', '人均23',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-04')::uuid, '李萍饭店', 'minglun', '人上人',
   '在明伦西门顺河公寓旁边，适合几个舍友一起去聚餐。', '人均40',
   md5('portal-food-post-author:沐阳')::uuid, '沐阳', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-05')::uuid, '学五香扒饭', 'jinming', '夯',
   '就在学五食堂的窗口，下课直接去，不用出校门。', '人均12',
   md5('portal-food-post-author:流年')::uuid, '流年', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-06')::uuid, '袁记水饺', 'jinming', '夯',
   '在东环路学府苑。连锁店，每次去味道都差不多，不会踩雷。', '人均15',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now()),
  (md5('portal-food-post:survey-07')::uuid, '金牌烧鹅', 'jinming', '夯',
   '在劳动路北段千禧广场。我几乎天天去，到现在还没吃腻。', '人均18',
   md5('portal-food-post-author:😕')::uuid, '😕', false, '2026-08-05 09:59:14.962683+00', now())
ON CONFLICT (id) DO NOTHING;
