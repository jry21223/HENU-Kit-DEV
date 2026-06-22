# Development

## Requirements

- Docker Desktop
- Node.js 22+
- Go 1.23+ for local Go commands, or Docker for containerized checks

## Commands

```bash
docker compose -f docker-compose.dev.yml config
docker compose -f docker-compose.dev.yml up --build
npm install
npm run build
npm audit --audit-level=low
cd services/api && go test ./...
cd ../worker && go test ./...
```

## Local Ports

- API: `8080`
- Web: `3000`
- Admin: `5173`
- PostgreSQL: `5432`
- Redis: `6379`

## API Seed

```bash
cd services/api
go run ./cmd/seed
```

The seed command runs AutoMigrate and inserts the current demo school, majors, courses, one material, one question, and demo users.
