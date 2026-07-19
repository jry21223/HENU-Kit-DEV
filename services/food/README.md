# Food Operations Service

Food owns the operational facts and HTTP contract for submission review, anomaly-ticket handling, and tier-adjustment confirmation.

Apply `db/migrations/000001_food_operations.up.sql`, configure `.env.example`, then run:

```bash
go run ./cmd/server
```

PostgreSQL owns Food records, durable actor-scoped idempotency results, optimistic versions, and append-only audit events. Redis stores only short-lived HMAC nonces. Console Gateway signs verified actor, permission, and product Scope headers and never receives Food database credentials. Reads return explicit `ok`, `empty`, or `stale`; dependency failures return an error instead of an empty success.
