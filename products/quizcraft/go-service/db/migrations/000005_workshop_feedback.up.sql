ALTER TABLE quizcraft_banks
    ADD COLUMN IF NOT EXISTS lifecycle_version bigint NOT NULL DEFAULT 1 CHECK (lifecycle_version >= 1);

CREATE TABLE IF NOT EXISTS quizcraft_workshop_version_states (
    bank_id uuid NOT NULL,
    bank_version_id uuid PRIMARY KEY,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','validated')),
    created_by uuid NOT NULL,
    validated_by uuid,
    validation_note text NOT NULL DEFAULT '' CHECK (char_length(validation_note) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    validated_at timestamptz,
    FOREIGN KEY(bank_id, bank_version_id) REFERENCES quizcraft_bank_versions(bank_id, id),
    CHECK ((state='draft' AND validated_by IS NULL AND validated_at IS NULL) OR (state='validated' AND validated_by IS NOT NULL AND validated_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS quizcraft_workshop_audit_events (
    id uuid PRIMARY KEY,
    actor_user_id uuid NOT NULL,
    permission_code text NOT NULL,
    action text NOT NULL CHECK (action IN ('create_bank','create_version','import_version','validate_version','publish_version','unpublish_version','rollback_bank')),
    bank_id uuid NOT NULL REFERENCES quizcraft_banks(id),
    bank_version_id uuid,
    expected_version bigint NOT NULL CHECK (expected_version >= 0),
    resulting_version bigint NOT NULL CHECK (resulting_version = expected_version + 1),
    request_id text NOT NULL,
    note text NOT NULL DEFAULT '' CHECK (char_length(note) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(bank_id, bank_version_id) REFERENCES quizcraft_bank_versions(bank_id, id)
);
CREATE INDEX IF NOT EXISTS quizcraft_workshop_audit_bank_idx ON quizcraft_workshop_audit_events(bank_id, created_at, id);

CREATE TABLE IF NOT EXISTS quizcraft_feedbacks (
    id uuid PRIMARY KEY,
    bank_id uuid NOT NULL,
    question_id uuid NOT NULL,
    question_version_id uuid NOT NULL,
    actor_user_id uuid,
    actor_key text NOT NULL CHECK (char_length(actor_key) BETWEEN 1 AND 200),
    category text NOT NULL CHECK (category IN ('wrong_answer','ambiguous','typo','outdated','other')),
    detail text NOT NULL CHECK (char_length(detail) BETWEEN 1 AND 4000),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(bank_id, question_id, question_version_id) REFERENCES quizcraft_question_versions(bank_id, question_id, id)
);
CREATE INDEX IF NOT EXISTS quizcraft_feedbacks_question_idx ON quizcraft_feedbacks(bank_id, question_id, created_at DESC);

CREATE TABLE IF NOT EXISTS quizcraft_feedback_inbox_outbox (
    id uuid PRIMARY KEY,
    feedback_id uuid NOT NULL UNIQUE REFERENCES quizcraft_feedbacks(id),
    source_product_code text NOT NULL DEFAULT 'quizcraft' CHECK (source_product_code='quizcraft'),
    source_resource_type text NOT NULL DEFAULT 'feedback' CHECK (source_resource_type='feedback'),
    source_resource_id text NOT NULL,
    source_resource_url text NOT NULL,
    category text NOT NULL CHECK (category IN ('wrong_answer','ambiguous','typo','outdated','other')),
    priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS quizcraft_feedback_inbox_deliveries (
    outbox_id uuid PRIMARY KEY REFERENCES quizcraft_feedback_inbox_outbox(id) ON DELETE RESTRICT,
    platform_item_id uuid,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    delivered_at timestamptz,
    last_error text NOT NULL DEFAULT '' CHECK (char_length(last_error) <= 500),
    next_attempt_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS quizcraft_service_nonces (
    client_id text NOT NULL,
    key_id text NOT NULL,
    nonce text NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(client_id, key_id, nonce)
);
CREATE INDEX IF NOT EXISTS quizcraft_service_nonces_received_idx ON quizcraft_service_nonces(received_at);

DO $$ BEGIN
    CREATE TRIGGER quizcraft_workshop_audit_events_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_workshop_audit_events
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
DO $$ BEGIN
    CREATE TRIGGER quizcraft_feedbacks_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_feedbacks
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
DO $$ BEGIN
    CREATE TRIGGER quizcraft_feedback_inbox_outbox_immutable
        BEFORE UPDATE OR DELETE OR TRUNCATE ON quizcraft_feedback_inbox_outbox
        FOR EACH STATEMENT EXECUTE FUNCTION quizcraft_reject_immutable_mutation();
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
