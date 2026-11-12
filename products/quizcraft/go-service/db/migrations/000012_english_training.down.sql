-- Rollback requires a write freeze and backup; never erase live training facts.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM quizcraft_training_units) OR EXISTS(SELECT 1 FROM quizcraft_training_sessions) THEN
  RAISE EXCEPTION 'authored training exists; retain immutable bindings and use forward recovery';
 END IF;
END $$;
DROP TRIGGER IF EXISTS quizcraft_training_session_consistent ON quizcraft_practice_sessions;
DROP TRIGGER IF EXISTS quizcraft_training_membership_consistent ON quizcraft_practice_session_questions;
DROP TRIGGER IF EXISTS quizcraft_training_attempt_consistent ON quizcraft_practice_attempts;
DROP VIEW IF EXISTS quizcraft_expected_learning_state;
DROP VIEW IF EXISTS quizcraft_learning_attempt_facts;
DROP VIEW IF EXISTS quizcraft_ordinary_attempts;
DROP TABLE IF EXISTS quizcraft_training_disclosures;
DROP TABLE IF EXISTS quizcraft_training_outcomes;
DROP TABLE IF EXISTS quizcraft_training_sessions;
DROP TABLE IF EXISTS quizcraft_training_members;
DROP TABLE IF EXISTS quizcraft_training_unit_versions;
DROP TABLE IF EXISTS quizcraft_training_units;
DROP FUNCTION IF EXISTS quizcraft_guard_training_session();
DROP FUNCTION IF EXISTS quizcraft_guard_training_attempt();
ALTER TABLE quizcraft_practice_sessions DROP COLUMN IF EXISTS purpose;
DO $$ BEGIN
 DELETE FROM quizcraft_schema_migrations WHERE version='000012';
EXCEPTION WHEN undefined_table THEN NULL;
END $$;
