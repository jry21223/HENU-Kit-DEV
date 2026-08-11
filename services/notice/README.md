# Notice service

Independent owner for Notice sources, immutable versions, review, audience, and distribution lifecycle.

Review the [migration runbook](db/MIGRATION.md), apply `db/migrations/000001_notice.up.sql`, and configure the values shown in `.env.example`. Run the API and delivery Worker as separate processes:

```bash
go run ./cmd/server
go run ./cmd/worker
```

Notice has two fixed, distinct signed API clients. The Console Gateway uses
`NOTICE_SERVICE_*` for management calls and carries a Platform Core-verified
actor, permission code, and product Scope. The Portal Gateway uses the dedicated
`NOTICE_PORTAL_*` capability only for its actor-bound, all-students in-app read
route; it cannot call Console management routes. Generate the two credential
pairs independently and never reuse their client IDs, key IDs, or secrets.

HMAC nonces use Redis only for atomic replay coordination; PostgreSQL remains the durable source of Notice facts, 24-hour idempotency history, and append-only audit events. The Worker claims queued deliveries with `SKIP LOCKED`, retries provider failures up to three times, and records `delivered` or `failed` as an audited fact.
