DROP TABLE IF EXISTS quizcraft_shadow_gate_reports;
DROP TABLE IF EXISTS quizcraft_migration_event_receipts;
DROP TABLE IF EXISTS quizcraft_legacy_ranking_snapshots;
DROP TABLE IF EXISTS quizcraft_legacy_feedback_state_events;
DROP TABLE IF EXISTS quizcraft_migration_exception_resolutions;
DROP TABLE IF EXISTS quizcraft_migration_exceptions;
DROP TABLE IF EXISTS quizcraft_migration_runs;
DROP INDEX IF EXISTS quizcraft_feedbacks_legacy_id_idx;
ALTER TABLE quizcraft_feedbacks
    DROP COLUMN IF EXISTS legacy_resolution_note,
    DROP COLUMN IF EXISTS legacy_resolved_at,
    DROP COLUMN IF EXISTS legacy_status,
    DROP COLUMN IF EXISTS legacy_feedback_id;
