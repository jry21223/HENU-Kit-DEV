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

Portal Gateway is a read-only BFF. It authenticates users via Platform Core OAuth, establishes a Portal Session, and proxies GET requests to product services with verified permissions. It does not forward write operations, does not own business data, and does not expose service credentials to the browser.

## Key terms

- **Portal Session**: An encrypted cookie containing UserID, the login-time Display Name snapshot, ExchangeToken, and ExpiresAt. The snapshot is presentation-only and refreshes on the next OAuth login; Platform Core remains the Display Name source of truth. Distinct from Console Session and Core Session.
- **Exchange token**: A Platform Core session exchange token, stored server-side in the encrypted cookie, forwarded to Platform Core for permission checks.
- **Portal permission**: A permission code prefixed with `portal.*` (e.g., `portal.library.read`), distinct from Console's `console.*` permissions.

## Relationships

- **Portal Gateway → Platform Core**: OAuth code exchange, permission verification. Uses HMAC-SHA256 service-to-service auth.
- **Portal Gateway → Product services**: Signed read-only proxying. Each request carries `X-Actor-User-Id` and `X-Request-Id`.
- **Portal Gateway → Account Portfolio**: Signed authenticated account reads; the Gateway owns neither account balances nor a fallback response.
- **Portal frontend → Portal Gateway**: Same-origin API calls with session cookie (`credentials: "same-origin"`).
