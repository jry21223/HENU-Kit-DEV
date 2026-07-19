# Library Compatibility Adapter

Migration boundary between HENUKit Console and Study Legacy API. It exposes only courses, materials, downloads, material submission review, and course/material correction operations.

Apply `db/migrations/000001_library_adapter.up.sql`, configure `.env.example`, then run:

```bash
go run ./cmd/server
```

The service has no Study database credentials. PostgreSQL stores only the adapter's 24-hour idempotency ledger and append-only audit events; Redis stores short-lived HMAC nonces. A failed legacy source is represented as `partial` or `unavailable`, never as an empty successful result. Community, quiz, commerce, points, and membership routes and fields are rejected at the adapter boundary.
