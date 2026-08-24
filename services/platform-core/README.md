# HENU Kit Platform Core

Provision the QuizCraft OAuth redirect and rotatable HMAC client key after applying migrations with `scripts/provision-quizcraft-client.sh`. The script requires `DATABASE_URL`, `QUIZCRAFT_PUBLIC_URL`, `QUIZCRAFT_PLATFORM_KEY_ID`, and `QUIZCRAFT_PLATFORM_CLIENT_SECRET`; it stores only the SHA-256 secret hash and never places the plaintext secret in SQL or process arguments. Each rotation keeps only the immediately previous active key as `retiring` and revokes any older retiring key, bounding the overlap window to one rotation.

Independent Go service for platform-owned identity and operations data. The delivered HC-05 through HC-08 slices implement:

- host-only Core Session validation;
- Account Center registration at `/register`; it atomically verifies the HENU mailbox, creates the encrypted Email Identity and Argon2id password credential, consumes the code, and issues one non-rolling 30-day Core Session;
- password and email-code login at `/login`; neither login path creates an account, and successful password authentication upgrades stale Argon2id parameters;
- email-code password recovery at `/recover`, which atomically replaces the credential, revokes every old Core and exchange Session, consumes the code, and issues exactly one new Core Session;
- authenticated password changes at `/account/security`, which require the current password plus a fresh email code, retain only the current Core Session, and revoke every other Core and exchange Session;
- a bounded Account Center Bootstrap response that supplies Portal with CSRF and account-flow state without exposing OAuth or Session facts, plus explicit status responses that remove Portal's dependency on Platform Core HTML;
- exact-callback OAuth authorization with S256 PKCE;
- 60–120 second, hash-only, single-use Authorization Codes;
- eight-hour product-local exchange Sessions for Console and Workshop high-privilege work, with immediate server-side revocation; the `portal-gateway` OAuth client overrides this to 30 days (`PLATFORM_CORE_EXCHANGE_SESSION_TTL_OVERRIDES`) so the Portal Session stays valid for the full Core Session window;
- server-to-server code exchange protected by Basic client authentication, S256 PKCE, HMAC-SHA256, a five-minute timestamp window, and Redis nonce replay prevention;
- hash-only Session persistence; completed OAuth exchange replays return a safe conflict and require restarting OAuth rather than recovering a prior credential;
- PostgreSQL as the durable source of truth and Redis only for short-lived coordination;
- liveness and dependency readiness endpoints.
- propagated request IDs and structured, redacted request audit logs.
- default-deny authorization checks derived from server-side permission codes, multiple roles, and platform/product/resource Scope grants;
- immediate PostgreSQL-backed propagation for account suspension, role/grant revocation, exchange Session revocation, and parent Core Session revocation;
- transactional allowed/denied authorization audit events correlated by actor, Session, request, service, permission, target resource, grant, and revision;
- database constraints that prevent concurrent duplicate active role grants for the same Scope.
- student-email verification requests with hash-only codes, encrypted recipients and encrypted minimal mail payloads;
- transactional PostgreSQL verification facts plus critical-priority mail Outbox jobs, with Redis used only for fail-closed email/IP/device hourly, daily, and resend limits;
- single-use verification with attempt limits and idempotent success replay under concurrent requests;
- an independently deployable mail worker with lease recovery, bounded provider timeouts, exponential retry, immutable transition audits, dead letters, controlled operator requeue, provider acceptance and separate delivery confirmation states.
- a network-isolated SMTP Provider with Bearer authentication, implicit TLS on port 465 or mandatory STARTTLS on submission ports, multipart plain/HTML verification mail, and a root-owned idempotency ledger;
- Operations Inbox creation, product-scoped querying, assignment, priority, SLA and status updates using source-resource references only;
- durable write idempotency, optimistic versions, transactional append-only operations audits, and server-side permission plus Scope enforcement.

It does not own Console Gateway sessions or product-local sessions. Legacy QuizCraft IDs are not automatically mapped to Email Identities.

Production configuration is environment-only. Copy key names from `.env.example`; use distinct Platform Core PostgreSQL credentials, an authenticated `rediss://` URL, and separate random 32-byte idempotency and verification keys. The service never logs connection URLs, credentials, request bodies, email addresses, verification codes, authorization codes, or Session tokens.

Authentication cookies follow ADR-0015. Direct TLS, or an exact
`X-Forwarded-Proto: https` supplied by a peer in
`PLATFORM_CORE_TRUSTED_PROXY_CIDRS`, selects the Secure `__Host-` cookie
family. Direct local HTTP selects the separate non-`__Host-` name configured
by `PLATFORM_CORE_LOCAL_COOKIE_NAME`. Session issuance, rotation, revocation,
CSRF, and device cookies all use the same per-request decision; forwarding
headers from untrusted peers cannot enable production cookies.

Production login is code-locked to the single `henu.edu.cn` domain. `PLATFORM_CORE_STUDENT_EMAIL_DOMAINS` remains an explicit deployment assertion and the process refuses to start if it contains any other value. Run `go run ./cmd/auth-retention-cleanup` at least hourly with the Platform Core database URL: it atomically removes expired OAuth exchange idempotency responses and scrubs verification hashes, nonces, and request/consume idempotency facts after 24 hours while retaining non-secret mail and login audit relationships.

