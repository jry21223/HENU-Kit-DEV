# HENU Kit Platform Core

Provision the QuizCraft OAuth redirect and rotatable HMAC client key after applying migrations with `scripts/provision-quizcraft-client.sh`. The script requires `DATABASE_URL`, `QUIZCRAFT_PUBLIC_URL`, `QUIZCRAFT_PLATFORM_KEY_ID`, and `QUIZCRAFT_PLATFORM_CLIENT_SECRET`; it stores only the SHA-256 secret hash and never places the plaintext secret in SQL or process arguments. Each rotation keeps only the immediately previous active key as `retiring` and revokes any older retiring key, bounding the overlap window to one rotation.

Independent Go service for platform-owned identity and operations data. The delivered HC-05 through HC-08 slices implement:

- host-only Core Session validation;
- an account-center HTML flow at `/login`; successful login verification atomically creates/restores an encrypted Email Identity and issues a non-rolling 15-day Core Session;
- exact-callback OAuth authorization with S256 PKCE;
- 60–120 second, hash-only, single-use Authorization Codes;
- eight-hour product-local exchange Sessions for Console and Workshop high-privilege work, with immediate server-side revocation;
- server-to-server code exchange protected by Basic client authentication, S256 PKCE, HMAC-SHA256, a five-minute timestamp window, and Redis nonce replay prevention;
- encrypted idempotent exchange responses retained for at least 24 hours without persisting plaintext Session tokens;
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
- a loopback-only SMTP Provider with Bearer authentication, STARTTLS, and a root-owned idempotency ledger;
- Operations Inbox creation, product-scoped querying, assignment, priority, SLA and status updates using source-resource references only;
- durable write idempotency, optimistic versions, transactional append-only operations audits, and server-side permission plus Scope enforcement.

It does not own Console Gateway sessions or product-local sessions. Legacy QuizCraft IDs are not automatically mapped to Email Identities.

Production configuration is environment-only. Copy key names from `.env.example`; use distinct Platform Core PostgreSQL credentials, an authenticated `rediss://` URL, and separate random 32-byte idempotency and verification keys. The service never logs connection URLs, credentials, request bodies, email addresses, verification codes, authorization codes, or Session tokens.

`POST /api/v1/oauth/token` requires `Idempotency-Key`, `X-Service-Id`, `X-Key-Id`, `X-Timestamp`, `X-Nonce`, and `X-Signature`. The signature is base64url HMAC-SHA256 over `METHOD`, the actual `PATH_AND_QUERY`, timestamp, nonce, and lowercase hexadecimal `SHA256(BODY)`, separated by newlines. Each OAuth client key progresses through `active`, `retiring`, and `revoked`; only the first two states authenticate during a rotation window.

`POST /api/v1/authorization/check` uses the same Basic + HMAC service authentication and a server-held exchange Session token in its JSON body. Callers provide only a permission code and structured Scope; client roles and `isAdmin` are never authorization evidence. The service reads PostgreSQL on every check, so the implemented revocation propagation is the next request and remains below the contract's 30-second maximum.

`POST /api/v1/auth/email-codes` and `/api/v1/auth/email-codes/verify` require `Idempotency-Key`. Platform Core issues a signed, `HttpOnly`, `Secure` device cookie instead of trusting a browser-supplied device ID; both request and verification attempts use email/IP/device hourly and daily limits. Rate-limited send requests return the same privacy-preserving `202` shape but create no verification or Outbox row, verification-attempt limits return `429`, and Redis failure returns `503` (fail closed). Client IP comes from the socket peer unless it is in `PLATFORM_CORE_TRUSTED_PROXY_CIDRS`; trusted proxy chains are stripped from right to left so an appended client-controlled `X-Forwarded-For` prefix is not trusted.

A verification request `202` means only that processing was accepted; it does not prove provider acceptance or delivery. `cmd/mail-worker` claims jobs with `FOR UPDATE SKIP LOCKED`, refuses expired payloads, sends the decrypted payload only to the configured HTTP Provider, and records every transition without email addresses or codes. The production Provider is `cmd/smtp-provider`, bound to `127.0.0.1`, and reads SMTP credentials only from its root-owned environment. `POST /api/v1/mail/deliveries` requires HMAC, a five-minute timestamp window, Redis-backed single-use Nonce, and an active or retiring Key ID. Receipts are persisted before reconciliation, so an early provider callback is applied after the matching Outbox acceptance instead of being lost. A failed job can be requeued deliberately with `cmd/mail-worker -requeue-outbox ... -request-id ... -actor-id ... -reason ...`; the dead letter and database-protected append-only operator audit remain durable. Build the worker and provider with `Dockerfile.worker` and `Dockerfile.smtp-provider`.

After the first operator signs in normally, run `cmd/grant-initial-operator` as root with `-email`, `-request-id`, and `-reason`. It grants `platform.operations.read/write` at platform Scope and `quizcraft.workshop.read/write/publish` at QuizCraft product Scope. It never creates a global superadmin and never requires manual SQL.

Operations Inbox uses `GET/POST /api/v1/operations-inbox/items`, `POST /api/v1/operations-inbox/items/{item_id}/updates`, and `GET /api/v1/operations-inbox/operations/{operation}` to resolve an unknown write outcome with the original idempotency key. Calls require Basic plus HMAC service authentication and the server-held `X-Session-Exchange-Token`; writes and operation-status lookups also require `Idempotency-Key`. The data model deliberately has no title, body, content or feedback-text field: full source content stays with the product identified by the immutable product/resource reference. Append-only audits retain a safe coordination snapshot for every committed version so owner, priority, SLA and status changes can be reconstructed without copying product content.

## Verification

From the repository root on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File services/platform-core/scripts/test-integration.ps1
```

The script runs migration down/up/down/up against a dedicated PostgreSQL 17 database, then executes Go tests against real PostgreSQL and Redis containers. In CI, the integration suite also starts its own dependencies with Testcontainers when external test URLs are absent. Generated sqlc files live in `internal/store`; `internal/contract` is generated from the OpenAPI file. CI rejects stale generated output and breaking OpenAPI changes.
