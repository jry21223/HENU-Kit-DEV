# Career Opportunities Service

Career Opportunities owns the Work Radar search job and its normalized result
facts. It is the async core behind the 05 求职雷达 module.

Apply `db/migrations/000001_career_searches.up.sql`, configure `.env.example`,
then run:

```bash
go run ./cmd/server
```

PostgreSQL owns search jobs, the frozen profile snapshot, and the durable
actor-scoped idempotency ledger. Redis stores only short-lived HMAC nonces.
Portal Gateway signs the verified actor into `X-Actor-User-Id`; the service
never trusts a browser-supplied `user_id` and stores no `lifetime` flag (Career
is not a membership authority). A background worker claims `queued` searches
and advances `queued -> running -> completed`, or lands on `failed` with a
stable browser-safe error code.

## Ownership boundary

**Owns**: career search jobs, status/stage, frozen `profile_snapshot`,
normalized search results, and the create idempotency ledger.

**Does not own**: Portal accounts/sessions, membership entitlement (`lifetime`
lives in Account Portfolio), the GetWork crawler (integrated behind the
`Work` seam in #396), or email delivery (#397).
