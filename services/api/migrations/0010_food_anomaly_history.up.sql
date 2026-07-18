ALTER TABLE food_vote_anomalies
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS food_tier_adjustments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    entry_id uuid NOT NULL REFERENCES food_entries(id), round_id uuid NOT NULL REFERENCES food_calibration_rounds(id),
    from_tier_id uuid NOT NULL REFERENCES food_tier_definitions(id), to_tier_id uuid NOT NULL REFERENCES food_tier_definitions(id),
    direction varchar(20) NOT NULL, actor_id uuid NOT NULL REFERENCES users(id), adjusted_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_food_tier_adjustments_entry ON food_tier_adjustments(entry_id, adjusted_at DESC);

