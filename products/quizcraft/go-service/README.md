# QuizCraft Go Contract and Import Baseline

> 学生自主运营 · 非河南大学官方项目

This directory is the parallel Go/PostgreSQL foundation for QuizCraft. HC-16 freezes the five-domain API contract and provides deterministic, explicit JSON imports; it does not replace or proxy the existing FastAPI runtime.

HC-17 adds the Practice Core shadow HTTP process. It serves guest sessions, server-side scoring for all four question types, and authenticated progress/wrong-question state while FastAPI remains live. Set `QUIZCRAFT_LEGACY_BASE_URL` and the matching `QUIZCRAFT_LEGACY_COMPARE_SECRET` / FastAPI `QUIZCRAFT_SHADOW_COMPARE_SECRET` to record bounded comparisons through the side-effect-free legacy `/api/practice/shadow-compare` route; legacy errors never change the new API response or legacy statistics.

HC-18 and HC-19 add authenticated per-bank favorites and public rankings derived from immutable Practice Attempts. Ranking defaults to the current UTC Monday-to-Monday week and exposes only the controlled nickname, system avatar, and correct-answer count of visible Ranking Profiles. A user without a visible Profile is absent from public standings; a visible Profile may deliberately leave its nickname blank and is then represented by the neutral `匿名学习者` label. The four allowed avatars are `scholar-blue`, `coder-green`, `reader-amber`, and `owl-purple`.

The historical bootstrap importer is no longer an activation path. `cmd/importbank` now exits with a migration message so an operator cannot bypass Workshop review. Apply the versioned migration command, start the service, then use the authenticated Workshop import endpoint:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' go run ./cmd/migrate
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

Create the target as the independently owned `quizcraft_v2` database; it must not reuse the legacy database name, database OID, or credentials. The operator commands accept only `QUIZCRAFT_V2_DATABASE_URL` whose configured and connected database name is exactly `quizcraft_v2`; they do not use the runtime service's generic `DATABASE_URL`. Apply the ordered schema artifacts with the idempotent migration command. It records each source SHA-256 and fails closed if an already-applied file changes:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' go run ./cmd/migrate
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' go run ./cmd/migrate # reports only skipped artifacts
```

An existing released V2 schema that was manually applied through `000008` is adopted only on PostgreSQL 16 when its columns, constraints, indexes, non-internal triggers, and trigger functions match the frozen released catalog fingerprint; the report records those immutable checksums as `adopted` and then applies every later tracked artifact (currently `000009` and `000010`). A partial or altered untracked schema fails before a migration-history table is created.

Freeze a read-only legacy snapshot before the full import. The artifact includes the source database identity and a content checksum, and is create-only so it can be reused after an interruption. Then run the full PostgreSQL-to-PostgreSQL migration:

```bash
LEGACY_DATABASE_URL='postgres://.../quizcraft_legacy' \
go run ./cmd/reconcile -mode snapshot \
  -source-name quizcraft-legacy-production \
  -snapshot-file /root/quizcraft-migration/legacy-snapshot.json

QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
go run ./cmd/reconcile -mode full \
  -snapshot-file /root/quizcraft-migration/legacy-snapshot.json
```

`full` holds one target-wide advisory lock and rejects a target containing any V2 business fact, so an interrupted run must use `resume` with the same artifact instead of beginning another full import. The JSON report independently records bank/question/answered counts, question types, chapters, canonical answer SHA-256, content SHA-256, source snapshot SHA-256, and zero/unresolved exception state. It also compares every V2 bank, active and inactive bank version, question, question version, and immutable membership to the frozen import facts. Resolvable feedback is copied with stable bank/question/version references and no guessed user mapping. Unresolvable feedback produces an immutable exception fact and blocks the report. Legacy standings are stored as an immutable snapshot; `GET /api/v1/rankings/legacy` exposes only rank, display name, correct count, and total, never a legacy subject key.

If the import process is interrupted after it has emitted a `run_id`, run the same immutable artifact again. It rejects a changed snapshot and preserves the original stable IDs instead of creating a second run:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
go run ./cmd/reconcile -mode resume -run-id '<run UUID>' \
  -snapshot-file /root/quizcraft-migration/legacy-snapshot.json
```

Use the returned `run_id` to catch up the transactional legacy event log. The command accepts increasing PostgreSQL sequence IDs (rolled-back transactions may leave numeric gaps), advances an audited cursor, and exits non-zero while lag or unresolved exceptions remain:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
LEGACY_DATABASE_URL='postgres://.../quizcraft_legacy' \
go run ./cmd/reconcile -mode catch-up -run-id '<run UUID>'
```

Finally evaluate a fixed, observed shadow window. Both mismatches and legacy errors count toward the rate, insufficient samples block, and every decision is immutable:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
go run ./cmd/reconcile -mode shadow-gate \
  -window-start '2026-07-20T00:00:00Z' \
  -window-end '2026-07-21T00:00:00Z' \
  -minimum-samples 1000 \
  -mismatch-threshold 0.001
```

Before any maintenance-window cutover, run a separate backup/restore rehearsal. It retains the custom-format dump for audit, restores it into a generated database, verifies each required table's row count and deterministic content summary against the source, then drops only that generated restore database:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
QUIZCRAFT_RESTORE_ADMIN_URL='postgres://.../postgres' \
go run ./cmd/backuprestore \
  -backup-directory /root/quizcraft-migration/backups
```

The artifact commands create no Practice Session, Answer, learning-state, favorite, ranking, or Portal routing fact. They prepare evidence only; Portal reads and the Go write promise remain controlled by the separate cutover ticket and runbook. See [`docs/migration-artifacts.md`](docs/migration-artifacts.md) for the operator sequence and recovery boundary.

A zero exit status means only that reconciliation/catch-up/shadow evidence passed. It does not authorize production traffic movement; gradual reads, write cutover, rollback snapshots, and the legacy read-only observation window belong to the separate cutover workflow.

The production sequence, rollback boundary, service/Nginx examples, and live SHA-aware smoke are defined in [`docs/cutover-runbook.md`](docs/cutover-runbook.md). Go writes are disabled unless `QUIZCRAFT_WRITES_ENABLED=1`; the maintenance release uses one `VITE_QUIZCRAFT_GO_WRITES=1` browser bundle to switch all reads and writes atomically, without percentage cohorts.
