-- name: GetVerificationRequestByKey :one
SELECT id, request_fingerprint, expires_at, created_at
FROM verification_codes
WHERE request_key = $1;

-- name: GetConsumedVerificationReplay :one
SELECT verification_codes.id, verification_codes.consumed_request_fingerprint,
       sessions.expires_at AS login_session_expires_at,
       users.id AS login_user_id, users.email_verified AS login_user_email_verified,
       users.status AS login_user_status, users.created_at AS login_user_created_at
FROM verification_codes
LEFT JOIN sessions ON sessions.id = verification_codes.login_session_id
LEFT JOIN users ON users.id = sessions.user_id
WHERE consumed_request_key = $1 AND used_at IS NOT NULL;

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

-- name: GetEmailIdentityForUpdate :one
SELECT identity.user_id, users.email_verified, users.status, users.created_at
FROM email_identities AS identity
JOIN users ON users.id = identity.user_id
WHERE identity.email_lookup_hash = $1
FOR UPDATE OF identity, users;

-- name: CreateEmailLoginUser :one
INSERT INTO users (email_verified, status)
VALUES (true, 'active')
RETURNING id, email_verified, status, created_at;

-- name: CreateEmailIdentity :exec
INSERT INTO email_identities (user_id, email_lookup_hash, email_ciphertext, verified_at)
VALUES ($1, $2, $3, now());

-- name: CreateCoreSession :one
INSERT INTO sessions (user_id, kind, token_hash, expires_at)
VALUES ($1, 'core', $2, $3)
RETURNING id, expires_at;

-- name: AttachLoginSessionToVerification :execrows
UPDATE verification_codes
SET login_session_id = $2
WHERE id = $1 AND used_at IS NOT NULL AND purpose = 'login' AND login_session_id IS NULL;

-- name: ScrubExpiredVerificationSecrets :execrows
UPDATE verification_codes
SET request_key = NULL,
    request_fingerprint = NULL,
    code_nonce = NULL,
    code_hash = NULL,
    consumed_request_key = NULL,
    consumed_request_fingerprint = NULL,
    sensitive_cleared_at = now()
WHERE created_at <= $1
  AND sensitive_cleared_at IS NULL;

-- name: ScrubExpiredVerificationOutboxPayloads :execrows
UPDATE mail_outbox AS job
SET payload_ciphertext = NULL,
    payload_cleared_at = now(),
    updated_at = now()
FROM verification_codes AS verification
WHERE job.verification_code_id = verification.id
  AND verification.created_at <= $1
  AND job.payload_cleared_at IS NULL;

-- name: DeleteExpiredOAuthExchangeIdempotency :execrows
DELETE FROM oauth_exchange_idempotency
WHERE expires_at <= $1;

-- name: FailExhaustedOutboxLeases :exec
WITH transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'failed', locked_at = NULL, locked_by = NULL,
        failed_at = now(), last_error_code = 'WORKER_LEASE_EXHAUSTED', updated_at = now()
    WHERE job.status = 'processing' AND job.locked_at < $1 AND job.attempt_count >= job.max_attempts
    RETURNING job.id, job.request_id, job.attempt_count
), dead_lettered AS (
    INSERT INTO mail_dead_letters (outbox_id, attempt_count, error_code)
    SELECT id, attempt_count, 'WORKER_LEASE_EXHAUSTED' FROM transitioned
    RETURNING outbox_id
)
INSERT INTO mail_outbox_audit_events (
    outbox_id, request_id, actor_kind, actor_id, action, attempt_count, reason_code
)
SELECT transitioned.id, transitioned.request_id, 'system', 'lease-recovery', 'failed',
       transitioned.attempt_count, 'WORKER_LEASE_EXHAUSTED'
FROM transitioned
JOIN dead_lettered ON dead_lettered.outbox_id = transitioned.id;

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
), claimed AS (
    UPDATE mail_outbox AS job
    SET status = 'processing', attempt_count = attempt_count + 1,
        locked_at = now(), locked_by = sqlc.arg(worker_id), updated_at = now()
    FROM candidate
    WHERE job.id = candidate.id
    RETURNING job.id, job.dedupe_key, job.request_id, job.recipient_ciphertext, job.payload_ciphertext,
              job.attempt_count, job.max_attempts
), audited AS (
    INSERT INTO mail_outbox_audit_events (
        outbox_id, request_id, actor_kind, actor_id, action, attempt_count
    )
    SELECT id, request_id, 'worker', sqlc.arg(worker_id), 'claimed', attempt_count
    FROM claimed
    RETURNING outbox_id
)
SELECT claimed.id, claimed.dedupe_key, claimed.request_id,
       claimed.recipient_ciphertext, claimed.payload_ciphertext,
       claimed.attempt_count, claimed.max_attempts
