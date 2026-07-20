DROP TABLE IF EXISTS quizcraft_service_nonces;
DROP TABLE IF EXISTS quizcraft_feedback_inbox_deliveries;
DROP TABLE IF EXISTS quizcraft_feedback_inbox_outbox;
DROP TABLE IF EXISTS quizcraft_feedbacks;
DROP TABLE IF EXISTS quizcraft_workshop_audit_events;
DROP TABLE IF EXISTS quizcraft_workshop_version_states;
ALTER TABLE quizcraft_banks DROP COLUMN IF EXISTS lifecycle_version;
