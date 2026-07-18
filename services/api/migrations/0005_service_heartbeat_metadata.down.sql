ALTER TABLE service_heartbeats
    DROP COLUMN IF EXISTS worker_anomalies,
    DROP COLUMN IF EXISTS outbox_pending,
    DROP COLUMN IF EXISTS deployment_time,
    DROP COLUMN IF EXISTS commit_sha,
    DROP COLUMN IF EXISTS service_version;

