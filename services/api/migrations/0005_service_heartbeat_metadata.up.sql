ALTER TABLE service_heartbeats
    ADD COLUMN IF NOT EXISTS service_version varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS commit_sha varchar(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deployment_time timestamptz,
    ADD COLUMN IF NOT EXISTS outbox_pending bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS worker_anomalies bigint NOT NULL DEFAULT 0;