FROM claimed
JOIN audited ON audited.outbox_id = claimed.id;

-- name: AcceptMailOutbox :execrows
WITH transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'accepted', locked_at = NULL, locked_by = NULL,
        provider_message_id = sqlc.arg(provider_message_id), accepted_at = now(),
        last_error_code = NULL, updated_at = now()
    WHERE job.id = sqlc.arg(outbox_id) AND job.status = 'processing' AND job.locked_by = sqlc.arg(worker_id)
    RETURNING job.id, job.request_id, job.attempt_count
)
INSERT INTO mail_outbox_audit_events (
    outbox_id, request_id, actor_kind, actor_id, action, attempt_count
)
SELECT id, request_id, 'worker', sqlc.arg(worker_id), 'accepted', attempt_count
FROM transitioned;

-- name: RetryMailOutbox :execrows
WITH transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'retry_due', locked_at = NULL, locked_by = NULL,
        available_at = sqlc.arg(available_at), last_error_code = sqlc.arg(last_error_code), updated_at = now()
    WHERE job.id = sqlc.arg(outbox_id) AND job.status = 'processing' AND job.locked_by = sqlc.arg(worker_id)
      AND job.attempt_count < job.max_attempts
    RETURNING job.id, job.request_id, job.attempt_count, job.last_error_code
)
INSERT INTO mail_outbox_audit_events (
    outbox_id, request_id, actor_kind, actor_id, action, attempt_count, reason_code
)
SELECT id, request_id, 'worker', sqlc.arg(worker_id), 'retry_scheduled', attempt_count, last_error_code
FROM transitioned;

-- name: FailMailOutbox :execrows
WITH transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'failed', locked_at = NULL, locked_by = NULL,
        failed_at = now(), last_error_code = sqlc.arg(last_error_code), updated_at = now()
    WHERE job.id = sqlc.arg(outbox_id) AND job.status = 'processing' AND job.locked_by = sqlc.arg(worker_id)
    RETURNING job.id, job.request_id, job.attempt_count, job.last_error_code
), dead_lettered AS (
    INSERT INTO mail_dead_letters (outbox_id, attempt_count, error_code)
    SELECT id, attempt_count, last_error_code FROM transitioned
    RETURNING outbox_id
)
INSERT INTO mail_outbox_audit_events (
    outbox_id, request_id, actor_kind, actor_id, action, attempt_count, reason_code
)
SELECT transitioned.id, transitioned.request_id, 'worker', sqlc.arg(worker_id), 'failed',
       transitioned.attempt_count, transitioned.last_error_code
FROM transitioned
JOIN dead_lettered ON dead_lettered.outbox_id = transitioned.id;

-- name: MarkMailOutboxDelivered :execrows
WITH transitioned AS (
    UPDATE mail_outbox
    SET status = 'delivered', delivered_at = now(), updated_at = now()
    WHERE provider_message_id = sqlc.arg(provider_message_id) AND status = 'accepted'
    RETURNING id, attempt_count
)
INSERT INTO mail_outbox_audit_events (
    outbox_id, request_id, actor_kind, actor_id, action, attempt_count
)
SELECT id, sqlc.arg(request_id), 'provider', sqlc.arg(actor_id), 'delivered', attempt_count
FROM transitioned;

-- name: RecordMailDeliveryReceipt :execrows
WITH receipt AS (
    INSERT INTO mail_delivery_receipts (message_id, request_id, actor_id)
    VALUES (sqlc.arg(message_id), sqlc.arg(request_id), sqlc.arg(actor_id))
    ON CONFLICT (message_id) DO UPDATE SET message_id = EXCLUDED.message_id
    RETURNING message_id, request_id, actor_id
), transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'delivered', delivered_at = now(), updated_at = now()
    FROM receipt
    WHERE job.provider_message_id = receipt.message_id AND job.status = 'accepted'
    RETURNING job.id, job.attempt_count, receipt.message_id, receipt.request_id, receipt.actor_id
), audited AS (
    INSERT INTO mail_outbox_audit_events (
        outbox_id, request_id, actor_kind, actor_id, action, attempt_count
    )
    SELECT id, request_id, 'provider', actor_id, 'delivered', attempt_count
    FROM transitioned
    RETURNING outbox_id
)
UPDATE mail_delivery_receipts AS receipt
SET applied_outbox_id = transitioned.id, applied_at = now()
FROM transitioned
JOIN audited ON audited.outbox_id = transitioned.id
WHERE receipt.message_id = transitioned.message_id AND receipt.applied_at IS NULL;

