$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$composeFile = Join-Path $repositoryRoot "services\platform-core\compose.test.yml"
$migrationDirectory = Join-Path $repositoryRoot "services\platform-core\db\migrations"
$goImage = "mirror.gcr.io/library/golang:1.26.5-alpine"

function Invoke-SqlFile([string]$path) {
    Get-Content -Raw -LiteralPath $path |
        docker compose -f $composeFile exec -T postgres psql -v ON_ERROR_STOP=1 -U platform_core -d platform_core_test
    if ($LASTEXITCODE -ne 0) { throw "Migration failed: $path" }
}

docker compose -f $composeFile up -d --wait
if ($LASTEXITCODE -ne 0) { throw "Test dependencies did not become healthy" }

Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.down.sql")
Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.up.sql")

$present = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.authorization_codes') IS NOT NULL AND to_regclass('public.sessions') IS NOT NULL;"
if (($present | Out-String).Trim() -ne "t") { throw "Migration up did not create the identity tables" }

Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.down.sql")
$absent = docker compose -f $composeFile exec -T postgres psql -At -U platform_core -d platform_core_test -c "SELECT to_regclass('public.authorization_codes') IS NULL AND to_regclass('public.sessions') IS NULL;"
if (($absent | Out-String).Trim() -ne "t") { throw "Migration down did not remove the identity tables" }
Invoke-SqlFile (Join-Path $migrationDirectory "000001_identity.up.sql")

docker run --rm --network henukit-platform-core-test_default `
    -e PLATFORM_CORE_TEST_DATABASE_URL="postgres://platform_core:platform_core_test@postgres:5432/platform_core_test?sslmode=disable" `
    -e PLATFORM_CORE_TEST_REDIS_ADDR="redis:6379" `
    -v "${repositoryRoot}:/workspace" `
    -v henukit-go-mod:/go/pkg/mod `
    -v henukit-go-build:/root/.cache/go-build `
    -w /workspace/services/platform-core `
    $goImage sh -lc "export PATH=/usr/local/go/bin:`$PATH; gofmt -d . > /tmp/gofmt.diff; test ! -s /tmp/gofmt.diff; go vet ./...; go test -count=1 -v ./...; go build -o /tmp/platform-core ./cmd/server"
if ($LASTEXITCODE -ne 0) { throw "Platform Core verification failed" }
