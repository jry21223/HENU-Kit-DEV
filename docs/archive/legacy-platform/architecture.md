# V2 Architecture

V2 uses a greenfield monorepo. The old Next.js route-handler backend and Prisma schema are archived in `legacy/v1-next-prisma` and are not runtime dependencies.

## Runtime Services

- `apps/web`: Next.js student web app. It calls the Go API only.
- `apps/admin`: Vue 3 admin console. It calls the Go API only.
- `services/api`: Go Gin monolith with PostgreSQL, Redis, GORM, JWT auth, uploads, payment, AI orchestration, and admin APIs.
- `services/worker`: Go async worker using Redis Streams for AI and background jobs.
- `postgres` and `redis`: local development infrastructure.

## Principles

- V2 APIs are the only supported APIs.
- No Prisma-to-GORM migration and no V1 compatibility layer.
- WeChat Pay Native is the target payment provider.
- AI output is draft-only until reviewed.
- Frontends never access the database directly.
