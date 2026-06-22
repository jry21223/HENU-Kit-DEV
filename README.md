# Final Review Platform V2

V2 is a greenfield rebuild of the one-stop study and final-review platform. The old Next.js + Prisma app has been archived under `legacy/v1-next-prisma` for reference only.

## Current Status

- V2 monorepo skeleton is being built.
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
npm run build:web
npm run build:admin
```

If Go is installed locally:

```bash
cd services/api && go test ./...
cd ../worker && go test ./...
```

## Security Notes

- Do not commit `.env`, JWT private keys, WeChat Pay keys, LLM API keys, or real course PDFs.
- `uploads/` is runtime storage and is ignored except placeholder files.
- Production must not use fixed verification codes or mock payment.
