-- Intentionally retained. The idempotent up migration cannot distinguish a
-- role it created from a pre-existing ungranted role, so deleting either on
-- rollback would violate ownership. An empty role grants no access.
DO $$ BEGIN END $$;
