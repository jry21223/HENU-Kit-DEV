CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS campus_notices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    source_id uuid NOT NULL, external_id varchar(240) NOT NULL, organization_id uuid, title varchar(300) NOT NULL,
    original_url varchar(1000) NOT NULL DEFAULT '', original_published_at timestamptz, current_version integer NOT NULL DEFAULT 1,
    content_hash varchar(64) NOT NULL, status varchar(40) NOT NULL DEFAULT 'review_pending',
    distribution_status varchar(40) NOT NULL DEFAULT 'not_scheduled', importance varchar(32) NOT NULL DEFAULT 'normal', version integer NOT NULL DEFAULT 1,
    CONSTRAINT uq_campus_notices_source_external UNIQUE (source_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_campus_notices_status ON campus_notices(status);
CREATE INDEX IF NOT EXISTS idx_campus_notices_distribution ON campus_notices(distribution_status);

CREATE TABLE IF NOT EXISTS campus_notice_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    notice_id uuid NOT NULL REFERENCES campus_notices(id), version integer NOT NULL, title varchar(300) NOT NULL,
    body text NOT NULL, content_hash varchar(64) NOT NULL, object_keys jsonb,
    CONSTRAINT uq_campus_notice_versions UNIQUE (notice_id, version)
);

CREATE TABLE IF NOT EXISTS notice_import_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    status varchar(40) NOT NULL, total_rows integer NOT NULL DEFAULT 0, created_rows integer NOT NULL DEFAULT 0,
    updated_rows integer NOT NULL DEFAULT 0, duplicate_rows integer NOT NULL DEFAULT 0, failed_rows integer NOT NULL DEFAULT 0,
    requested_by uuid NOT NULL, error_summary varchar(1000) NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS mail_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    category varchar(32) NOT NULL, status varchar(40) NOT NULL, recipient_hash varchar(64) NOT NULL,
    template_code varchar(120) NOT NULL, request_id varchar(128) NOT NULL, attempt_count integer NOT NULL DEFAULT 0,
    queued_at timestamptz NOT NULL, accepted_at timestamptz, delivered_at timestamptz, next_retry_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_mail_deliveries_queue ON mail_deliveries(category, status, queued_at);

CREATE TABLE IF NOT EXISTS mail_dead_letters (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    delivery_id uuid NOT NULL UNIQUE REFERENCES mail_deliveries(id), status varchar(40) NOT NULL DEFAULT 'open', reason_code varchar(120) NOT NULL, resolved_at timestamptz
);

CREATE TABLE IF NOT EXISTS platform_feedbacks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    user_id uuid, category varchar(80) NOT NULL, summary varchar(500) NOT NULL, content text NOT NULL,
    urgency varchar(20) NOT NULL CHECK (urgency IN ('urgent','normal')), status varchar(40) NOT NULL DEFAULT 'new',
    assignee_id uuid, due_at timestamptz NOT NULL, resolved_at timestamptz, request_id varchar(128) NOT NULL DEFAULT '', version integer NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_platform_feedback_due ON platform_feedbacks(status, due_at);

CREATE TABLE IF NOT EXISTS operation_cases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    source_service varchar(80) NOT NULL, source_type varchar(80) NOT NULL, source_id varchar(160) NOT NULL,
    summary varchar(500) NOT NULL, urgency varchar(20) NOT NULL CHECK (urgency IN ('urgent','normal')),
    status varchar(40) NOT NULL DEFAULT 'open', assignee_id uuid, due_at timestamptz NOT NULL, resolved_at timestamptz,
    action_path varchar(500) NOT NULL, version integer NOT NULL DEFAULT 1,
    CONSTRAINT uq_operation_case_source UNIQUE (source_service, source_type, source_id)
);
CREATE INDEX IF NOT EXISTS idx_operation_cases_due ON operation_cases(status, due_at);

