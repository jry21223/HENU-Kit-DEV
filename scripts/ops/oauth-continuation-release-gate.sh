#!/usr/bin/env bash
set -Eeuo pipefail

program="oauth-continuation-release-gate"

usage() {
  cat >&2 <<'EOF'
usage:
  oauth-continuation-release-gate.sh run --sha <full-git-sha> --output <receipt>
  oauth-continuation-release-gate.sh verify --sha <full-git-sha> --receipt <receipt>

Run executes the cumulative Platform Core, Portal, and Console gate against an
exact clean checkout and writes a SHA-bound receipt. Verify accepts only the
canonical receipt for the requested source tree.
EOF
}

die() {
  printf '%s: %s\n' "$program" "$*" >&2
  exit 1
}

[[ $# -ge 1 ]] || { usage; exit 64; }
mode="$1"
shift
release_sha=""
output=""
receipt=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      release_sha="$2"
      shift 2
      ;;
    --output)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      output="$2"
      shift 2
      ;;
    --receipt)
      [[ $# -ge 2 ]] || { usage; exit 64; }
      receipt="$2"
      shift 2
      ;;
    *)
      usage
      exit 64
      ;;
  esac
done

[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || die "--sha must be a full lowercase Git SHA"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
source_tree="$(git -C "$repo_root" rev-parse "${release_sha}^{tree}" 2>/dev/null)" ||
  die "requested SHA is not available in this checkout"
canonical_receipt() {
  printf 'format=henukit-oauth-continuation-gate-v1\n'
  printf 'release_sha=%s\n' "$release_sha"
  printf 'source_tree=%s\n' "$source_tree"
  printf 'result=pass\n'
}

verify_receipt() {
  local candidate="$1"
  [[ -f "$candidate" && -r "$candidate" && ! -L "$candidate" ]] ||
    die "gate receipt must be a readable regular non-symlink file"
  [[ "$(cat "$candidate")" == "$(canonical_receipt)" ]] ||
    die "gate receipt does not match requested SHA and source tree"
}

verify_checkout() {
  [[ "$(git -C "$repo_root" rev-parse HEAD)" == "$release_sha" ]] ||
    die "checkout HEAD does not match requested SHA"
  [[ "$(git -C "$repo_root" rev-parse HEAD^{tree})" == "$source_tree" ]] ||
    die "checkout source tree does not match requested SHA"
  git -C "$repo_root" diff --quiet --no-ext-diff --ignore-submodules -- ||
    die "checkout tracked files do not match requested SHA"
  git -C "$repo_root" diff --cached --quiet --no-ext-diff --ignore-submodules -- ||
    die "checkout tracked files do not match requested SHA"
  local receipt_path
  receipt_path="$(cd "$(dirname "$receipt")" && pwd -P)/$(basename "$receipt")"
  local untracked
  while IFS= read -r -d '' untracked; do
    [[ "$repo_root/$untracked" == "$receipt_path" ]] ||
      die "unexpected untracked source file: $untracked"
  done < <(git -C "$repo_root" ls-files --others --exclude-standard -z)
}

case "$mode" in
  run)
    [[ -n "$output" && -z "$receipt" ]] || { usage; exit 64; }
    [[ ! -e "$output" && ! -L "$output" ]] || die "refusing to overwrite gate receipt"
    [[ -d "$(dirname "$output")" && ! -L "$(dirname "$output")" ]] ||
      die "gate receipt parent must be an existing non-symlink directory"
    command -v pnpm >/dev/null 2>&1 || die "pnpm is required to run the gate"
    [[ "$(git -C "$repo_root" rev-parse HEAD)" == "$release_sha" ]] ||
      die "checkout HEAD does not match requested SHA"
    [[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
      die "gate checkout must be clean, including untracked files"
    pnpm -C "$repo_root" run test:oauth-continuation
    [[ "$(git -C "$repo_root" rev-parse HEAD)" == "$release_sha" ]] ||
      die "checkout HEAD changed while running the gate"
    [[ "$(git -C "$repo_root" rev-parse HEAD^{tree})" == "$source_tree" ]] ||
      die "checkout source tree changed while running the gate"
    [[ -z "$(git -C "$repo_root" status --porcelain --untracked-files=all)" ]] ||
      die "gate changed the checkout"
    incoming="$(mktemp "$(dirname "$output")/.oauth-continuation-gate.XXXXXX")"
    cleanup() { rm -f -- "$incoming"; }
    trap cleanup EXIT
    canonical_receipt > "$incoming"
    chmod 0444 "$incoming"
    mv "$incoming" "$output"
    incoming=""
    verify_receipt "$output"
    ;;
  verify)
    [[ -n "$receipt" && -z "$output" ]] || { usage; exit 64; }
    verify_receipt "$receipt"
    verify_checkout
    ;;
  *)
    usage
    exit 64
    ;;
esac
