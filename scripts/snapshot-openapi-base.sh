#!/usr/bin/env bash
set -euo pipefail

base_sha="${1:?exact base commit SHA is required}"
destination="${2:?empty snapshot directory is required}"
if [[ ! "$base_sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo 'The OpenAPI base must be an exact commit SHA.' >&2
  exit 1
fi
if [[ ! -d "$destination" || -L "$destination" ]]; then
  echo 'The OpenAPI snapshot needs a real empty directory.' >&2
  exit 1
fi
shopt -s nullglob dotglob
entries=("$destination"/*)
if ((${#entries[@]})); then
  echo 'The OpenAPI snapshot directory must be empty.' >&2
  exit 1
fi
repo_root="$(git rev-parse --show-toplevel)"
git -C "$repo_root" archive "$base_sha" packages/api-contracts/openapi | tar -x -C "$destination"
