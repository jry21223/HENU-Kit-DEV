$ErrorActionPreference = "Stop"

$servicePath = Split-Path -Parent $PSScriptRoot
$composePath = Join-Path $servicePath "compose.test.yml"
$repoPath = (Resolve-Path (Join-Path $servicePath "../..")).Path

docker compose -f $composePath up -d --wait
if ($LASTEXITCODE -ne 0) { throw "Console Gateway Redis failed to start" }

docker run --rm `
  --network console-gateway_default `
  -e CONSOLE_GATEWAY_TEST_REDIS_ADDR="redis:6379" `
  -e GOSUMDB=off `
  -v "${repoPath}:/workspace" `
  -v henukit-go-mod:/go/pkg/mod `
  -v henukit-go-build:/root/.cache/go-build `
  -w /workspace/services/console-gateway `
  mirror.gcr.io/library/golang:1.26.5 `
  sh -lc 'export PATH=/usr/local/go/bin:$PATH; go test ./...'
if ($LASTEXITCODE -ne 0) { throw "Console Gateway verification failed" }
