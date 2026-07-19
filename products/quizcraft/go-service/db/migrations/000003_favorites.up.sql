CREATE TABLE IF NOT EXISTS quizcraft_favorites (
    user_id uuid NOT NULL,
    bank_id uuid NOT NULL,
    question_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, bank_id, question_id),
    FOREIGN KEY (bank_id, question_id) REFERENCES quizcraft_questions(bank_id, id)
);

CREATE INDEX IF NOT EXISTS quizcraft_favorites_user_created_idx
    ON quizcraft_favorites(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS quizcraft_practice_session_claims (
    session_id uuid PRIMARY KEY REFERENCES quizcraft_practice_sessions(id),
    guest_actor_key text NOT NULL CHECK (guest_actor_key ~ '^guest:[0-9a-f-]{36}$'),
    user_id uuid NOT NULL,
    user_actor_key text NOT NULL CHECK (user_actor_key = 'user:' || user_id::text),
    claimed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(session_id, guest_actor_key),
    UNIQUE(session_id, user_actor_key)
);

DO $$ BEGIN
    CREATE TRIGGER quizcraft_practice_session_claims_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_practice_session_claims
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
