# Development

## Requirements

- Docker Desktop
- Node.js 22+
- Go 1.25+ for local Go commands, or Docker for containerized checks

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

The seed command runs AutoMigrate and inserts the current demo school, majors, courses, materials, questions, community content, mock AI task, and demo users.

## Material Manifest Import

Prepared course files can be imported through a manifest after the files are mounted or copied under `LOCAL_UPLOAD_DIR`:

```bash
cd services/api
go run ./cmd/import-materials ../../data/material-manifest.example.json
```

The importer is safe to run repeatedly. It upserts schools, colleges, majors, courses, packages, and materials, then idempotently binds imported materials to the course package. File paths in the manifest must resolve inside `LOCAL_UPLOAD_DIR`; missing files and traversal attempts fail the import transaction.
