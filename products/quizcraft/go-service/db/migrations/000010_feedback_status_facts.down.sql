DROP TRIGGER IF EXISTS quizcraft_feedback_status_facts_immutable ON quizcraft_feedback_status_facts;
DROP TABLE IF EXISTS quizcraft_feedback_status_facts;

DO $$ BEGIN
    DELETE FROM quizcraft_schema_migrations WHERE version = '000010';
EXCEPTION WHEN undefined_table THEN NULL;
END $$;
