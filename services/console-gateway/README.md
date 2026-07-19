# Console Gateway

Console Gateway is the independently deployable authentication and controlled-access edge for HENUKit Console. It has no PostgreSQL connection, business migrations, or product database credentials.

The Gateway stores one-time OAuth state, PKCE verifier, and the validated same-origin return path in Redis for five minutes. After the exact callback, it exchanges the single-use code with Platform Core over authenticated HMAC, then creates a host-only HttpOnly/Secure/SameSite=Lax encrypted Console Session cookie. `GET /api/v1/session` revalidates `console.overview.read` with Platform Core on every request, so account, grant, and Session revocation are not hidden by a Gateway authorization cache.

OAuth client credentials, the exact callback URL, HMAC keys, and the Console Session encryption key are provisioned through the deployment environment or secret manager. They are not created by Gateway migrations; the Gateway intentionally owns no database migrations.

## Verification

- `powershell -ExecutionPolicy Bypass -File services/console-gateway/scripts/test-integration.ps1`
- `docker build -t henukit-console-gateway:local services/console-gateway`
