# QuizCraft Go Contract and Import Baseline

This directory is the parallel Go/PostgreSQL foundation for QuizCraft. HC-16 freezes the five-domain API contract and provides deterministic, explicit JSON imports; it does not replace or proxy the existing FastAPI runtime.

HC-17 adds the Practice Core shadow HTTP process. It serves guest sessions, server-side scoring for all four question types, and authenticated progress/wrong-question state while FastAPI remains live. Set `QUIZCRAFT_LEGACY_BASE_URL` and the matching `QUIZCRAFT_LEGACY_COMPARE_SECRET` / FastAPI `QUIZCRAFT_SHADOW_COMPARE_SECRET` to record bounded comparisons through the side-effect-free legacy `/api/practice/shadow-compare` route; legacy errors never change the new API response or legacy statistics.

HC-18 and HC-19 add authenticated per-bank favorites and public rankings derived from immutable Practice Attempts. Ranking defaults to the current UTC Monday-to-Monday week and exposes only the controlled nickname, system avatar, and correct-answer count of opted-in profiles. The four allowed avatars are `scholar-blue`, `coder-green`, `reader-amber`, and `owl-purple`.

The historical bootstrap importer is no longer an activation path. `cmd/importbank` now exits with a migration message so an operator cannot bypass Workshop review. Apply migrations, start the service, then use the authenticated Workshop import endpoint:

```bash
for migration in db/migrations/*.up.sql; do psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$migration"; done
go run ./cmd/server
```

The Workshop import response contains source/content SHA-256 hashes, stable bank/question IDs, immutable version IDs, and question/type/chapter/answer counts. Validation failures write no bank content. An accepted import remains a draft until an authorized operator reads the full version, records human validation, and separately publishes it.

Stable IDs use a fixed UUIDv5 namespace: `bank_key` determines the Bank ID and the pair of Bank ID plus source question ID determines the Question ID. Operators must therefore treat both source keys as immutable identities. Semantic content and answer hashes determine new immutable version IDs without changing those stable identities.

PostgreSQL is the only runtime source of truth. No Go startup path scans JSON directories or falls back to local files; JSON enters the new runtime only through the scoped Workshop endpoint.

Run the shadow process after applying every migration and configuring `.env.example`:

```bash
go run ./cmd/server
```

After a UTC week closes, record its idempotent, reward-free Overall and per-bank settlement facts with:

```bash
go run ./cmd/settleranking
```

The optional `-at` RFC3339 instant is for deterministic recovery/testing. Settlement rows contain only the period, scope, `correct_answer_count` standings, and audit timestamps; they cannot be updated or deleted and do not grant points, membership, entitlements, or other rewards.

## Workshop and correction feedback

`/auth/login` and `/auth/callback` perform state-bound S256 PKCE and a server-side single-use code exchange with Platform Core. The exchange token is held only in an encrypted `__Host-quizcraft_session` HttpOnly/Secure cookie. Every `/api/v1/workshop/**` request asks Platform Core for the exact `quizcraft.workshop.read`, `.write`, or `.publish` permission and QuizCraft product/matching bank resource Scope, so revocation is not hidden by local claims; the Go service does not read `ADMIN_TOKEN`. Configure the `PLATFORM_CORE_*`, `QUIZCRAFT_PLATFORM_*`, `QUIZCRAFT_PUBLIC_URL`, and 32-byte `QUIZCRAFT_SESSION_ENCRYPTION_KEY` values from `.env.example` together.

Imported content is sealed as an immutable draft. `GET /api/v1/workshop/banks/{bank_id}/versions/{bank_version_id}` supplies the authorized full question/answer review view, and publication remains unavailable until an explicit human-validation event. Create, validate, publish, unpublish, and rollback commands use actor-scoped idempotency, optimistic `expected_version`, one database transaction, and append-only audit events.

`POST /api/v1/feedback` persists only a stable `(bank_id, question_id, question_version_id)` correction reference with full detail in QuizCraft. Its transactional Operations Inbox outbox contains only the feedback reference, QuizCraft deep link, category, and priority metadata; a bounded dispatcher delivers those fields to Platform Core with a rotated `QUIZCRAFT_INBOX_EXCHANGE_TOKEN` and idempotent retries. The full correction remains available only from QuizCraft's scoped deep-link endpoint.

