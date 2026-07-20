ALTER TABLE quizcraft_feedbacks
    ADD COLUMN IF NOT EXISTS legacy_feedback_id text,
    ADD COLUMN IF NOT EXISTS legacy_status text NOT NULL DEFAULT 'pending'
        CHECK (legacy_status IN ('pending','resolved','archived')),
    ADD COLUMN IF NOT EXISTS legacy_resolved_at timestamptz,
    ADD COLUMN IF NOT EXISTS legacy_resolution_note text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS quizcraft_feedbacks_legacy_id_idx
    ON quizcraft_feedbacks(legacy_feedback_id) WHERE legacy_feedback_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS quizcraft_migration_runs (
    id uuid PRIMARY KEY,
    source_name text NOT NULL CHECK (source_name <> ''),
    source_cutoff_event_id bigint NOT NULL CHECK (source_cutoff_event_id >= 0),
    caught_up_event_id bigint NOT NULL CHECK (caught_up_event_id >= source_cutoff_event_id),
    state text NOT NULL CHECK (state IN ('running','passed','blocked')),
    report jsonb NOT NULL DEFAULT '{}'::jsonb,
    report_sha256 text NOT NULL DEFAULT '' CHECK (report_sha256 = '' OR report_sha256 ~ '^[0-9a-f]{64}$'),
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CHECK ((state = 'running' AND completed_at IS NULL) OR (state <> 'running' AND completed_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS quizcraft_migration_exceptions (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES quizcraft_migration_runs(id),
    record_type text NOT NULL CHECK (record_type IN ('feedback')),
    legacy_record_id text NOT NULL CHECK (legacy_record_id <> ''),
    reason_code text NOT NULL CHECK (reason_code IN ('missing_bank_reference','missing_question_reference','ambiguous_question_reference','invalid_record')),
    detail jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id,record_type,legacy_record_id)
);

CREATE TABLE IF NOT EXISTS quizcraft_legacy_feedback_state_events (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES quizcraft_migration_runs(id),
    legacy_feedback_id text NOT NULL CHECK (legacy_feedback_id <> ''),
    source_event_id bigint NOT NULL CHECK (source_event_id >= 0),
    status text NOT NULL CHECK (status IN ('pending','resolved','archived')),
    resolved_at timestamptz,
    resolution_note text NOT NULL DEFAULT '',
    recorded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id,legacy_feedback_id,source_event_id)
);

CREATE TABLE IF NOT EXISTS quizcraft_migration_exception_resolutions (
    id uuid PRIMARY KEY,
    exception_id uuid NOT NULL UNIQUE REFERENCES quizcraft_migration_exceptions(id),
    resolved_by_event_id bigint NOT NULL CHECK (resolved_by_event_id > 0),
    resolution text NOT NULL CHECK (resolution = 'reference_resolved'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS quizcraft_legacy_ranking_snapshots (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES quizcraft_migration_runs(id),
    source_event_id bigint NOT NULL CHECK (source_event_id >= 0),
    standings jsonb NOT NULL,
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    captured_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id,source_event_id)
);

CREATE TABLE IF NOT EXISTS quizcraft_migration_event_receipts (
    source_name text NOT NULL,
    source_event_id bigint NOT NULL CHECK (source_event_id > 0),
    run_id uuid NOT NULL REFERENCES quizcraft_migration_runs(id),
    event_type text NOT NULL CHECK (event_type IN ('bank.upserted','feedback.upserted','ranking.changed')),
    aggregate_key text NOT NULL CHECK (aggregate_key <> ''),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    applied_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(source_name,source_event_id)
);

CREATE TABLE IF NOT EXISTS quizcraft_shadow_gate_reports (
    id uuid PRIMARY KEY,
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL CHECK (window_end > window_start),
    sample_count bigint NOT NULL CHECK (sample_count >= 0),
    mismatch_count bigint NOT NULL CHECK (mismatch_count >= 0),
    legacy_error_count bigint NOT NULL CHECK (legacy_error_count >= 0),
    mismatch_rate double precision NOT NULL CHECK (mismatch_rate >= 0 AND mismatch_rate <= 1),
    mismatch_threshold double precision NOT NULL CHECK (mismatch_threshold >= 0 AND mismatch_threshold <= 1),
    minimum_sample_count bigint NOT NULL CHECK (minimum_sample_count > 0),
    decision text NOT NULL CHECK (decision IN ('pass','block')),
    reasons jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

DO $$ BEGIN
    CREATE TRIGGER quizcraft_migration_exceptions_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_migration_exceptions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_legacy_feedback_state_events_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_legacy_feedback_state_events
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_migration_exception_resolutions_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_migration_exception_resolutions
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_legacy_ranking_snapshots_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_legacy_ranking_snapshots
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_migration_event_receipts_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_migration_event_receipts
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TRIGGER quizcraft_shadow_gate_reports_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_shadow_gate_reports
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
