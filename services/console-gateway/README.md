# Console Gateway

Console Gateway is the independently deployable authentication and controlled-access edge for HENUKit Console. It has no PostgreSQL connection, business migrations, or product database credentials.

The Gateway stores one-time OAuth state, PKCE verifier, and the validated same-origin return path in Redis for five minutes. A separate short-lived host-only HttpOnly/Secure/SameSite=Lax cookie binds that state to the browser that initiated login, preventing callback forwarding from swapping a victim into another account. After the exact callback, the Gateway exchanges the single-use code with Platform Core over authenticated HMAC, then creates an encrypted Console Session cookie with the same host-only security attributes. `GET /api/v1/session` revalidates `console.overview.read` with Platform Core on every request, so account, grant, and Session revocation are not hidden by a Gateway authorization cache.

OAuth client credentials, the exact callback URL, HMAC keys, and the Console Session encryption key are provisioned through the deployment environment or secret manager. They are not created by Gateway migrations; the Gateway intentionally owns no database migrations.

`GET /api/v1/overview` reads the configured Portal, Platform Operations, Notice, Library, QuizCraft, and Food summary endpoints concurrently. Every downstream read carries a rotatable Basic + HMAC identity dedicated to that one module, a fresh nonce, timestamp, and exact child request identifier. Summary secrets must be distinct across modules and separate from the Platform Core OAuth client secret, which limits a compromised module to its own audience. Each module has a two-second budget, the whole response has a three-second deadline, and only these idempotent GET reads receive at most one jittered retry. Successful bounded summaries may be cached in Redis for at most five minutes; a fallback is always labeled `stale` with `as_of`, `last_success_at`, and the current trace request identifier.

HC-11 connects the `portal` endpoint to `services/portal-summary`, whose OpenAPI schema inherits the Gateway module-summary schema. The Portal owner supplies deployment metadata and bounded probes; the Gateway does not synthesize or store Portal business state.

## Verification

- `powershell -ExecutionPolicy Bypass -File services/console-gateway/scripts/test-integration.ps1`
- `docker build -t henukit-console-gateway:local services/console-gateway`
