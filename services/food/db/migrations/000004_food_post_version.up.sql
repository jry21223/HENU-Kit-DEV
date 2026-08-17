-- Food Post governance versioning (#414).
--
-- Console ops may correct or hide a published Food Post; each governance
-- write bumps this optimistic version so concurrent edits conflict like
-- every other Food operation.

ALTER TABLE food_posts ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK (version >= 1);
