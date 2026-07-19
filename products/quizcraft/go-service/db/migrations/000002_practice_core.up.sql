DO $$ BEGIN
    ALTER TABLE quizcraft_bank_version_questions
        ADD CONSTRAINT quizcraft_bank_version_questions_identity_unique
        UNIQUE(bank_id, bank_version_id, question_id, question_version_id);
EXCEPTION WHEN duplicate_object OR duplicate_table THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS quizcraft_practice_sessions (
    id uuid PRIMARY KEY,
    bank_id uuid NOT NULL,
    bank_version_id uuid NOT NULL,
    user_id uuid,
    actor_key text NOT NULL CHECK (actor_key ~ '^(guest|user):[0-9a-f-]{36}$'),
    mode text NOT NULL CHECK (mode IN ('random','difficult','chapter','favorites')),
    chapter_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id, bank_id, bank_version_id),
    FOREIGN KEY(bank_id, bank_version_id) REFERENCES quizcraft_bank_versions(bank_id, id)
);

CREATE TABLE IF NOT EXISTS quizcraft_practice_session_questions (
    session_id uuid NOT NULL REFERENCES quizcraft_practice_sessions(id),
    bank_id uuid NOT NULL,
    bank_version_id uuid NOT NULL,
    question_id uuid NOT NULL,
    question_version_id uuid NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    PRIMARY KEY(session_id, question_id),
    UNIQUE(session_id, bank_id, bank_version_id, question_id, question_version_id),
    UNIQUE(session_id, position),
    FOREIGN KEY(session_id, bank_id, bank_version_id) REFERENCES quizcraft_practice_sessions(id, bank_id, bank_version_id),
    FOREIGN KEY(bank_id, bank_version_id, question_id, question_version_id)
        REFERENCES quizcraft_bank_version_questions(bank_id, bank_version_id, question_id, question_version_id)
);

CREATE TABLE IF NOT EXISTS quizcraft_practice_attempts (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL,
    bank_id uuid NOT NULL,
    bank_version_id uuid NOT NULL,
    question_id uuid NOT NULL,
    question_version_id uuid NOT NULL,
    user_id uuid,
    submitted_answer jsonb NOT NULL,
    correct boolean NOT NULL,
    expected_answer jsonb NOT NULL,
    analysis text NOT NULL DEFAULT '',
    response_body text NOT NULL CHECK (response_body IS JSON),
    submitted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(session_id, question_id),
    FOREIGN KEY(session_id, bank_id, bank_version_id, question_id, question_version_id)
        REFERENCES quizcraft_practice_session_questions(session_id, bank_id, bank_version_id, question_id, question_version_id),
    FOREIGN KEY(bank_id, question_id, question_version_id) REFERENCES quizcraft_question_versions(bank_id, question_id, id)
);

CREATE TABLE IF NOT EXISTS quizcraft_idempotency_results (
    actor_key text NOT NULL,
    operation_kind text NOT NULL,
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 16 AND 160),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    response_status integer NOT NULL CHECK (response_status BETWEEN 100 AND 599),
    response_body text NOT NULL CHECK (response_body IS JSON),
    resource_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(actor_key, operation_kind, idempotency_key)
);

CREATE TABLE IF NOT EXISTS quizcraft_question_stats (
    question_id uuid PRIMARY KEY REFERENCES quizcraft_questions(id),
    attempt_count bigint NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    correct_count bigint NOT NULL DEFAULT 0 CHECK (correct_count >= 0 AND correct_count <= attempt_count),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS quizcraft_learning_state (
    user_id uuid NOT NULL,
    bank_id uuid NOT NULL,
    question_id uuid NOT NULL,
    question_version_id uuid NOT NULL,
    wrong boolean NOT NULL,
    attempt_count bigint NOT NULL CHECK (attempt_count > 0),
    correct_count bigint NOT NULL CHECK (correct_count >= 0 AND correct_count <= attempt_count),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id, bank_id, question_id),
    FOREIGN KEY(bank_id, question_id, question_version_id) REFERENCES quizcraft_question_versions(bank_id, question_id, id)
);

CREATE TABLE IF NOT EXISTS quizcraft_shadow_comparisons (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES quizcraft_practice_sessions(id),
    question_id uuid NOT NULL REFERENCES quizcraft_questions(id),
    new_response jsonb NOT NULL,
    legacy_response jsonb,
    outcome text NOT NULL CHECK (outcome IN ('match','mismatch','legacy_error')),
    detail text NOT NULL DEFAULT '',
    compared_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS quizcraft_shadow_comparisons_session_idx ON quizcraft_shadow_comparisons(session_id, compared_at);

DO $$ BEGIN
    CREATE TRIGGER quizcraft_practice_sessions_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_practice_sessions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_practice_session_questions_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_practice_session_questions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_practice_attempts_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_practice_attempts
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_idempotency_results_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_idempotency_results
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_shadow_comparisons_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_shadow_comparisons
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
