ALTER TABLE quizcraft_learning_state
    DROP CONSTRAINT IF EXISTS quizcraft_learning_state_latest_attempt_fk;

ALTER TABLE quizcraft_learning_state
    DROP COLUMN IF EXISTS latest_attempt_id;

-- A raw rollback is only valid when its migration receipt is removed too;
-- otherwise the schema runner would incorrectly skip restoring this column.
DO $$ BEGIN
    DELETE FROM quizcraft_schema_migrations WHERE version = '000011';
EXCEPTION WHEN undefined_table THEN NULL;
END $$;