-- name: ApplyPendingMailDeliveryReceipt :execrows
WITH transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'delivered', delivered_at = now(), updated_at = now()
    FROM mail_delivery_receipts AS receipt
    WHERE job.provider_message_id = sqlc.arg(message_id)
      AND receipt.message_id = job.provider_message_id
      AND receipt.applied_at IS NULL AND job.status = 'accepted'
    RETURNING job.id, job.attempt_count, receipt.message_id, receipt.request_id, receipt.actor_id
), audited AS (
    INSERT INTO mail_outbox_audit_events (
        outbox_id, request_id, actor_kind, actor_id, action, attempt_count
    )
    SELECT id, request_id, 'provider', actor_id, 'delivered', attempt_count
    FROM transitioned
    RETURNING outbox_id
)
UPDATE mail_delivery_receipts AS receipt
SET applied_outbox_id = transitioned.id, applied_at = now()
FROM transitioned
JOIN audited ON audited.outbox_id = transitioned.id
WHERE receipt.message_id = transitioned.message_id;

-- name: ApplyAllPendingMailDeliveryReceipts :exec
WITH candidates AS (
    SELECT receipt.message_id, receipt.request_id, receipt.actor_id
    FROM mail_delivery_receipts AS receipt
    JOIN mail_outbox AS job ON job.provider_message_id = receipt.message_id
    WHERE receipt.applied_at IS NULL AND job.status = 'accepted'
    ORDER BY receipt.received_at
    FOR UPDATE OF receipt SKIP LOCKED
    LIMIT 50
), transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'delivered', delivered_at = now(), updated_at = now()
    FROM candidates
    WHERE candidates.message_id = job.provider_message_id AND job.status = 'accepted'
    RETURNING job.id, job.attempt_count, candidates.message_id, candidates.request_id, candidates.actor_id
), audited AS (
    INSERT INTO mail_outbox_audit_events (
        outbox_id, request_id, actor_kind, actor_id, action, attempt_count
    )
    SELECT id, request_id, 'provider', actor_id, 'delivered', attempt_count
    FROM transitioned
    RETURNING outbox_id
)
UPDATE mail_delivery_receipts AS receipt
SET applied_outbox_id = transitioned.id, applied_at = now()
FROM transitioned
JOIN audited ON audited.outbox_id = transitioned.id
WHERE receipt.message_id = transitioned.message_id;

-- name: RequeueMailOutbox :one
WITH target AS (
    SELECT dead_letter.id AS dead_letter_id, job.id AS outbox_id
    FROM mail_outbox AS job
    JOIN mail_dead_letters AS dead_letter ON dead_letter.outbox_id = job.id
    WHERE job.id = sqlc.arg(outbox_id) AND job.status = 'failed'
      AND dead_letter.requeued_at IS NULL
    ORDER BY dead_letter.dead_lettered_at DESC
    FOR UPDATE OF job, dead_letter
    LIMIT 1
), transitioned AS (
    UPDATE mail_outbox AS job
    SET status = 'pending', attempt_count = 0, available_at = now(),
        locked_at = NULL, locked_by = NULL, provider_message_id = NULL,
        accepted_at = NULL, delivered_at = NULL, failed_at = NULL,
        last_error_code = NULL, updated_at = now()
    FROM target
    WHERE job.id = target.outbox_id
    RETURNING job.id, job.request_id, job.attempt_count
), closed_dead_letter AS (
    UPDATE mail_dead_letters AS dead_letter
    SET requeued_at = now(), requeued_by = sqlc.arg(actor_id), requeue_reason = sqlc.arg(reason)
    FROM target
    WHERE dead_letter.id = target.dead_letter_id
    RETURNING dead_letter.outbox_id
), audited AS (
    INSERT INTO mail_outbox_audit_events (
        outbox_id, request_id, actor_kind, actor_id, action, attempt_count, reason_code
    )
    SELECT transitioned.id, sqlc.arg(request_id), 'operator', sqlc.arg(actor_id),
           'requeued', transitioned.attempt_count, 'MANUAL_REQUEUE'
    FROM transitioned
    JOIN closed_dead_letter ON closed_dead_letter.outbox_id = transitioned.id
    RETURNING outbox_id
)
SELECT transitioned.id
FROM transitioned
JOIN audited ON audited.outbox_id = transitioned.id;

-- name: GetMailOutboxByVerificationCode :one
SELECT id, status, attempt_count, max_attempts, provider_message_id,
       accepted_at, delivered_at, failed_at, last_error_code
FROM mail_outbox
WHERE verification_code_id = $1;

-- name: ListMailOutboxAuditEvents :many
SELECT request_id, actor_kind, actor_id, action, attempt_count, reason_code, created_at
FROM mail_outbox_audit_events
WHERE outbox_id = $1
ORDER BY created_at, id;
