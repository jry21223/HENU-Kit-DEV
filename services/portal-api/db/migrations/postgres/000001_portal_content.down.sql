-- Reverses 000001_portal_content.up.sql.
--
-- Children first: portal_food_comments and portal_campus_messages carry
-- REFERENCES back to their parents, and portal_food_post_images (000002) also
-- references portal_food_posts, so 000002 must be reversed before this file.
--
-- Indexes are dropped explicitly ahead of their tables so a migration that only
-- partially applied still reverses cleanly.
DROP INDEX IF EXISTS idx_portal_campus_messages_item;
DROP TABLE IF EXISTS portal_campus_messages;

DROP INDEX IF EXISTS idx_portal_campus_items_status;
DROP INDEX IF EXISTS idx_portal_campus_items_type;
DROP TABLE IF EXISTS portal_campus_items;

DROP INDEX IF EXISTS idx_portal_food_comments_post;
DROP TABLE IF EXISTS portal_food_comments;

DROP INDEX IF EXISTS idx_portal_food_posts_hidden;
DROP INDEX IF EXISTS idx_portal_food_posts_campus;
DROP TABLE IF EXISTS portal_food_posts;
