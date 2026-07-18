# HENU Kit Platform Core

Independent Go service for platform-owned identity data. The HC-05 slice implements:

- host-only Core Session validation;
- exact-callback OAuth authorization with S256 PKCE;
- 60–120 second, hash-only, single-use Authorization Codes;
- server-to-server code exchange with a short-lived, hash-only exchange Session;
- PostgreSQL as the durable source of truth and Redis only as an exchange lock;
- liveness and dependency readiness endpoints.

It does not implement Console Gateway or a production account-login/bootstrap endpoint.

Production configuration is environment-only. Copy key names from `.env.example`; use distinct Platform Core PostgreSQL credentials and an authenticated `rediss://` URL. The service never logs either connection URL.

## Verification

From the repository root on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File services/platform-core/scripts/test-integration.ps1
```

The script runs migration down/up/down/up against a dedicated PostgreSQL 17 database, then executes Go tests against real PostgreSQL and Redis containers. Generated sqlc files live in `internal/store`; regenerate them from `sqlc.yaml` when SQL changes.
