$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$composeFile = Join-Path $repositoryRoot "services\platform-core\compose.test.yml"
$migrationDirectory = Join-Path $repositoryRoot "services\platform-core\db\migrations"
$goImage = "mirror.gcr.io/library/golang:1.26.6-alpine"

function Invoke-SqlFile([string]$path) {
    Get-Content -Raw -LiteralPath $path |
        docker compose -f $composeFile exec -T postgres psql -v ON_ERROR_STOP=1 -U platform_core -d platform_core_test
    if ($LASTEXITCODE -ne 0) { throw "Migration failed: $path" }
}

docker compose -f $composeFile up -d --wait
if ($LASTEXITCODE -ne 0) { throw "Test dependencies did not become healthy" }

Invoke-SqlFile (Join-Path $migrationDirectory "000005_console_access.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000004_operations_inbox.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000003_verification_mail.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000002_authorization.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.up.sql")

$legacyReady = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.sessions') IS NOT NULL AND to_regclass('public.permission_codes') IS NULL AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'users' AND column_name = 'authorization_revision');"
if (($legacyReady | Out-String).Trim() -ne "t") { throw "Migration 000001 did not reproduce the supported HC-05 schema" }
docker compose -f $composeFile exec -T postgres psql -v ON_ERROR_STOP=1 -U platform_core -d platform_core_test -c "INSERT INTO users (email_verified, status) VALUES (true, 'active');"
if ($LASTEXITCODE -ne 0) { throw "Could not seed a pre-HC-06 user" }

Invoke-SqlFile (Join-Path $migrationDirectory "000002_authorization.up.sql")
$upgraded = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.permission_codes') IS NOT NULL AND (SELECT authorization_revision = 1 FROM users LIMIT 1);"
if (($upgraded | Out-String).Trim() -ne "t") { throw "Migration 000002 did not upgrade the supported HC-05 schema and backfill users" }

$preMailReady = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.verification_codes') IS NULL AND to_regclass('public.mail_outbox') IS NULL;"
if (($preMailReady | Out-String).Trim() -ne "t") { throw "Migration 000002 did not reproduce the supported HC-06 schema" }
Invoke-SqlFile (Join-Path $migrationDirectory "000003_verification_mail.up.sql")
$mailUpgraded = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.verification_codes') IS NOT NULL AND to_regclass('public.mail_outbox') IS NOT NULL AND (SELECT count(*) = 1 FROM users);"
if (($mailUpgraded | Out-String).Trim() -ne "t") { throw "Migration 000003 did not upgrade the supported HC-06 schema without data loss" }

$preInboxReady = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.operations_inbox_items') IS NULL;"
if (($preInboxReady | Out-String).Trim() -ne "t") { throw "Migration 000003 did not reproduce the supported HC-07 schema" }
Invoke-SqlFile (Join-Path $migrationDirectory "000004_operations_inbox.up.sql")
$inboxUpgraded = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.operations_inbox_items') IS NOT NULL AND to_regclass('public.operations_inbox_audit_events') IS NOT NULL AND (SELECT count(*) = 1 FROM users);"
if (($inboxUpgraded | Out-String).Trim() -ne "t") { throw "Migration 000004 did not upgrade the supported HC-07 schema without data loss" }
Invoke-SqlFile (Join-Path $migrationDirectory "000004_operations_inbox.up.sql")
$repeatedInboxUp = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT count(*) = 2 FROM permission_codes WHERE code IN ('platform.operations_inbox.read', 'platform.operations_inbox.write');"
if (($repeatedInboxUp | Out-String).Trim() -ne "t") { throw "Migration 000004 repeated Up was not idempotent" }

Invoke-SqlFile (Join-Path $migrationDirectory "000005_console_access.up.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000005_console_access.up.sql")
$consolePermissionReady = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT count(*) = 1 FROM permission_codes WHERE code = 'console.overview.read' AND status = 'active';"
if (($consolePermissionReady | Out-String).Trim() -ne "t") { throw "Migration 000005 repeated Up did not provision the Console permission" }

Invoke-SqlFile (Join-Path $migrationDirectory "000005_console_access.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000004_operations_inbox.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000003_verification_mail.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000002_authorization.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.down.sql")
$absent = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.authorization_codes') IS NULL AND to_regclass('public.sessions') IS NULL AND to_regclass('public.permission_codes') IS NULL;"
if (($absent | Out-String).Trim() -ne "t") { throw "Migration down did not remove all versioned tables" }
Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.up.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000002_authorization.up.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000003_verification_mail.up.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000004_operations_inbox.up.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000005_console_access.up.sql")

docker run --rm --network henukit-platform-core-test_default `
    -e PLATFORM_CORE_TEST_DATABASE_URL="postgres://platform_core:platform_core_test@postgres:5432/platform_core_test?sslmode=disable" `
    -e PLATFORM_CORE_TEST_REDIS_ADDR="redis:6379" `
    -v "${repositoryRoot}:/workspace" `
    -v henukit-go-mod:/go/pkg/mod `
    -v henukit-go-build:/root/.cache/go-build `
    -w /workspace/services/platform-core `
    $goImage sh -lc "set -eu; export PATH=/usr/local/go/bin:`$PATH; gofmt -d platformcore.go cmd/contractgen internal/httpapi internal/operationsinbox internal/store internal/contract tests/operations_inbox_test.go > /tmp/gofmt.diff; test ! -s /tmp/gofmt.diff; go vet ./...; go test -count=1 -v ./...; go build -o /tmp/platform-core ./cmd/server; go build -o /tmp/platform-mail-worker ./cmd/mail-worker"
if ($LASTEXITCODE -ne 0) { throw "Platform Core verification failed" }
