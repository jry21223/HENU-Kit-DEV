#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
service_dir="$(cd "$script_dir/.." && pwd)"
repo_root="$(cd "$service_dir/../../.." && pwd)"
contract="$repo_root/packages/api-contracts/openapi/quizcraft.yaml"
bundle_dir="$(mktemp -d "${TMPDIR:-/tmp}/quizcraft-contract.XXXXXX")"
bundled_contract="$bundle_dir/quizcraft.json"
trap 'rm -f "$bundled_contract"; rmdir "$bundle_dir"' EXIT
npx --yes --package=@redocly/cli@2.39.0 redocly bundle \
  "$contract" --output "$bundled_contract" --ext json

cd "$service_dir"
go run ./cmd/contractgen
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -generate types \
  -package contract \
  -o internal/contract/types.gen.go \
  "$bundled_contract"
gofmt -w internal/contract/types.gen.go

cd "$repo_root"
npx --yes openapi-typescript-codegen@0.29.0 \
  --input "$bundled_contract" \
  --output products/quizcraft/web-app/src/generated/quizcraft-api \
  --client fetch \
  --useOptions \
  --useUnionTypes
find products/quizcraft/web-app/src/generated/quizcraft-api -type f -name '*.ts' -exec perl -0pi -e 's/\n\n\z/\n/' {} +
