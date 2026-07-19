# QuizCraft Go Contract and Import Baseline

This directory is the parallel Go/PostgreSQL foundation for QuizCraft. HC-16 freezes the five-domain API contract and provides deterministic, explicit JSON imports; it does not replace or proxy the existing FastAPI runtime.

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

Generate and verify contract bindings:

```bash
bash scripts/generate-contract.sh
git diff --exit-code -- internal/contract ../web-app/src/generated/quizcraft-api
go test -race ./...
```

The pinned generator emits Go contract types under `internal/contract` and a fetch-based TypeScript client under `web-app/src/generated/quizcraft-api`. CI regenerates both and fails on drift before running the breaking-change check.
