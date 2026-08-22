# Career Opportunities Service

Career Opportunities owns the Work Radar search job and its normalized result
facts. It is the async core behind the 05 求职雷达 module.

Apply every `db/migrations/*.up.sql` file in lexical order, configure
`.env.example`, then run:

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
normalized search results, the authorized official-source adapters, the create
idempotency ledger, and resume AI-extraction jobs.

**Does not own**: Portal accounts/sessions, membership entitlement (`lifetime`
lives in Account Portfolio), the AI provider (a custom OpenAI-compatible
endpoint configured by the operator), or email delivery (#397).

## Authorized opportunity sources

`CAREER_SOURCE_ALLOWLIST=official.meituan` enables the independently written
Meituan campus-recruitment adapter. The adapter reads only Meituan's public
official JSON endpoint, normalizes source-attributed jobs, and scores them
against the frozen Career Profile. Unknown allowlist keys fail startup. An
empty allowlist remains the emergency kill switch for local/degraded use.

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
- PDF resumes are rendered in memory into at most 10 bounded JPEG pages and
  sent as `image_url` parts, because the configured SGLang endpoint rejects
  raw PDF/file parts. DOCX/TXT inputs are converted to bounded text. No source
  or rendered file is written to disk.
- Production sets `CAREER_REQUIRE_AI=1`; startup rejects an empty, partial,
  placeholder, or mock provider configuration, then runs a non-user-data PDF
  extraction probe before serving traffic. Plaintext public HTTP stays blocked
  unless `CAREER_ALLOW_INSECURE_AI_HTTP=1` and the base URL is exactly
  `http://125.46.96.207:30000/v1`; that exception exposes rendered resume pages
  and extracted profile output to the network without transport encryption.
- `CAREER_EXTRACT_RATE_LIMIT` (default 5) bounds extractions per actor per
  hour via a short-lived Redis counter.

`CAREER_SEARCH_RATE_LIMIT` (default 10 per UTC hour) and
`CAREER_SEARCH_ACTIVE_LIMIT` (default 1) bound crawler and digest work per
actor. Exact idempotent replays bypass both gates; Redis accounting fails
closed, so an unavailable limiter never creates an unmetered task.

Completed searches persist their Platform Core enqueue state in
`career_digest_deliveries` (migration `000004`). A transient enqueue failure stays independent from
the completed result and is reclaimed with a bounded delay; Platform Core's
search-scoped idempotency key makes the retry safe.
