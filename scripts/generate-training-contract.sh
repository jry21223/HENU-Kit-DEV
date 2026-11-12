#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
owner_contract="$repo_root/packages/api-contracts/openapi/quizcraft-training.yaml"
portal_contract="$repo_root/packages/api-contracts/openapi/portal-training.yaml"

mkdir -p "$repo_root/products/quizcraft/go-service/internal/trainingcontract" \
  "$repo_root/services/portal-gateway/internal/trainingcontract"
cd "$repo_root/products/quizcraft/go-service"
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -generate types,skip-prune -package trainingcontract \
  -o internal/trainingcontract/types.gen.go "$owner_contract"
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -generate types,client -package trainingcontract \
  -o "$repo_root/services/portal-gateway/internal/trainingcontract/client.gen.go" "$owner_contract"

cd "$repo_root"
bundle_dir="$(mktemp -d "${TMPDIR:-/tmp}/portal-training.XXXXXX")"
bundled_contract="$bundle_dir/portal-training.json"
trap 'rm -f "$bundled_contract"; rmdir "$bundle_dir"' EXIT
npx --yes --package=@redocly/cli@2.39.0 redocly bundle \
  "$portal_contract" --output "$bundled_contract" --ext json
npx --yes openapi-typescript-codegen@0.29.0 \
  --input "$bundled_contract" \
  --output apps/portal/src/lib/api/generated/training \
  --client fetch --useOptions --useUnionTypes
find apps/portal/src/lib/api/generated/training -type f -name '*.ts' \
  -exec perl -0pi -e 's/\n\n\z/\n/' {} +
# These typed files need no runtime lint suppression. Keep the generator's
# suppression only in runtime helpers that actually require it.
find apps/portal/src/lib/api/generated/training/models \
  apps/portal/src/lib/api/generated/training/services -type f -name '*.ts' \
  -exec perl -0pi -e 's@^/\* eslint-disable \*/\n@@m' {} +
perl -0pi -e 's@^/\* eslint-disable \*/\n@@m' \
  apps/portal/src/lib/api/generated/training/index.ts \
  apps/portal/src/lib/api/generated/training/core/OpenAPI.ts
# The OpenAPI root server is '/', while the fetch generator concatenates BASE
# with an already absolute path. An empty BASE preserves same-origin /api URLs.
perl -0pi -e "s/BASE: '\/',/BASE: '',/" \
  apps/portal/src/lib/api/generated/training/core/OpenAPI.ts
