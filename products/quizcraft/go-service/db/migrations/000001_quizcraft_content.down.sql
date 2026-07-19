ALTER TABLE IF EXISTS quizcraft_banks DROP CONSTRAINT IF EXISTS quizcraft_banks_active_version_fk;
DROP TABLE IF EXISTS quizcraft_bank_version_questions;
DROP TABLE IF EXISTS quizcraft_question_versions;
DROP TABLE IF EXISTS quizcraft_questions;
DROP TABLE IF EXISTS quizcraft_bank_versions;
DROP TABLE IF EXISTS quizcraft_banks;
DROP FUNCTION IF EXISTS quizcraft_guard_membership_insert();
DROP FUNCTION IF EXISTS quizcraft_guard_bank_version_update();
DROP FUNCTION IF EXISTS quizcraft_reject_immutable_mutation();
