# Account Portfolio

Account Portfolio is the sole persistent owner of user points, memberships,
membership orders, notifications, and support tickets. It does not import or
translate legacy Study account data.

The service accepts separately credentialed, actor-bound signed requests from
Portal Gateway and Console Gateway. Browser clients use the same-origin Portal
Gateway contract and never receive service credentials or an actor identifier
they can assert themselves. Console Gateway is the only operator caller and
must use a credential distinct from Portal Gateway's.

The ¥9.9 lifetime Membership Order kernel is durable, but the process does not
read a payment Provider credential or enable a real Provider. A missing
Provider returns an explicit unavailable result and creates neither an order
nor an entitlement. Portal keeps the purchase surface closed until a separate
Provider Spike and authorization establish a compliant browser boundary.

When an approved Provider adapter is enabled in a later ticket, the service
commits a local order plus stable merchant-order intent before any external
create call. Retried creates and verified callbacks reuse that intent to repair
an external order that was created before its local binding committed. Callback
data must exactly match the Provider query before a verified payment fact can
change the entitlement; the membership retains the current paid fact so an
older refund cannot revoke a newer valid lifetime purchase.

The merchant ID is a private, service-generated intent value rather than the
public order ID. Adapter `CreateOrder` implementations must be idempotent by
that merchant ID; the service also commits a short dispatch lease so concurrent
same-key retries do not start duplicate creates, while an expired lease or a
verified callback can recover a crashed dispatcher.

## Run

Set `ACCOUNT_PORTFOLIO_DATABASE_URL`, `ACCOUNT_PORTFOLIO_SERVICE_CLIENT_ID`,
`ACCOUNT_PORTFOLIO_SERVICE_KEY_ID`, and `ACCOUNT_PORTFOLIO_SERVICE_SECRET`,
plus the Console caller's `ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID`,
`ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID`, and `ACCOUNT_PORTFOLIO_CONSOLE_SECRET`.
Also set `ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY` to a separately stored Base64
encoding of exactly 32 random bytes (for example, `openssl rand -base64 32`).
It is the Owner-only AES-256-GCM key for point-ledger continuation tokens; do
not reuse a gateway credential, return it, or log it. Then run:

```bash
go run ./cmd/server
```

The server applies its embedded additive migrations before listening. For a
separate migration step, run `go run ./cmd/migrate` with only the database URL.
`GET /healthz` returns success only when PostgreSQL is reachable.

For existing HENU Kit PostgreSQL volumes, the fixed-SHA release helper creates
the `account_portfolio` database before this service starts; it never imports
legacy Study account data.
