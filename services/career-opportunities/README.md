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
normalized search results, the create idempotency ledger, and resume
AI-extraction jobs.

**Does not own**: Portal accounts/sessions, membership entitlement (`lifetime`
lives in Account Portfolio), the GetWork crawler (integrated behind the
`Work` seam in #396), the AI provider (a custom OpenAI-compatible endpoint
configured by the operator), or email delivery (#397).

## Resume extraction (上传简历 → AI 识别)

Apply `db/migrations/000003_career_resume_extractions.up.sql` alongside the
earlier migrations. The upload boundary is `POST/GET
/api/v1/career/profile/extractions`, gated like every Career route.

- Uploaded files (PDF / DOCX / TXT, ≤ 10 MiB) are held **transiently** in the
  job row and purged once the worker processes the job; only the extracted
  text fields stay durable — the "不存储简历文件" promise holds end to end.
- The AI provider is a custom OpenAI-compatible `/chat/completions` endpoint:
  `CAREER_AI_BASE_URL`, `CAREER_AI_API_KEY`, `CAREER_AI_MODEL`. Empty config
  is the production-safe off state (uploads rejected with `AI_UNCONFIGURED`);
  `CAREER_AI_MODE=mock` selects the deterministic dev extractor.
- `CAREER_EXTRACT_RATE_LIMIT` (default 5) bounds extractions per actor per
  hour via a short-lived Redis counter.
