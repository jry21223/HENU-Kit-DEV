-- sqlc cannot infer DDL nested inside the migration's transactional DO block.
-- This compiler-only overlay describes the resulting schema; production
-- migrations continue to use db/migrations as their sole execution source.
ALTER TABLE quizcraft_learning_state
    ADD COLUMN IF NOT EXISTS latest_attempt_id uuid NOT NULL;
