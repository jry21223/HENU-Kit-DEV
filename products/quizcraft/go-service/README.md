# Practice service

Go/PostgreSQL service for HENU Kit question banks, practice sessions, server-side answer evaluation, favorites, rankings, personal statistics, and correction feedback.

## Browser boundary

HENU Kit Portal and Portal Gateway own authenticated browser journeys. This service does not expose an independent OAuth login or callback and does not issue new product sessions. Guest practice uses only the service-issued anonymous HttpOnly cookie; authenticated Portal commands bind the signed Platform user ID on the server.

The versioned bank-administration API remains frozen for trusted migration callers only. It has no browser page, navigation entry, admin-token form, or independent login. Removing that API requires a separate breaking-contract release.

## Runtime

Required inputs are documented in `.env.example`. PostgreSQL is the only runtime source of truth; startup never scans local JSON or turns fixtures into production success. Portal read and command clients use separate HMAC credentials, nonce replay protection, bounded bodies, and default-off write gates.

Apply migrations:

```bash
go run ./cmd/migrate -database "$DATABASE_URL"
```

Run locally:

```bash
go run ./cmd/server
```

## Verification

```bash
go test ./...
bash scripts/generate-contract.sh
git diff --exit-code -- internal/contract ../web-app/src/generated/quizcraft-api
```

The browser cutover verifier exercises real guest practice, answer submission, correction feedback, ranking, retired-route convergence, and visible-copy checks at desktop and 390px. It requires HTTPS and does not inject an independent product login Session.

## Operational claims

Keep these states separate: candidate build, CI result, merge SHA, deployed SHA, and production user journey. `/healthz`, `/readyz`, HTTP 200, or a single redirect are not acceptance evidence for practice or authentication.
