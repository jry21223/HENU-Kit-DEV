# 000012 Auth Credential Retention

## Preflight

Run these read-only checks and retain the output with the release record:

```sql
SELECT current_setting('server_version_num')::integer >= 170000 AS supported_postgres;
SELECT to_regclass('public.sessions') IS NOT NULL
   AND to_regclass('public.verification_codes') IS NOT NULL
   AND to_regclass('public.mail_outbox') IS NOT NULL
   AND to_regclass('public.oauth_exchange_idempotency') IS NOT NULL AS prerequisite_schema;
SELECT count(*) AS verification_rows,
       count(*) FILTER (WHERE used_at IS NOT NULL) AS consumed_rows,
       pg_size_pretty(pg_total_relation_size('verification_codes')) AS verification_size,
       pg_size_pretty(pg_total_relation_size('mail_outbox')) AS outbox_size
FROM verification_codes;
SELECT count(*) AS cached_oauth_exchanges,
       coalesce(sum(octet_length(response_ciphertext)), 0) AS cached_oauth_bytes
FROM oauth_exchange_idempotency;
```

Both Boolean checks must be true. Record counts and relation sizes before choosing the maintenance window.

## Lock and data impact

The migration takes `ACCESS EXCLUSIVE` locks while altering `verification_codes`, `mail_outbox`, and `oauth_exchange_idempotency`. Constraint validation scans those relations. It replaces every recoverable Core-login and OAuth-exchange Session credential with an empty byte string; Session hashes and active Session rows are unchanged. Run in the declared maintenance window with login and mail workers stopped. The SQL aborts if a lock is unavailable for five seconds or total execution exceeds five minutes. For relations up to 100,000 rows each, the expected PostgreSQL 17 maintenance-window duration is under one minute; above that size, rehearse with the recorded relation sizes and retain the observed duration before production approval.

The discard triggers are an expand-phase compatibility guard: an older binary can still address the existing columns, but PostgreSQL replaces any credential value with empty bytes before persistence. Such a binary cannot replay a completed exchange and must restart OAuth. The new binary returns safe completion metadata for verification replay and `409 AUTH_CODE_ALREADY_USED` for OAuth exchange replay; neither path issues a second Session.

## Verification

```sql
SELECT count(*) = 0 AS no_recoverable_core_login_tokens
FROM verification_codes
WHERE coalesce(octet_length(login_session_token_ciphertext), 0) > 0;
SELECT count(*) = 0 AS no_recoverable_oauth_exchange_tokens
FROM oauth_exchange_idempotency
WHERE octet_length(response_ciphertext) > 0;
SELECT count(*) = 3 AND bool_and(convalidated) AS retention_constraints_valid
FROM pg_constraint
WHERE conname IN (
  'verification_codes_secret_retention_shape',
  'mail_outbox_payload_retention_shape',
  'oauth_exchange_response_discarded'
);
```

All results must be true. CI applies 000012 twice, runs Down then Up again against PostgreSQL 17, and executes identity, Session, concurrency, authorization, retention, and sensitive-log tests.

## Retention operation

Run the `auth-retention-cleanup` executable at least hourly. It atomically removes expired OAuth exchange idempotency rows and, at 24 hours, scrubs verification hashes, nonces, request/consume idempotency facts, and the encrypted Outbox payload that contained the verification code. It retains Session hashes and non-secret mail/login audit relationships. The command is idempotent and reports only aggregate row counts. Production scheduling is intentionally part of the later deployment ticket; this ticket supplies and CI-builds the executable boundary without deploying it.

## Rollback

Down starts with an irreversible-state guard and runs in one transaction, so refusal cannot partially delete schema or audit data. Before the first cleanup, stop the new binaries and run Down during maintenance, then restore the prior artifact. Previously discarded cached Session responses cannot be reconstructed, so affected callers restart login/OAuth. After cleanup has run, roll forward. For an exact pre-migration state, restore and checksum-verify the pre-migration backup before starting the prior artifact.
