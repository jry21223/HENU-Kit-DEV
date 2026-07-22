# 000011 Email Identity Login

## Preflight

Run these read-only checks before the migration and retain their output with the release record:

```sql
SELECT current_setting('server_version_num')::integer >= 170000 AS supported_postgres;
SELECT to_regclass('public.users') IS NOT NULL
   AND to_regclass('public.sessions') IS NOT NULL
   AND to_regclass('public.verification_codes') IS NOT NULL AS prerequisite_schema;
SELECT count(*) AS verification_rows,
       count(*) FILTER (WHERE used_at IS NOT NULL) AS consumed_rows,
       pg_size_pretty(pg_total_relation_size('verification_codes')) AS verification_size
FROM verification_codes;
SELECT count(*) AS active_core_sessions
FROM sessions
WHERE kind = 'core' AND revoked_at IS NULL AND expires_at > now();
```

Both Boolean checks must be true. Record row counts and relation size before choosing the maintenance window.

## Lock and data impact

The migration takes `ACCESS EXCLUSIVE` locks while altering `verification_codes`. Dropping the obsolete encrypted Session credential is metadata-only; validating the replacement retention constraints scans `verification_codes`. `email_identities` starts empty and the remaining objects are catalog changes. Run during the declared maintenance window with login writes stopped. Use a transaction-level `lock_timeout` appropriate to the window and abort rather than wait behind unexpected traffic.

No user, identity, Session hash, mail state, or audit row is deleted by Up. Existing encrypted login Session credentials are deliberately destroyed; callers retain their already-issued cookie, while an idempotent verification replay returns completion metadata without reissuing a credential.

## Verification

```sql
SELECT to_regclass('public.email_identities') IS NOT NULL AS identity_table_ready;
SELECT count(*) = 0 AS no_recoverable_session_columns
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'verification_codes'
  AND column_name LIKE '%session%token%';
SELECT convalidated
FROM pg_constraint
WHERE conrelid = 'verification_codes'::regclass
  AND conname IN ('verification_codes_login_session_shape', 'verification_codes_secret_retention_shape');
```

The two Boolean results and every `convalidated` result must be true. CI applies Up twice, then runs the complete Down chain and Up chain again against PostgreSQL 17 before running the identity, Session, concurrency, authorization, retention, and sensitive-log tests.

## Retention operation

Schedule `auth-retention-cleanup` at least hourly. It scrubs verification hashes, nonces, request fingerprints, and request/consume idempotency keys once `created_at` is 24 hours old, and deletes OAuth exchange idempotency responses after their expiry. The operation is transactional and idempotent; it retains non-secret relationships needed by mail and login audits.

## Rollback

Before the first retention cleanup, stop the new binaries and run Down inside the maintenance window; then restore the prior application artifact. Down refuses to proceed after any verification row has been scrubbed because discarded credentials cannot be reconstructed. After cleanup has run, roll forward. If an exact pre-migration schema is mandatory, restore the pre-migration database backup and verify its checksum before bringing the prior artifact online.
