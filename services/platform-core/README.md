# HENU Kit Platform Core

Independent Go service for platform-owned identity data. The delivered HC-05 through HC-07 slices implement:

- host-only Core Session validation;
- exact-callback OAuth authorization with S256 PKCE;
- 60–120 second, hash-only, single-use Authorization Codes;
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
- transactional PostgreSQL verification facts plus critical-priority mail Outbox jobs, with Redis used only for a replaceable resend limit;
- single-use verification with attempt limits and idempotent success replay under concurrent requests;
- an independently deployable mail worker with lease recovery, bounded provider timeouts, exponential retry, permanent failure, provider acceptance and separate delivery confirmation states.

It does not implement Console Gateway or turn a successful verification into a production account-login/bootstrap Session yet.

Production configuration is environment-only. Copy key names from `.env.example`; use distinct Platform Core PostgreSQL credentials, an authenticated `rediss://` URL, and separate random 32-byte idempotency and verification keys. The service never logs connection URLs, credentials, request bodies, email addresses, verification codes, authorization codes, or Session tokens.

`POST /api/v1/oauth/token` requires `Idempotency-Key`, `X-Service-Id`, `X-Key-Id`, `X-Timestamp`, `X-Nonce`, and `X-Signature`. The signature is base64url HMAC-SHA256 over `METHOD`, the actual `PATH_AND_QUERY`, timestamp, nonce, and lowercase hexadecimal `SHA256(BODY)`, separated by newlines. Each OAuth client key progresses through `active`, `retiring`, and `revoked`; only the first two states authenticate during a rotation window.

`POST /api/v1/authorization/check` uses the same Basic + HMAC service authentication and a server-held exchange Session token in its JSON body. Callers provide only a permission code and structured Scope; client roles and `isAdmin` are never authorization evidence. The service reads PostgreSQL on every check, so the implemented revocation propagation is the next request and remains below the contract's 30-second maximum.

`POST /api/v1/auth/email-codes` and `/api/v1/auth/email-codes/verify` require `Idempotency-Key`. A `202` means the durable mail job was queued; it does not mean the provider accepted or delivered the email. `cmd/mail-worker` claims jobs with `FOR UPDATE SKIP LOCKED`, sends the decrypted payload only to the configured HTTPS provider, records `accepted` after the provider returns a message ID, and records `delivered` only after explicit delivery confirmation. Build its independent image with `Dockerfile.worker`.

## Verification

From the repository root on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File services/platform-core/scripts/test-integration.ps1
```

The script runs migration down/up/down/up against a dedicated PostgreSQL 17 database, then executes Go tests against real PostgreSQL and Redis containers. In CI, the integration suite also starts its own dependencies with Testcontainers when external test URLs are absent. Generated sqlc files live in `internal/store`; `internal/contract` is generated from the OpenAPI file. CI rejects stale generated output and breaking OpenAPI changes.