Passwords are counted as 10–128 Unicode code points and are never trimmed,
truncated, or normalized. Platform Core rejects the exact normalized email
local-part and a versioned weak-password set. It stores only salted,
versioned Argon2id PHC verifiers. The default parameters are 64 MiB, three
iterations, parallelism one, with at most two concurrent hashes. Calibrate
these values on each production host so one hash takes roughly 150–300 ms
under representative load, while preserving the accepted bounds enforced at
startup. Never reduce them merely to make tests faster.

Failed password authentication is counted across email, trusted client IP, and
signed device-cookie axes. Five failures in 15 minutes, ten in an hour, or
twenty in a day require a successful email-code login before password
authentication or password changes resume. Successful password or email-code
login clears these temporary counters. Redis failure rejects password login,
recovery, and credential changes rather than bypassing this boundary.

`POST /api/v1/oauth/token` requires `Idempotency-Key`, `X-Service-Id`, `X-Key-Id`, `X-Timestamp`, `X-Nonce`, and `X-Signature`. The signature is base64url HMAC-SHA256 over `METHOD`, the actual `PATH_AND_QUERY`, timestamp, nonce, and lowercase hexadecimal `SHA256(BODY)`, separated by newlines. Each OAuth client key progresses through `active`, `retiring`, and `revoked`; only the first two states authenticate during a rotation window.

`POST /api/v1/authorization/check` uses the same Basic + HMAC service authentication and a server-held exchange Session token in its JSON body. Callers provide only a permission code and structured Scope; client roles and `isAdmin` are never authorization evidence. The service reads PostgreSQL on every check, so the implemented revocation propagation is the next request and remains below the contract's 30-second maximum.

`POST /api/v1/auth/email-codes` and `/api/v1/auth/email-codes/verify` require `Idempotency-Key`. Platform Core issues a signed, `HttpOnly`, `Secure` device cookie instead of trusting a browser-supplied device ID; both request and verification attempts use email/IP/device hourly and daily limits. Rate-limited send requests return the same privacy-preserving `202` shape but create no verification or Outbox row, verification-attempt limits return `429`, and Redis failure returns `503` (fail closed). Client IP comes from the socket peer unless it is in `PLATFORM_CORE_TRUSTED_PROXY_CIDRS`; trusted proxy chains are stripped from right to left so an appended client-controlled `X-Forwarded-For` prefix is not trusted.

A verification request `202` means only that processing was accepted; it does not prove provider acceptance or delivery. `cmd/mail-worker` claims jobs with `FOR UPDATE SKIP LOCKED`, refuses expired payloads, sends the decrypted payload only to the configured HTTP Provider, and records every transition without email addresses or codes. The production Provider is `cmd/smtp-provider`: bind it to loopback for a host deployment, or expose it only on the private Compose network without publishing its port. It reads SMTP credentials only from its protected environment. Its structured delivery audit records request/result/error classification, duration, attempt/retry counts, and non-secret Provider/Key IDs; it never records recipients, codes, credentials, message bodies, idempotency keys, or raw SMTP errors. `POST /api/v1/mail/deliveries` requires HMAC, a five-minute timestamp window, Redis-backed single-use Nonce, and an active or retiring Key ID. Receipts are persisted before reconciliation, so an early provider callback is applied after the matching Outbox acceptance instead of being lost. A failed job can be requeued deliberately with `cmd/mail-worker -requeue-outbox ... -request-id ... -actor-id ... -reason ...`; the dead letter and database-protected append-only operator audit remain durable. Build the worker and provider with `Dockerfile.worker` and `Dockerfile.smtp-provider`.

After the first operator signs in normally, run `cmd/grant-initial-operator` as root with `-email`, `-request-id`, and `-reason`. It grants `platform.operations.read/write` at platform Scope and `quizcraft.workshop.read/write/publish` at QuizCraft product Scope. It never creates a global superadmin and never requires manual SQL.

Operations Inbox uses `GET/POST /api/v1/operations-inbox/items`, `POST /api/v1/operations-inbox/items/{item_id}/updates`, and `GET /api/v1/operations-inbox/operations/{operation}` to resolve an unknown write outcome with the original idempotency key. Calls require Basic plus HMAC service authentication and the server-held `X-Session-Exchange-Token`; writes and operation-status lookups also require `Idempotency-Key`. The data model deliberately has no title, body, content or feedback-text field: full source content stays with the product identified by the immutable product/resource reference. Append-only audits retain a safe coordination snapshot for every committed version so owner, priority, SLA and status changes can be reconstructed without copying product content.

`POST /api/v1/platform-operations/account-lookups` lets a platform-read-scoped service caller resolve an exact full email to an account id, display name, and status. The email appears only in the request body: never in URLs, logs, audit rows, or the response, and the Redis rate limit is keyed on the caller rather than the email. A miss returns the same 200 envelope shape as a hit and performs an equivalent index probe so response time cannot reveal whether an account exists. Email normalization and hashing reuse the login/verification path byte-for-byte, so the same mailbox always yields the same lookup hash. The operational snapshot, sessions, and audit events also carry the optional `display_name` (omitted for legacy rows) and audit rows survive the actor's hard deletion via `LEFT JOIN`.

## Verification

From the repository root on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File services/platform-core/scripts/test-integration.ps1
```

The script runs migration down/up/down/up against a dedicated PostgreSQL 17 database, then executes Go tests against real PostgreSQL and Redis containers. In CI, the integration suite also starts its own dependencies with Testcontainers when external test URLs are absent. Generated sqlc files live in `internal/store`; `internal/contract` is generated from the OpenAPI file. CI rejects stale generated output and breaking OpenAPI changes.
