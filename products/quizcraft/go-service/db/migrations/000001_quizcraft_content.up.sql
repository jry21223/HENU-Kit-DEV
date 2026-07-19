CREATE TABLE IF NOT EXISTS quizcraft_banks (
    id uuid PRIMARY KEY,
    bank_key text NOT NULL UNIQUE CHECK (bank_key ~ '^[a-z0-9][a-z0-9-]{1,78}[a-z0-9]$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
    active_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS quizcraft_bank_versions (
    id uuid PRIMARY KEY,
    bank_id uuid NOT NULL REFERENCES quizcraft_banks(id),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
    source_version text NOT NULL DEFAULT '',
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    import_report jsonb NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now(),
    sealed_at timestamptz,
    UNIQUE(bank_id, content_sha256),
    UNIQUE(bank_id, id)
);

DO $$ BEGIN
    ALTER TABLE quizcraft_banks ADD CONSTRAINT quizcraft_banks_active_version_fk FOREIGN KEY(id, active_version_id) REFERENCES quizcraft_bank_versions(bank_id, id);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS quizcraft_questions (
    id uuid PRIMARY KEY,
    bank_id uuid NOT NULL REFERENCES quizcraft_banks(id),
    source_question_id text NOT NULL CHECK (char_length(source_question_id) BETWEEN 1 AND 160),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(bank_id, source_question_id),
    UNIQUE(bank_id, id)
);

CREATE TABLE IF NOT EXISTS quizcraft_question_versions (
    id uuid PRIMARY KEY,
    bank_id uuid NOT NULL,
    question_id uuid NOT NULL,
    type text NOT NULL CHECK (type IN ('single','multi','judge','blank')),
    chapter_id text NOT NULL CHECK (char_length(chapter_id) BETWEEN 1 AND 160),
    chapter_name text NOT NULL CHECK (char_length(chapter_name) BETWEEN 1 AND 240),
    content text NOT NULL CHECK (char_length(content) BETWEEN 1 AND 10000),
    options jsonb,
    answer jsonb NOT NULL,
    analysis text NOT NULL DEFAULT '',
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(question_id, content_sha256),
    UNIQUE(bank_id, question_id, id),
    FOREIGN KEY(bank_id, question_id) REFERENCES quizcraft_questions(bank_id, id)
);

CREATE TABLE IF NOT EXISTS quizcraft_bank_version_questions (
    bank_id uuid NOT NULL,
    bank_version_id uuid NOT NULL,
    question_id uuid NOT NULL,
    question_version_id uuid NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    PRIMARY KEY(bank_version_id, question_id),
    UNIQUE(bank_version_id, position),
    FOREIGN KEY(bank_id, bank_version_id) REFERENCES quizcraft_bank_versions(bank_id, id),
    FOREIGN KEY(bank_id, question_id) REFERENCES quizcraft_questions(bank_id, id),
    FOREIGN KEY(bank_id, question_id, question_version_id) REFERENCES quizcraft_question_versions(bank_id, question_id, id)
);
CREATE INDEX IF NOT EXISTS quizcraft_bank_version_questions_order_idx ON quizcraft_bank_version_questions(bank_version_id, position);

CREATE OR REPLACE FUNCTION quizcraft_reject_immutable_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'QuizCraft immutable content cannot be updated, deleted, or truncated' USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION quizcraft_guard_bank_version_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.sealed_at IS NULL
       AND NEW.sealed_at IS NOT NULL
       AND (to_jsonb(NEW) - 'sealed_at') = (to_jsonb(OLD) - 'sealed_at') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'QuizCraft bank versions are immutable after creation' USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION quizcraft_guard_membership_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    version_sealed_at timestamptz;
    existing quizcraft_bank_version_questions%ROWTYPE;
BEGIN
    SELECT sealed_at INTO version_sealed_at
      FROM quizcraft_bank_versions
     WHERE id = NEW.bank_version_id
     FOR SHARE;
    IF version_sealed_at IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT * INTO existing
      FROM quizcraft_bank_version_questions
     WHERE bank_version_id = NEW.bank_version_id AND question_id = NEW.question_id;
    IF FOUND
       AND existing.bank_id = NEW.bank_id
       AND existing.question_version_id = NEW.question_version_id
       AND existing.position = NEW.position THEN
        RETURN NULL;
    END IF;
    RAISE EXCEPTION 'QuizCraft sealed bank version membership cannot be extended' USING ERRCODE = '55000';
END;
$$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_bank_versions_update_guard
        BEFORE UPDATE ON quizcraft_bank_versions
        FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_bank_version_update();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_bank_versions_delete_guard
        BEFORE DELETE OR TRUNCATE ON quizcraft_bank_versions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_question_versions_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_question_versions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_bank_version_questions_insert_guard
        BEFORE INSERT ON quizcraft_bank_version_questions
        FOR EACH ROW EXECUTE FUNCTION quizcraft_guard_membership_insert();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_bank_version_questions_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_bank_version_questions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
