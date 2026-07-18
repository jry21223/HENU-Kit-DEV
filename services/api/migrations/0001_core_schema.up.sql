CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    email varchar(255) NOT NULL UNIQUE, name varchar(80) NOT NULL, role varchar(32) NOT NULL DEFAULT 'user', status varchar(32) NOT NULL DEFAULT 'active',
    school_id uuid, major_id uuid, grade varchar(32) NOT NULL DEFAULT '', email_verified boolean NOT NULL DEFAULT false,
    frozen_until timestamptz, points_balance bigint NOT NULL DEFAULT 0, token_version integer NOT NULL DEFAULT 1, version integer NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

CREATE TABLE IF NOT EXISTS email_verification_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    email varchar(255) NOT NULL, code_hash varchar(128) NOT NULL, purpose varchar(32) NOT NULL DEFAULT 'login', expires_at timestamptz NOT NULL, used_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_email_verification_lookup ON email_verification_codes(email, purpose, used_at, created_at DESC);

CREATE TABLE IF NOT EXISTS quiz_questions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    course_id uuid NOT NULL, knowledge_point_id uuid, type varchar(40) NOT NULL, stem text NOT NULL, answer text NOT NULL,
    explanation text NOT NULL DEFAULT '', difficulty integer NOT NULL DEFAULT 1, status varchar(32) NOT NULL DEFAULT 'draft', author_id uuid
);
CREATE INDEX IF NOT EXISTS idx_quiz_questions_course ON quiz_questions(course_id);

CREATE TABLE IF NOT EXISTS quiz_options (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    question_id uuid NOT NULL, label varchar(16) NOT NULL, content text NOT NULL, sort_order integer NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_quiz_options_question ON quiz_options(question_id, sort_order);

CREATE TABLE IF NOT EXISTS notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    user_id uuid NOT NULL, type varchar(60) NOT NULL, title varchar(200) NOT NULL, body text NOT NULL DEFAULT '', data jsonb, read_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    status varchar(32) NOT NULL DEFAULT 'pending', reviewer_id uuid, reviewed_at timestamptz, review_reason varchar(1000) NOT NULL DEFAULT '',
    reporter_id uuid NOT NULL, target_type varchar(60) NOT NULL, target_id varchar(120) NOT NULL, reason varchar(500) NOT NULL DEFAULT '', description text NOT NULL DEFAULT '',
    target_snapshot jsonb, json_verified boolean NOT NULL DEFAULT false, postgres_verified boolean NOT NULL DEFAULT false,
    api_verified boolean NOT NULL DEFAULT false, version integer NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status);
CREATE INDEX IF NOT EXISTS idx_reports_target ON reports(target_type, target_id);

CREATE TABLE IF NOT EXISTS operation_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    operator_id uuid NOT NULL, action varchar(120) NOT NULL, target_type varchar(60) NOT NULL DEFAULT '', target_id varchar(120) NOT NULL DEFAULT '',
    ip varchar(80) NOT NULL DEFAULT '', user_agent varchar(500) NOT NULL DEFAULT '', metadata jsonb
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_actor_time ON operation_logs(operator_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_logs_action ON operation_logs(action);
