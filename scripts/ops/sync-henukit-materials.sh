#!/usr/bin/env bash
# Mirror reviewed course materials from the public HENU-Final-Review repository
# into the directory nginx serves.
#
# The git checkout and the served root are deliberately separate directories.
# Serving the checkout itself would expose .git — the whole repository history —
# plus the repository's tooling, so only files named by manifest.json are copied
# across, and only those whose role is not a 待复核 (pending review) one. The
# repository's own PUBLICATION_POLICY.md keeps unreviewed material out of the
# formal directories until a maintainer confirms its provenance; this mirror
# holds that same line.
#
# Safe to re-run: the mirror is rebuilt from the manifest every time, so files
# removed or renamed upstream disappear here too.
set -Eeuo pipefail

REPO_URL="${HENUKIT_MATERIALS_REPO_URL:-https://github.com/jry21223/HENU-Final-Review.git}"
REPO_REF="${HENUKIT_MATERIALS_REPO_REF:-main}"
ROOT="${HENUKIT_MATERIALS_ROOT:-/opt/henukit-materials}"
CHECKOUT="$ROOT/repo"
PUBLIC="$ROOT/public"
STAGING="$ROOT/.staging"

die() {
  echo "sync-henukit-materials: $*" >&2
  exit 1
}

command -v git >/dev/null || die "git is required"
command -v python3 >/dev/null || die "python3 is required"

mkdir -p "$ROOT"

if [[ -d "$CHECKOUT/.git" ]]; then
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
echo "Mirroring reviewed assets from $synced_sha"

rm -rf "$STAGING"
mkdir -p "$STAGING"

# The manifest is the only source of truth for what is published. Copying the
# tree wholesale would also publish repository tooling and unreviewed material.
python3 - "$CHECKOUT" "$STAGING" <<'PY'
import json
import pathlib
import shutil
import sys

checkout = pathlib.Path(sys.argv[1]).resolve()
staging = pathlib.Path(sys.argv[2]).resolve()
manifest = json.loads((checkout / "manifest.json").read_text(encoding="utf-8"))

copied = 0
skipped_pending = 0
missing = []

for subject in manifest.get("subjects", []):
    for asset in subject.get("assets", []):
        role = asset.get("role", "")
        if role.startswith("待复核"):
            skipped_pending += 1
            continue

        public_path = asset.get("publicPath", "")
        if not public_path:
            continue

        source = (checkout / public_path).resolve()
        # A manifest entry must not be able to reach outside the checkout.
        if not source.is_relative_to(checkout):
            raise SystemExit(f"manifest path escapes the checkout: {public_path}")
        if not source.is_file():
            missing.append(public_path)
            continue

        target = (staging / public_path).resolve()
        if not target.is_relative_to(staging):
            raise SystemExit(f"manifest path escapes the mirror: {public_path}")

        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        copied += 1

print(f"mirrored {copied} assets, skipped {skipped_pending} pending review")
if missing:
    print(f"WARNING: {len(missing)} manifest entries have no file:")
    for path in missing[:20]:
        print(f"  {path}")
    if len(missing) > 20:
        print(f"  ... and {len(missing) - 20} more")
PY

# Nothing under the mirror is executable, and nothing there is a dotfile.
find "$STAGING" -type f -exec chmod 0444 {} +
find "$STAGING" -type d -exec chmod 0755 {} +
if find "$STAGING" -name '.*' -mindepth 1 | grep -q .; then
  die "refusing to publish: the mirror contains dotfiles"
fi

# Swap atomically so a reader never sees a half-written mirror.
rm -rf "$PUBLIC.previous"
if [[ -d "$PUBLIC" ]]; then
  mv "$PUBLIC" "$PUBLIC.previous"
fi
mv "$STAGING" "$PUBLIC"
rm -rf "$PUBLIC.previous"

printf '%s\n' "$synced_sha" > "$ROOT/SYNCED_SHA"
echo "Mirror published at $PUBLIC ($(find "$PUBLIC" -type f | wc -l | tr -d ' ') files, $(du -sh "$PUBLIC" | cut -f1))"

# Files on disk are only half the job: the Library lists what the Study
# catalogue knows about, so a mirror without an index shows an empty page.
# Both halves come from the same manifest, so the catalogue is reproducible.
indexer="$(dirname "$0")/index-henukit-materials.mjs"
if [[ -z "${STUDY_DATABASE_URL:-}" ]]; then
  echo "STUDY_DATABASE_URL is not set; skipping catalogue indexing" >&2
elif [[ ! -f "$indexer" ]]; then
  echo "indexer missing at $indexer; skipping catalogue indexing" >&2
else
  echo "Indexing the catalogue"
  catalogue_sql="$(mktemp)"
  trap 'rm -f "$catalogue_sql"' EXIT
  HENUKIT_MATERIALS_ROOT="$ROOT" node "$indexer" > "$catalogue_sql"
  psql "$STUDY_DATABASE_URL" -v ON_ERROR_STOP=1 -q -f "$catalogue_sql"
  echo "Catalogue indexed"
fi
