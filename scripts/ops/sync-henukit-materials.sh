#!/bin/bash
# Prepare one manifest-approved materials snapshot for the privileged driver.
#
# This helper never mutates the public mirror. The caller owns publication and
# database reconciliation so a failed pipeline cannot leave half of the new
# snapshot live.
set -Eeuo pipefail

REPO_URL="${HENUKIT_MATERIALS_REPO_URL:-https://github.com/jry21223/HENU-Final-Review.git}"
REPO_REF="${HENUKIT_MATERIALS_REPO_REF:-main}"
EXPECTED_SHA="${HENUKIT_MATERIALS_EXPECTED_SHA:-}"
ROOT="${HENUKIT_MATERIALS_ROOT:-/opt/henukit-materials}"
CHECKOUT="$ROOT/repo"
STAGING_ROOT="${HENUKIT_MATERIALS_STAGING_ROOT:-$ROOT/.staging}"
STAGING_PUBLIC="$STAGING_ROOT/public"

die() {
  echo "sync-henukit-materials: $*" >&2
  exit 1
}

command -v git >/dev/null || die "git is required"
command -v python3 >/dev/null || die "python3 is required"
[[ "$REPO_REF" =~ ^[A-Za-z0-9._/-]+$ ]] || die "invalid repository ref"
if [[ -n "$EXPECTED_SHA" && ! "$EXPECTED_SHA" =~ ^[0-9a-f]{40}$ ]]; then
  die "expected SHA must be a full lowercase Git SHA"
fi

mkdir -p "$ROOT"

if [[ -d "$CHECKOUT/.git" ]]; then
  actual_remote="$(git -C "$CHECKOUT" remote get-url origin)"
  [[ "$actual_remote" == "$REPO_URL" ]] || die "checkout origin does not match configured repository"
  echo "Fetching $REPO_REF"
  git -C "$CHECKOUT" fetch --depth 1 origin "$REPO_REF"
  git -C "$CHECKOUT" reset --hard "origin/$REPO_REF"
  git -C "$CHECKOUT" clean -fdx
else
  echo "Cloning $REPO_URL"
  rm -rf "$CHECKOUT"
  git clone --depth 1 --branch "$REPO_REF" "$REPO_URL" "$CHECKOUT"
fi

[[ -f "$CHECKOUT/manifest.json" ]] || die "manifest.json missing from the checkout"

synced_sha="$(git -C "$CHECKOUT" rev-parse HEAD)"
if [[ -n "$EXPECTED_SHA" && "$synced_sha" != "$EXPECTED_SHA" ]]; then
  die "target branch is at $synced_sha, expected webhook SHA $EXPECTED_SHA"
fi
echo "Preparing reviewed assets from $synced_sha"

rm -rf "$STAGING_ROOT"
mkdir -p "$STAGING_PUBLIC"

python3 - "$CHECKOUT" "$STAGING_PUBLIC" <<'PY'
import hashlib
import json
import pathlib
import shutil
import sys

checkout = pathlib.Path(sys.argv[1]).resolve()
staging = pathlib.Path(sys.argv[2]).resolve()
manifest = json.loads((checkout / "manifest.json").read_text(encoding="utf-8"))

subjects = manifest.get("subjects")
if not isinstance(subjects, list):
    raise SystemExit("manifest.subjects must be an array")

copied = 0
skipped_pending = 0

for subject in subjects:
    assets = subject.get("assets")
    if not isinstance(assets, list):
        raise SystemExit(f"subject {subject.get('name', '<unnamed>')} has no assets array")
    for asset in assets:
        role = asset.get("role", "")
        if role.startswith("待复核"):
            skipped_pending += 1
            continue

        public_path = asset.get("publicPath", "")
        if not public_path:
            raise SystemExit("publishable manifest asset is missing publicPath")

        expected_sha = asset.get("sha256", "")
        expected_bytes = asset.get("bytes")
        if (
            not isinstance(expected_sha, str)
            or len(expected_sha) != 64
            or any(character not in "0123456789abcdef" for character in expected_sha)
        ):
            raise SystemExit(f"manifest asset has invalid sha256: {public_path}")
        if type(expected_bytes) is not int or expected_bytes < 0:
            raise SystemExit(f"manifest asset has invalid byte count: {public_path}")

        relative_path = pathlib.PurePosixPath(public_path)
        if relative_path.is_absolute() or ".." in relative_path.parts:
            raise SystemExit(f"manifest path escapes the checkout: {public_path}")
        source_path = checkout.joinpath(*relative_path.parts)
        cursor = checkout
        for part in relative_path.parts:
            cursor = cursor / part
            if cursor.is_symlink():
                raise SystemExit(f"manifest asset is missing or not a regular file: {public_path}")
        source = source_path.resolve()
        if not source.is_relative_to(checkout):
            raise SystemExit(f"manifest path escapes the checkout: {public_path}")
        if not source.is_file():
            raise SystemExit(f"manifest asset is missing or not a regular file: {public_path}")

        actual_bytes = source.stat().st_size
        if actual_bytes != expected_bytes:
            raise SystemExit(
                f"manifest byte count mismatch for {public_path}: "
                f"expected {expected_bytes}, got {actual_bytes}"
            )
        digest = hashlib.sha256()
        with source.open("rb") as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
        actual_sha = digest.hexdigest()
        if actual_sha != expected_sha:
            raise SystemExit(
                f"manifest sha256 mismatch for {public_path}: "
                f"expected {expected_sha}, got {actual_sha}"
            )

        target = (staging / public_path).resolve()
        if not target.is_relative_to(staging):
            raise SystemExit(f"manifest path escapes the mirror: {public_path}")
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        copied += 1

print(f"prepared {copied} assets, skipped {skipped_pending} pending review")
PY

find "$STAGING_PUBLIC" -type f -exec chmod 0444 {} +
find "$STAGING_PUBLIC" -type d -exec chmod 0755 {} +
if find "$STAGING_PUBLIC" -name '.*' -mindepth 1 | grep -q .; then
  die "refusing to publish: the mirror contains dotfiles"
fi

printf '%s\n' "$synced_sha" > "$STAGING_ROOT/SYNCED_SHA"
echo "Snapshot prepared at $STAGING_PUBLIC ($(find "$STAGING_PUBLIC" -type f | wc -l | tr -d ' ') files)"
