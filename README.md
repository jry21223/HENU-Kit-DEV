# Final Review Platform V2

V2 is a greenfield rebuild of the one-stop study and final-review platform. The old Next.js + Prisma app has been archived under `legacy/v1-next-prisma` for reference only.

## Current Status

- V2 monorepo skeleton is in place.
- The Go API includes health/version, email-code auth, JWT cookies/tokens, role middleware, school/course/material endpoints, admin CRUD for organization/course/material records, local material upload guards, material download permission checks, quiz submission, wrong-question records, and basic weakness counts.
- Go API, Go Worker, Next.js Web, Vue Admin, PostgreSQL, and Redis are the target runtime.
- No production data migration is planned.
- WeChat Pay Native is the target payment provider; local development starts with mock payment boundaries.
- AI uses mock LLM first and must never publish generated content without review.

## Directory Layout

```txt
apps/web                 Next.js student web app
apps/admin               Vue 3 admin console
services/api             Go Gin/GORM monolith API
services/worker          Go Redis Streams worker
integrations/langbot-sales-agent
legacy/v1-next-prisma    Archived V1 reference implementation
infra                    Docker and nginx support files
docs                     V2 docs
scripts                  Seed and development scripts
uploads                  Local runtime storage, ignored except .gitkeep files
```

## Quick Start

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up --build
```

Expected local ports:

- Web: `http://localhost:3000`
- Admin: `http://localhost:5173`
- API: `http://localhost:8080/api/v1/healthz`

## Local Checks

```bash
docker compose -f docker-compose.dev.yml config
npm install
npm run build
npm audit --audit-level=low
```

If Go is installed locally:

```bash
cd services/api && go test ./...
cd ../worker && go test ./...
```

This repo can also be checked with a portable Go toolchain under `.tools/`; `.tools/` is ignored and must not be committed.

Implemented API checks currently cover:

- email-code login and role denial
- material detail not leaking storage keys
- free/login-required/paid material download gates
- unsafe material storage keys returning 404
- admin-only organization/course/material mutation
- upload filename, extension, content, and size guards
- quiz list/detail not leaking answers
- quiz submission and wrong-question user isolation

## Seed Data

After PostgreSQL is available:

```bash
cd services/api
go run ./cmd/seed
```

The seed creates demo organization/course data and demo accounts:

- `admin@example.com`
- `reviewer@example.com`
- `creator@example.com`
- `user@example.com`

Development verification code is `123456` by default. Production must configure real delivery and must not depend on a fixed code.

## Security Notes

- Do not commit `.env`, JWT private keys, WeChat Pay keys, LLM API keys, or real course PDFs.
- `uploads/` is runtime storage and is ignored except placeholder files.
- Production must not use fixed verification codes or mock payment.
- Paid material access is enforced by the Go API before serving local files; the current V2 foundation checks direct material grants only. Course package grants are still a later module.
