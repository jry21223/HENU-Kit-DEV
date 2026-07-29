# Portal Gateway Context

## Owns

- Portal-specific OAuth callback, PKCE flow, and one-time state (Redis + browser-binding nonce).
- Portal Session cookie (`__Host-henukit_portal_session`) with AES-256-GCM encryption.
- Server-to-server signed forwarding to product services (Library, Food, Practice, Notice, Account Portfolio).
- Portal-specific permission verification via Platform Core (`portal.library.read`, `portal.food.read`, `portal.practice.read`, `portal.notice.read`).

## Does not own

- Platform users, Core Sessions, authorization codes, or permission grants (owned by Platform Core).
- Product data, business logic, or databases (owned by each product service).
- Console Gateway sessions, permissions, or admin operations.
- Portal frontend implementation (owned by apps/portal).

## Current boundary

Portal Gateway is read-only by default. It authenticates users via Platform Core OAuth, establishes a Portal Session, and proxies GET requests to product services with verified permissions. Account Portfolio point-ledger reads accept only a bounded opaque cursor and page size; the Gateway includes the canonical query in the signed owner RequestURI while validating the returned data against the static owner route. ADR-0017 creates one deliberately narrow exception for authenticated Account Portfolio support-ticket create/follow-up and notification mark-read commands. ADR-0018 creates a separate, default-off exception for exactly two QuizCraft Practice commands: create a server-selected session and submit one answer. For that boundary Gateway uses an independent command credential, signs a validated Portal Session UUID only for signed-in requests, and otherwise relays only the Core-issued `quizcraft_anonymous` cookie; it never forwards a generic browser cookie jar or a generic product write. Both Practice command gates remain off until #166. Gateway does not own business data and never exposes service credentials to the browser.

Before QuizCraft #166, `/api/v1/practice/catalog` is a dark V2 handoff seam, not a normal public product proxy. With `PORTAL_ENABLE_QUIZCRAFT_CATALOG=0` (the default), the legacy Practice wildcard returns `404` for that exact path without calling Portal API or QuizCraft. With the flag explicitly `1`, Gateway signs the existing QuizCraft Core catalog contract, maps only bank ID, immutable bank-version ID, name, question count, and published availability, and never substitutes legacy or mock data. Every catalog read uses the explicit `anonymous` actor: the directory is public and has no user-specific result, while the existing read signature does not bind an actor header, so no browser Session identity enters the Core request.

## Key terms

- **Portal Session**: An encrypted cookie containing UserID, the login-time Display Name snapshot, ExchangeToken, and ExpiresAt. The snapshot is presentation-only and refreshes on the next OAuth login; Platform Core remains the Display Name source of truth. Distinct from Console Session and Core Session.
- **Exchange token**: A Platform Core session exchange token, stored server-side in the encrypted cookie, forwarded to Platform Core for permission checks.
- **Portal permission**: A permission code prefixed with `portal.*` (e.g., `portal.library.read`), distinct from Console's `console.*` permissions.
- **Portal Practice command**: The ADR-0018-specific signed bridge for only create-session and submit-answer. It is not a product write proxy, and before #166 it must fail closed without contacting QuizCraft Core.

## Relationships

- **Portal Gateway → Platform Core**: OAuth code exchange, permission verification. Uses HMAC-SHA256 service-to-service auth.
- **Portal Gateway → Product services**: Signed read-only proxying. Each request carries `X-Actor-User-Id` and `X-Request-Id`.
- **Portal Gateway → Account Portfolio**: Signed authenticated account reads plus the ADR-0017 self-service ticket/notification command exception; the Gateway owns neither account balances nor a fallback response.
- **Portal Gateway → QuizCraft Practice Core**: ADR-0018's two default-off commands only, using a credential distinct from catalog reads; QuizCraft remains the owner of session selection, scoring, attempts, and the anonymous identity cookie.
- **Portal frontend → Portal Gateway**: Same-origin API calls with session cookie (`credentials: "same-origin"`).
