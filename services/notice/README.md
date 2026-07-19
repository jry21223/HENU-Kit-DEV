# Notice service

Independent owner for Notice sources, immutable versions, review, audience, and distribution lifecycle.

Review the [migration runbook](db/MIGRATION.md), apply `db/migrations/000001_notice.up.sql`, and configure the values shown in `.env.example`. Run the API and delivery Worker as separate processes:

```bash
go run ./cmd/server
go run ./cmd/worker
```

The Console Gateway is the only supported API client. It must authenticate each call and carry a Platform Core-verified actor, permission code, and product Scope. HMAC nonces use Redis only for atomic replay coordination; PostgreSQL remains the durable source of Notice facts, 24-hour idempotency history, and append-only audit events. The Worker claims queued deliveries with `SKIP LOCKED`, retries provider failures up to three times, and records `delivered` or `failed` as an audited fact.
