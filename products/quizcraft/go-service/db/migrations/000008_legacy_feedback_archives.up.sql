CREATE TABLE IF NOT EXISTS quizcraft_legacy_feedback_archives (
    id uuid PRIMARY KEY,
    source_name text NOT NULL CHECK (source_name <> ''),
    legacy_feedback_id text NOT NULL CHECK (legacy_feedback_id <> ''),
    bank_key text NOT NULL DEFAULT '',
    source_question_id text NOT NULL DEFAULT '',
    question_index integer NOT NULL CHECK (question_index > 0),
    question_content text NOT NULL DEFAULT '',
    legacy_user_id text NOT NULL DEFAULT '',
    source_page text NOT NULL DEFAULT 'quiz',
    detail text NOT NULL CHECK (char_length(detail) BETWEEN 1 AND 4000),
    legacy_status text NOT NULL CHECK (legacy_status = 'archived'),
    created_at timestamptz NOT NULL,
    resolved_at timestamptz,
    resolution_note text NOT NULL DEFAULT '',
    archived_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(source_name, legacy_feedback_id)
);

ALTER TABLE quizcraft_migration_exception_resolutions
    DROP CONSTRAINT quizcraft_migration_exception_resolutions_resolution_check;
ALTER TABLE quizcraft_migration_exception_resolutions
    ADD CONSTRAINT quizcraft_migration_exception_resolutions_resolution_check
    CHECK (resolution IN ('reference_resolved','archive_preserved'));

DO $$ BEGIN
    CREATE TRIGGER quizcraft_legacy_feedback_archives_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_legacy_feedback_archives
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
