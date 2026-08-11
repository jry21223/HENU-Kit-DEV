# Portal Gateway Context

## Owns

- Portal-specific OAuth callback, PKCE flow, and one-time state (Redis + browser-binding nonce).
- Portal Session cookie (`__Host-henukit_portal_session`) with AES-256-GCM encryption.
- Server-to-server signed forwarding to product services (Library, Food, Practice, Notice, Account Portfolio).
- Portal-specific permission verification via Platform Core (`portal.practice.read`; generic `portal.library.read`/`portal.food.read` owner clients remain removed by #261). ADR-0027 adds only one anonymous, exact Library download-grant client.

## Does not own

- Platform users, Core Sessions, authorization codes, or permission grants (owned by Platform Core).
- Product data, business logic, or databases (owned by each product service).
- Console Gateway sessions, permissions, or admin operations.
- Portal frontend implementation (owned by apps/portal).

## Current boundary

Portal Gateway is read-only by default. It authenticates users via Platform Core OAuth, establishes a Portal Session, and proxies GET requests to product services with verified permissions. Account Portfolio point-ledger reads accept only a bounded opaque cursor and page size; the Gateway includes the canonical query in the signed owner RequestURI while validating the returned data against the static owner route. ADR-0017 creates one deliberately narrow exception for authenticated Account Portfolio support-ticket create/follow-up and notification mark-read commands. ADR-0018 creates a separate, default-off exception for exactly two QuizCraft Practice commands: create a server-selected session and submit one answer. For that boundary Gateway uses an independent command credential, signs a validated Portal Session UUID only for signed-in requests, and otherwise relays only the Core-issued `quizcraft_anonymous` cookie; it never forwards a generic browser cookie jar or a generic product write. Both Practice command gates remain off until #166. Gateway does not own business data and never exposes service credentials to the browser.

ADR-0027 adds one further exact façade:
`GET /api/v1/library/materials/{material_id}/download` calls Library's signed
download-start command without requiring or inventing a Portal actor. It accepts
only the material ID, validates Library's HTTPS OSS grant and 60-second expiry,
and returns a no-store, no-referrer `303`. This exact route fails closed before
the legacy Library wildcard; it never forwards browser storage authority,
returns the grant as page JSON, or falls back to Portal API or `/materials/`.

ADR-0019 adds exactly one Account Portfolio membership-order command: an
authenticated Portal Session user may create their own order with an empty
browser-owned payload and a required idempotency key. Portal Gateway binds the
actor from the session, validates the returned order and browser-safe
`weixin://` checkout handle, and permits no Portal close, refund, or
refund-status command.

Before QuizCraft #166, `/api/v1/practice/catalog` is a dark V2 handoff seam, not a normal public product proxy. With `PORTAL_ENABLE_QUIZCRAFT_CATALOG=0` (the default), the legacy Practice wildcard returns `404` for that exact path without calling Portal API or QuizCraft. With the flag explicitly `1`, Gateway signs the existing QuizCraft Core catalog contract, maps only bank ID, immutable bank-version ID, name, question count, published chapter IDs/names, and availability, and never substitutes legacy or mock data. Catalog membership is authoritative for setup: if the selected immutable version is absent after refresh, Portal must block every Practice start and ask the user to retry or return to the current catalog. Every catalog read uses the explicit `anonymous` actor: the directory is public and has no user-specific result, while the existing read signature does not bind an actor header, so no browser Session identity enters the Core request.

## Key terms

- **Portal Session**: An encrypted cookie containing UserID, the login-time Display Name snapshot, ExchangeToken, and ExpiresAt. The snapshot is presentation-only and refreshes on the next OAuth login; Platform Core remains the Display Name source of truth. Distinct from Console Session and Core Session.
- **Exchange token**: A Platform Core session exchange token, stored server-side in the encrypted cookie, forwarded to Platform Core for permission checks.
- **Portal permission**: A permission code prefixed with `portal.*` (e.g., `portal.practice.read`), distinct from Console's `console.*` permissions.
- **Portal Practice command**: The ADR-0018-specific signed bridge for only create-session and submit-answer. It is not a product write proxy, and before #166 it must fail closed without contacting QuizCraft Core.
- **Portal Library download façade**: ADR-0027's anonymous exact route that relays one Library-owned short-lived grant as a `303`; it owns no catalog, Object key, or Download Start fact.

## Relationships

- **Portal Gateway → Platform Core**: OAuth code exchange, permission verification. Uses HMAC-SHA256 service-to-service auth.
- **Portal Gateway → Product services**: Signed read-only proxying normally carries `X-Actor-User-Id` and `X-Request-Id`; ADR-0027's exact anonymous Library download command deliberately carries no invented actor.
- **Portal Gateway → Account Portfolio**: Signed authenticated account reads, the ADR-0017 self-service ticket/notification commands, and ADR-0019's self-order creation command only; the Gateway owns neither account balances nor a fallback response.
- **Portal Gateway → QuizCraft Practice Core**: ADR-0018's two default-off commands only, using a credential distinct from catalog reads; QuizCraft remains the owner of session selection, scoring, attempts, and the anonymous identity cookie.
- **Portal Gateway → Library**: ADR-0027's dedicated signed download-start command only; Library owns active eligibility, signing, ledger persistence, and aggregates.
- **Portal frontend → Portal Gateway**: Same-origin API calls with session cookie (`credentials: "same-origin"`).
