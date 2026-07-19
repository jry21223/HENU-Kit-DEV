CREATE TABLE IF NOT EXISTS quizcraft_ranking_profiles (
    user_id uuid PRIMARY KEY,
    nickname text NOT NULL CHECK (nickname = btrim(nickname) AND char_length(nickname) BETWEEN 1 AND 32),
    system_avatar text NOT NULL CHECK (system_avatar IN ('scholar-blue','coder-green','reader-amber','owl-purple')),
    visible boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS quizcraft_practice_attempts_ranking_overall_idx
    ON quizcraft_practice_attempts(submitted_at,user_id)
    WHERE correct AND user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS quizcraft_practice_attempts_ranking_bank_idx
    ON quizcraft_practice_attempts(bank_id,submitted_at,user_id)
    WHERE correct AND user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS quizcraft_ranking_settlement_events (
    id uuid PRIMARY KEY,
    period_start timestamptz NOT NULL,
    period_end timestamptz NOT NULL CHECK (period_end > period_start),
    scope text NOT NULL CHECK (scope IN ('overall','bank')),
    bank_id uuid REFERENCES quizcraft_banks(id),
    metric text NOT NULL CHECK (metric = 'correct_answer_count'),
    standings jsonb NOT NULL CHECK (jsonb_typeof(standings) = 'array'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((scope='overall' AND bank_id IS NULL) OR (scope='bank' AND bank_id IS NOT NULL)),
    UNIQUE NULLS NOT DISTINCT(period_start,period_end,scope,bank_id)
);

DO $$ BEGIN
    CREATE TRIGGER quizcraft_ranking_settlement_events_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_ranking_settlement_events
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
