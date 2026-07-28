CREATE TABLE IF NOT EXISTS quizcraft_feedback_status_facts (
    id uuid PRIMARY KEY,
    feedback_id uuid NOT NULL REFERENCES quizcraft_feedbacks(id) ON DELETE RESTRICT,
    status text NOT NULL CHECK (status IN ('pending','in_progress','blocked','resolved','archived')),
    source text NOT NULL CHECK (source IN ('submission','operations_inbox','legacy_migration')),
    source_event_id text NOT NULL CHECK (char_length(source_event_id) BETWEEN 1 AND 300),
    source_version bigint,
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(feedback_id, source_event_id),
    CHECK (
        (source = 'operations_inbox' AND source_version IS NOT NULL AND source_version > 0)
        OR (source <> 'operations_inbox' AND source_version IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS quizcraft_feedback_status_facts_current_idx
    ON quizcraft_feedback_status_facts(feedback_id, recorded_at DESC, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS quizcraft_feedback_status_facts_inbox_version_idx
    ON quizcraft_feedback_status_facts(feedback_id, source_version DESC)
    WHERE source = 'operations_inbox';

DO $$ BEGIN
    CREATE TRIGGER quizcraft_feedback_status_facts_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_feedback_status_facts
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

WITH baseline AS (
    SELECT
        f.*,
        COALESCE(state.status, f.legacy_status) AS status_fact_status,
        COALESCE(state.resolved_at, f.legacy_resolved_at, f.created_at) AS status_fact_occurred_at,
        COALESCE(state.source_event_id, 0) AS status_source_event_id,
        md5('quizcraft-feedback-status:' || f.id::text || ':baseline') AS status_fact_hash
    FROM quizcraft_feedbacks AS f
    LEFT JOIN LATERAL (
        SELECT status,resolved_at,source_event_id
        FROM quizcraft_legacy_feedback_state_events
        WHERE f.legacy_feedback_id IS NOT NULL
          AND legacy_feedback_id=f.legacy_feedback_id
        ORDER BY source_event_id DESC,recorded_at DESC,id DESC
        LIMIT 1
    ) AS state ON true
)
INSERT INTO quizcraft_feedback_status_facts(id, feedback_id, status, source, source_event_id, occurred_at)
SELECT
    (
        substr(status_fact_hash, 1, 8) || '-' ||
        substr(status_fact_hash, 9, 4) || '-' ||
        substr(status_fact_hash, 13, 4) || '-' ||
        substr(status_fact_hash, 17, 4) || '-' ||
        substr(status_fact_hash, 21, 12)
    )::uuid,
    id,
    status_fact_status,
    CASE WHEN legacy_feedback_id IS NULL THEN 'submission' ELSE 'legacy_migration' END,
    CASE WHEN legacy_feedback_id IS NULL THEN 'baseline:' || id::text ELSE 'legacy-migration:' || id::text || ':' || status_source_event_id::text END,
    status_fact_occurred_at
FROM baseline
ON CONFLICT(feedback_id, source_event_id) DO NOTHING;
