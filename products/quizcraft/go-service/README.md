# QuizCraft Go Contract and Import Baseline

This directory is the parallel Go/PostgreSQL foundation for QuizCraft. HC-16 freezes the five-domain API contract and provides deterministic, explicit JSON imports; it does not replace or proxy the existing FastAPI runtime.

HC-17 adds the Practice Core shadow HTTP process. It serves guest sessions, server-side scoring for all four question types, and authenticated progress/wrong-question state while FastAPI remains live. Set `QUIZCRAFT_LEGACY_BASE_URL` and the matching `QUIZCRAFT_LEGACY_COMPARE_SECRET` / FastAPI `QUIZCRAFT_SHADOW_COMPARE_SECRET` to record bounded comparisons through the side-effect-free legacy `/api/practice/shadow-compare` route; legacy errors never change the new API response or legacy statistics.

Apply the migration, then import one named file explicitly:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/migrations/000001_quizcraft_content.up.sql
go run ./cmd/importbank \
  --bank-key programming-basics \
  --file /absolute/path/to/bank.json
```

Set `DATABASE_URL` in the process environment as shown above. The optional `--database-url` override is intended only for disposable local databases because command arguments may be visible in shell history and process listings.

The command prints a JSON report with source/content SHA-256 hashes, stable bank/question IDs, immutable version IDs, and question/type/chapter/answer counts. Validation failures return a non-zero status and write no bank content.

Stable IDs use a fixed UUIDv5 namespace: `bank_key` determines the Bank ID and the pair of Bank ID plus source question ID determines the Question ID. Operators must therefore treat both source keys as immutable identities. Semantic content and answer hashes determine new immutable version IDs without changing those stable identities.

PostgreSQL is the only runtime source of truth for this baseline. No Go startup path scans JSON directories or falls back to local files. JSON is read only when an operator invokes `importbank` with `--file`.

Run the shadow process after applying every migration and configuring `.env.example`:

```bash
go run ./cmd/server
```

An unauthenticated browser receives an unguessable `quizcraft_anonymous` HttpOnly, Secure, SameSite=Lax cookie and can practice without creating a Core user. After a server-side Platform identity exchange, the business site supplies its short-lived QuizCraft-local session through the HttpOnly `quizcraft_session` cookie; trusted non-browser clients may use the equivalent local bearer JWT. Platform Core exchange tokens and client-provided legacy user IDs are never accepted directly as identity evidence. Every session creation and answer submission requires an `Idempotency-Key`; `(session, question)` uniqueness and transaction locks prevent concurrent duplicate scoring.

Generate and verify contract bindings:

```bash
bash scripts/generate-contract.sh
docker run --rm -v "$PWD:/src" -w /src sqlc/sqlc:1.31.0 generate
git diff --exit-code -- internal/contract internal/store ../web-app/src/generated/quizcraft-api
go test -race ./...
```

The pinned generator emits Go contract types under `internal/contract` and a fetch-based TypeScript client under `web-app/src/generated/quizcraft-api`. CI regenerates both and fails on drift before running the breaking-change check.
