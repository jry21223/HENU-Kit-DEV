-- name: GetVerificationRequestByKey :one
SELECT id, request_fingerprint, expires_at, created_at
FROM verification_codes
WHERE request_key = $1;

-- name: CreateVerificationCode :one
INSERT INTO verification_codes (
    email_lookup_hash, purpose, request_key, request_fingerprint,
    code_nonce, code_hash, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, expires_at, created_at;

-- name: CreateVerificationMailOutbox :one
INSERT INTO mail_outbox (
    verification_code_id, dedupe_key, request_id, kind, priority,
    recipient_ciphertext, payload_ciphertext
) VALUES ($1, $2, $3, 'verification_code', 'critical', $4, $5)
RETURNING id;

-- name: GetVerificationCodeForUpdate :one
SELECT id, code_nonce, code_hash, expires_at, used_at, revoked_at, failed_attempts,
       consumed_request_key, consumed_request_fingerprint
FROM verification_codes
WHERE email_lookup_hash = $1 AND purpose = $2
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE;

-- name: RegisterFailedVerificationAttempt :one
UPDATE verification_codes
SET failed_attempts = failed_attempts + 1,
    revoked_at = CASE WHEN failed_attempts + 1 >= 5 THEN now() ELSE revoked_at END
WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL AND failed_attempts < 5
RETURNING failed_attempts, revoked_at;

-- name: ConsumeVerificationCode :execrows
UPDATE verification_codes
SET used_at = now(), consumed_request_key = $2, consumed_request_fingerprint = $3
WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now();

-- name: FailExhaustedOutboxLeases :exec
UPDATE mail_outbox
SET status = 'failed', locked_at = NULL, locked_by = NULL,
    failed_at = now(), last_error_code = 'WORKER_LEASE_EXHAUSTED', updated_at = now()
WHERE status = 'processing' AND locked_at < $1 AND attempt_count >= max_attempts;

-- name: ClaimMailOutbox :one
WITH candidate AS (
    SELECT candidate_job.id
    FROM mail_outbox AS candidate_job
    WHERE (
        (candidate_job.status IN ('pending', 'retry_due') AND candidate_job.available_at <= now())
        OR (candidate_job.status = 'processing' AND candidate_job.locked_at < sqlc.arg(reclaim_before))
    )
      AND candidate_job.attempt_count < candidate_job.max_attempts
    ORDER BY CASE candidate_job.priority WHEN 'critical' THEN 1 ELSE 2 END, candidate_job.available_at, candidate_job.created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE mail_outbox AS job
SET status = 'processing', attempt_count = attempt_count + 1,
    locked_at = now(), locked_by = sqlc.arg(worker_id), updated_at = now()
FROM candidate
WHERE job.id = candidate.id
RETURNING job.id, job.dedupe_key, job.request_id, job.recipient_ciphertext, job.payload_ciphertext,
          job.attempt_count, job.max_attempts;

-- name: AcceptMailOutbox :execrows
UPDATE mail_outbox
SET status = 'accepted', locked_at = NULL, locked_by = NULL,
    provider_message_id = $3, accepted_at = now(), last_error_code = NULL, updated_at = now()
WHERE id = $1 AND status = 'processing' AND locked_by = $2;

-- name: RetryMailOutbox :execrows
UPDATE mail_outbox
SET status = 'retry_due', locked_at = NULL, locked_by = NULL,
    available_at = $3, last_error_code = $4, updated_at = now()
WHERE id = $1 AND status = 'processing' AND locked_by = $2 AND attempt_count < max_attempts;

-- name: FailMailOutbox :execrows
UPDATE mail_outbox
SET status = 'failed', locked_at = NULL, locked_by = NULL,
    failed_at = now(), last_error_code = $3, updated_at = now()
WHERE id = $1 AND status = 'processing' AND locked_by = $2;

-- name: MarkMailOutboxDelivered :execrows
UPDATE mail_outbox
SET status = 'delivered', delivered_at = now(), updated_at = now()
WHERE provider_message_id = $1 AND status = 'accepted';

-- name: GetMailOutboxByVerificationCode :one
SELECT id, status, attempt_count, max_attempts, provider_message_id,
       accepted_at, delivered_at, failed_at, last_error_code
FROM mail_outbox
WHERE verification_code_id = $1;
