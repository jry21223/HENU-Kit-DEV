# HENU Kit Platform Core

Independent Go service for platform-owned identity data. The HC-05 slice implements:

- host-only Core Session validation;
- exact-callback OAuth authorization with S256 PKCE;
- 60–120 second, hash-only, single-use Authorization Codes;
- server-to-server code exchange protected by Basic client authentication, S256 PKCE, HMAC-SHA256, a five-minute timestamp window, and Redis nonce replay prevention;
- encrypted idempotent exchange responses retained for at least 24 hours without persisting plaintext Session tokens;
- PostgreSQL as the durable source of truth and Redis only for short-lived coordination;
- liveness and dependency readiness endpoints.
- propagated request IDs and structured, redacted request audit logs.

It does not implement Console Gateway or a production account-login/bootstrap endpoint.

Production configuration is environment-only. Copy key names from `.env.example`; use distinct Platform Core PostgreSQL credentials, an authenticated `rediss://` URL, and a random 32-byte idempotency encryption key. The service never logs connection URLs, credentials, request bodies, authorization codes, or Session tokens.

`POST /api/v1/oauth/token` requires `Idempotency-Key`, `X-Service-Id`, `X-Key-Id`, `X-Timestamp`, `X-Nonce`, and `X-Signature`. The signature is base64url HMAC-SHA256 over `METHOD`, the actual `PATH_AND_QUERY`, timestamp, nonce, and lowercase hexadecimal `SHA256(BODY)`, separated by newlines. Each OAuth client can have concurrent `active` and `retiring` keys during rotation.

## Verification

From the repository root on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File services/platform-core/scripts/test-integration.ps1
```

The script runs migration down/up/down/up against a dedicated PostgreSQL 17 database, then executes Go tests against real PostgreSQL and Redis containers. In CI, the integration suite also starts its own dependencies with Testcontainers when external test URLs are absent. Generated sqlc files live in `internal/store`; `internal/contract` is generated from the OpenAPI file. CI rejects stale generated output and breaking OpenAPI changes.