CREATE TABLE IF NOT EXISTS food_tier_definitions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    code varchar(60) NOT NULL UNIQUE, name varchar(80) NOT NULL, sort_order integer NOT NULL UNIQUE, enabled boolean NOT NULL DEFAULT true
);
INSERT INTO food_tier_definitions (id, code, name, sort_order) VALUES
    (gen_random_uuid(), 'hang', '夯', 10),
    (gen_random_uuid(), 'ren_shang_ren', '人上人', 20),
    (gen_random_uuid(), 'top', '顶级', 30),
    (gen_random_uuid(), 'npc', 'NPC', 40),
    (gen_random_uuid(), 'la_wan_le', '拉完了', 50)
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order;

CREATE TABLE IF NOT EXISTS food_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    submitter_id uuid NOT NULL, name varchar(200) NOT NULL, location varchar(500) NOT NULL, suggested_tier_id uuid NOT NULL REFERENCES food_tier_definitions(id),
    reason text NOT NULL, image_object_key varchar(500) NOT NULL DEFAULT '', status varchar(40) NOT NULL DEFAULT 'pending', version integer NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_food_submissions_status ON food_submissions(status, created_at);

CREATE TABLE IF NOT EXISTS food_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    submission_id uuid NOT NULL UNIQUE REFERENCES food_submissions(id), name varchar(200) NOT NULL, location varchar(500) NOT NULL,
    initial_tier_id uuid NOT NULL REFERENCES food_tier_definitions(id), current_tier_id uuid NOT NULL REFERENCES food_tier_definitions(id),
    last_adjusted_at timestamptz, version integer NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS food_calibration_rounds (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    entry_id uuid NOT NULL REFERENCES food_entries(id), round_number integer NOT NULL, status varchar(40) NOT NULL DEFAULT 'open',
    policy_version varchar(80) NOT NULL DEFAULT 'food_calibration_v1', opened_at timestamptz NOT NULL, closed_at timestamptz,
    CONSTRAINT uq_food_entry_round UNIQUE (entry_id, round_number)
);

CREATE TABLE IF NOT EXISTS food_calibration_votes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    round_id uuid NOT NULL REFERENCES food_calibration_rounds(id), user_id uuid NOT NULL,
    position varchar(32) NOT NULL CHECK (position IN ('underrated','about_right','overrated')),
    status varchar(32) NOT NULL DEFAULT 'valid' CHECK (status IN ('valid','suspected','invalidated','restored')),
    CONSTRAINT uq_food_round_user UNIQUE (round_id, user_id)
);

CREATE TABLE IF NOT EXISTS food_vote_anomalies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    round_id uuid NOT NULL REFERENCES food_calibration_rounds(id), rule_code varchar(120) NOT NULL,
    severity varchar(20) NOT NULL, status varchar(32) NOT NULL DEFAULT 'open', blocking boolean NOT NULL DEFAULT false, evidence jsonb
);

CREATE TABLE IF NOT EXISTS service_heartbeats (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    service_id varchar(100) NOT NULL UNIQUE, status varchar(32) NOT NULL, service_version varchar(80) NOT NULL DEFAULT '', commit_sha varchar(80) NOT NULL DEFAULT '',
    deployment_time timestamptz, last_ready_at timestamptz NOT NULL, outbox_pending bigint NOT NULL DEFAULT 0, worker_anomalies bigint NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS idempotency_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    actor_id uuid NOT NULL, method varchar(16) NOT NULL, route varchar(300) NOT NULL, key varchar(200) NOT NULL,
    request_hash varchar(64) NOT NULL, state varchar(24) NOT NULL, status_code integer NOT NULL DEFAULT 0, response_body text NOT NULL DEFAULT '',
    CONSTRAINT uq_idempotency_scope UNIQUE (actor_id, method, route, key)
);
CREATE INDEX IF NOT EXISTS idx_idempotency_records_state ON idempotency_records(state, created_at);
