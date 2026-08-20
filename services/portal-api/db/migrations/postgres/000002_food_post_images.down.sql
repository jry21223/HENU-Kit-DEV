-- Reverses 000002_food_post_images.up.sql.
--
-- Dropping the table discards every stored photo: the bytes live only here, so
-- rolling this back is a data-losing operation and needs a dump taken first.
-- The index goes with the table; it is named only so a partially applied
-- migration still reverses cleanly.
DROP INDEX IF EXISTS idx_portal_food_post_images_post;
DROP TABLE IF EXISTS portal_food_post_images;