`GET /api/v1/console-summary` exposes only published-bank, draft-validation, and pending Inbox-delivery counts. It requires the Console Gateway's dedicated Basic + HMAC credential, stores each nonce atomically to reject replay, and preserves the signed child `X-Request-Id`; the Console links back to `/extract` for every editing action. Enable the Workshop independently with `VITE_QUIZCRAFT_WORKSHOP=1`; `VITE_QUIZCRAFT_GO_SHADOW` continues to control Practice shadow traffic only.

An unauthenticated browser receives an unguessable `quizcraft_anonymous` HttpOnly, Secure, SameSite=Lax cookie and can practice without creating a Core user. Legacy short-lived local sessions remain limited to learner state during the shadow phase; when Platform Core is configured they cannot authorize Workshop operations. Every session creation and answer submission requires an `Idempotency-Key`; `(session, question)` uniqueness and transaction locks prevent concurrent duplicate scoring.

Generate and verify contract bindings:

```bash
bash scripts/generate-contract.sh
docker run --rm -v "$PWD:/src" -w /src sqlc/sqlc:1.31.0 generate
git diff --exit-code -- internal/contract internal/store ../web-app/src/generated/quizcraft-api
go test -race ./...
```

The pinned generator emits Go contract types under `internal/contract` and a fetch-based TypeScript client under `web-app/src/generated/quizcraft-api`. CI regenerates both and fails on drift before running the breaking-change check.

## Reconciliation and shadow gate

Before the legacy service receives any migration-window writes, deploy its updated `db_storage.py` and run the normal `init_schema()` startup path. Bank, feedback, and ranking writes then append `quizcraft_migration_events` in the same PostgreSQL transaction. Event payloads preserve legacy subjects only as legacy snapshot keys; they never create Platform Core users or `quizcraft_ranking_profiles`.

Apply every target migration to a physically separate, empty temporary QuizCraft database. Then run the full PostgreSQL-to-PostgreSQL migration:

```bash
DATABASE_URL='postgres://.../quizcraft_temp' \
LEGACY_DATABASE_URL='postgres://.../quizcraft_legacy' \
go run ./cmd/reconcile -mode full -source-name quizcraft-legacy-production
```

The JSON report independently records bank/question/answered counts, question types, chapters, canonical answer SHA-256, and content SHA-256 for the source import and stored target rows. Resolvable feedback is copied with stable bank/question/version references and no guessed user mapping. Unresolvable feedback produces an immutable exception fact and blocks the report. Legacy standings are stored as an immutable snapshot; `GET /api/v1/rankings/legacy` exposes only rank, display name, correct count, and total, never a legacy subject key.

Use the returned `run_id` to catch up the transactional legacy event log. The command accepts increasing PostgreSQL sequence IDs (rolled-back transactions may leave numeric gaps), advances an audited cursor, and exits non-zero while lag or unresolved exceptions remain:

```bash
go run ./cmd/reconcile -mode catch-up -run-id '<run UUID>'
```

Finally evaluate a fixed, observed shadow window. Both mismatches and legacy errors count toward the rate, insufficient samples block, and every decision is immutable:

```bash
go run ./cmd/reconcile -mode shadow-gate \
  -window-start '2026-07-20T00:00:00Z' \
  -window-end '2026-07-21T00:00:00Z' \
  -minimum-samples 1000 \
  -mismatch-threshold 0.001
```

A zero exit status means only that reconciliation/catch-up/shadow evidence passed. It does not authorize production traffic movement; gradual reads, write cutover, rollback snapshots, and the legacy read-only observation window belong to the separate cutover workflow.

The production sequence, rollback boundary, service/Nginx examples, and live SHA-aware smoke are defined in [`docs/cutover-runbook.md`](docs/cutover-runbook.md). Go writes are disabled unless `QUIZCRAFT_WRITES_ENABLED=1`; the maintenance release uses one `VITE_QUIZCRAFT_GO_WRITES=1` browser bundle to switch all reads and writes atomically, without percentage cohorts.
