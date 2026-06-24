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
go run ./cmd/import-materials -dry-run ../../data/material-manifest.example.json
go run ./cmd/import-materials ../../data/material-manifest.example.json
```

The importer is safe to run repeatedly. It upserts schools, colleges, majors, courses, packages, and materials, then idempotently binds imported materials to the course package. File paths in the manifest must resolve inside `LOCAL_UPLOAD_DIR`; missing files and traversal attempts fail the import transaction.

Use `-dry-run` before importing real internal materials. Dry-run mode executes the same validation, upsert, package-bind, and report path inside a rolled-back transaction, returns `"dryRun": true`, and reports the planned create/update/bind counts without persisting rows.

The import JSON includes a `report` block for acceptance checks:

- `filesChecked` and `totalFileBytes` confirm the mounted files resolved under `LOCAL_UPLOAD_DIR`.
- `accessLevels`, `statuses`, and `types` summarize what will be exposed as free/login-required/paid/member-only and draft/published/etc.
- `paidMaterials`, `publishedMaterials`, and `packageItemLinks` show whether paid assets are represented and bound to course packages.
- `packages` gives per-package material, paid material, access-level, item-link, and byte totals.
- `duplicateFiles` flags multiple manifest materials that point at the same storage file; this is allowed but should be manually reviewed before an internal release.

Material import delivery smoke:

- `TestMaterialManifestImportSmokeCoversPaidDownloadDelivery` imports temporary fixture files through the manifest importer, then exercises the public package detail API and material download API.
- The smoke verifies free downloads, login-required downloads, paid denial without entitlement, package-grant unlock, and successful paid download audit logging.
- This is automated safety coverage, not proof that real internal course files have been imported. For an internal release, mount the real `uploads/materials/...` directory, run `go run ./cmd/import-materials -dry-run <manifest.json>`, then run the real import and perform the same paid-download smoke with a test account.
