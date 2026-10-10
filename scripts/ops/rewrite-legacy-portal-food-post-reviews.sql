-- Rewrites the review_text of the seven legacy seed food posts imported by
-- import-legacy-portal-food-posts.sql into plainer first-person copy. That
-- import uses ON CONFLICT DO NOTHING, so already-imported rows need this script.
--
-- Guarded: each UPDATE only matches the deterministic id AND the original
-- seed text, so a row an owner has since edited is left untouched.
-- Idempotent: a second run matches nothing.
--
-- Run against the food database:
--   psql -U henukit -d food -v ON_ERROR_STOP=1 -f rewrite-legacy-portal-food-post-reviews.sql

BEGIN;

UPDATE food_posts SET review_text = '在苹果园东路。汤底够味，蒜泥很香，十几块就能吃饱。', updated_at = now()
WHERE id = md5('portal-food-post:survey-01')::uuid
  AND review_text = '苹果园东路，汤味足、蒜泥香，价格实惠，评级顶级。';

UPDATE food_posts SET review_text = '老河大西门夜市那家，便宜又好吃，逛夜市顺路来一碗。', updated_at = now()
WHERE id = md5('portal-food-post:survey-02')::uuid
  AND review_text = '老河大西门夜市，好吃实惠，评级人上人。';

UPDATE food_posts SET review_text = '在东环路北段学府苑。第一次去建议点微辣。', updated_at = now()
WHERE id = md5('portal-food-post:survey-03')::uuid
  AND review_text = '东环路北段学府苑，推荐微辣，评级人上人。';

UPDATE food_posts SET review_text = '在明伦西门顺河公寓旁边，适合几个舍友一起去聚餐。', updated_at = now()
WHERE id = md5('portal-food-post:survey-04')::uuid
  AND review_text = '明伦西门顺河公寓旁，适合舍友聚餐，评级人上人。';

UPDATE food_posts SET review_text = '就在学五食堂的窗口，下课直接去，不用出校门。', updated_at = now()
WHERE id = md5('portal-food-post:survey-05')::uuid
  AND review_text = '学五食堂窗口，评级夯。';

UPDATE food_posts SET review_text = '在东环路学府苑。连锁店，每次去味道都差不多，不会踩雷。', updated_at = now()
WHERE id = md5('portal-food-post:survey-06')::uuid
  AND review_text = '东环路学府苑，连锁出品稳定，评级夯。';

UPDATE food_posts SET review_text = '在劳动路北段千禧广场。我几乎天天去，到现在还没吃腻。', updated_at = now()
WHERE id = md5('portal-food-post:survey-07')::uuid
  AND review_text = '劳动路北段千禧广场，几乎每天吃不腻，评级夯。';

COMMIT;
