#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "working tree must be clean before subtree import" >&2
  exit 1
fi

import_subtree() {
  local prefix="$1"
  local repository="$2"
  local ref="$3"
  local label="$4"

  if [[ -e "$prefix" ]]; then
    echo "skip: $prefix already exists"
    return 0
  fi

  echo "importing $label from $repository@$ref into $prefix"
  git subtree add \
    --prefix="$prefix" \
    "$repository" \
    "$ref" \
    --squash

  local source_sha
  source_sha="$(git ls-remote "$repository" "refs/heads/$ref" | awk '{print $1}')"
  if [[ -z "$source_sha" ]]; then
    echo "unable to resolve source sha for $repository@$ref" >&2
    exit 1
  fi

  cat > "$prefix/HENUKIT_IMPORT.md" <<EOF
# HENU Kit Monorepo Import Metadata

- Source repository: \`$repository\`
- Source branch: \`$ref\`
- Source commit at import: \`$source_sha\`
- Import strategy: \`git subtree --squash\`
- Imported path: \`$prefix\`

This directory was imported during the HENU Kit monorepo migration. Keep the original license and attribution. Do not develop a competing source of truth in the old repository after the migration freeze.
EOF
}

import_subtree \
  "products/quizcraft" \
  "https://github.com/jry21223/quizcraft-cn.git" \
  "master" \
  "QuizCraft"

import_subtree \
  "archive/henukit-planning" \
  "https://github.com/jry21223/HENU-Kit.git" \
  "main" \
  "legacy HENU Kit planning repository"

if [[ -n "$(git status --porcelain)" ]]; then
  git add products/quizcraft/HENUKIT_IMPORT.md archive/henukit-planning/HENUKIT_IMPORT.md
  git commit -m "docs(monorepo): record imported repository sources"
fi

echo "subtree imports complete"
