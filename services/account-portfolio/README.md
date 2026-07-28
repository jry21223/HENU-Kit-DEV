# Account Portfolio

Account Portfolio is the sole persistent owner of user points, memberships,
membership orders, notifications, and support tickets. It does not import or
translate legacy Study account data.

The service accepts only Portal Gateway's signed service requests. Browser
clients use the same-origin Portal Gateway contract and never receive service
credentials or an actor identifier they can assert themselves.

## Run

Set `ACCOUNT_PORTFOLIO_DATABASE_URL`, `ACCOUNT_PORTFOLIO_SERVICE_CLIENT_ID`,
`ACCOUNT_PORTFOLIO_SERVICE_KEY_ID`, and `ACCOUNT_PORTFOLIO_SERVICE_SECRET`,
then run:

```bash
go run ./cmd/server
```

The server applies its embedded additive migrations before listening. For a
separate migration step, run `go run ./cmd/migrate` with only the database URL.
`GET /healthz` returns success only when PostgreSQL is reachable.

For existing HENU Kit PostgreSQL volumes, the fixed-SHA release helper creates
the `account_portfolio` database before this service starts; it never imports
legacy Study account data.
