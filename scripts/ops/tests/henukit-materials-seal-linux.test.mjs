import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const dockerAvailable = spawnSync("docker", ["version", "--format", "{{.Server.Version}}"], {
  encoding: "utf8",
}).status === 0;

if (process.env.CI === "true" && !dockerAvailable) {
  test("the CI runner provides Docker for root-owned materials seal verification", () => {
    assert.fail("Docker is required to verify root-owned materials sealing in CI");
  });
}

const integration = dockerAvailable ? test : test.skip;

function run(command, args, { cwd, allowFailure = false } = {}) {
  const result = spawnSync(command, args, { cwd, encoding: "utf8" });
  if (!allowFailure) {
    assert.equal(result.status, 0, `${command} ${args.join(" ")}\n${result.stdout}\n${result.stderr}`);
  }
  return result;
}

integration("Linux root sealing ignores candidate content and rejects unsafe root-owned output state", () => {
  const context = mkdtempSync(join(tmpdir(), "henukit-materials-seal-linux-"));
  let image = "";
  try {
    copyFileSync(
      join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-seal"),
      join(context, "henukit-materials-seal"),
    );
    copyFileSync(
      join(repositoryRoot, "scripts", "ops", "seal-henukit-materials.mjs"),
      join(context, "seal-henukit-materials.mjs"),
    );
    writeFileSync(
      join(context, "Dockerfile"),
      [
        "FROM node:22-alpine",
        "RUN apk add --no-cache bash git coreutils",
        "WORKDIR /fixture",
        "COPY henukit-materials-seal seal-henukit-materials.mjs probe.sh /fixture/",
        "RUN chown root:root /fixture/henukit-materials-seal /fixture/seal-henukit-materials.mjs /fixture/probe.sh && chmod 0755 /fixture/henukit-materials-seal /fixture/probe.sh && chmod 0600 /fixture/seal-henukit-materials.mjs",
        'ENTRYPOINT ["/fixture/probe.sh"]',
        "",
      ].join("\n"),
    );
    writeFileSync(
      join(context, "probe.sh"),
      `#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

readonly source=/work/source
readonly candidate_root=/var/lib/henukit-materials/candidates
readonly sealed_root=/var/lib/henukit-materials/sealed
readonly attempt=.attempt.Ab1Cd2Ef3G
readonly public_sentinel=/sentinels/public
readonly study_sentinel=/sentinels/study

adduser -D -H candidate >/dev/null
mkdir -p "$source/materials" "$candidate_root/$attempt" "$sealed_root" /etc/henukit-deploy /sentinels
printf 'public sentinel\\n' > "$public_sentinel"
printf 'study sentinel\\n' > "$study_sentinel"
printf 'sealed fixture material\\n' > "$source/materials/outline.pdf"
asset_bytes="$(wc -c < "$source/materials/outline.pdf" | tr -d '[:space:]')"
asset_sha="$(sha256sum "$source/materials/outline.pdf" | awk '{print $1}')"
printf '%s\\n' '{"version":1,"subjects":[{"name":"离散数学","assets":[{"role":"复习讲义","title":"离散数学_复习讲义_提纲.pdf","publicPath":"materials/outline.pdf","bytes":'"$asset_bytes"',"sha256":"'"$asset_sha"'"}]}]}' > "$source/manifest.json"
git init --quiet --initial-branch=main "$source"
git -C "$source" add .
git -C "$source" -c user.name=Fixture -c user.email=fixture@example.test commit --quiet -m fixture
accepted_sha="$(git -C "$source" rev-parse HEAD)"
manifest_sha="$(sha256sum "$source/manifest.json" | awk '{print $1}')"
release_id="$accepted_sha-\${manifest_sha:0:16}"

# This malformed/symlinked unprivileged tree is intentionally never named in
# root configuration. A successful seal proves no candidate marker or bytes
# were opened by the root consumer.
ln -s /does-not-exist "$candidate_root/$attempt/candidate"
chown -R candidate:candidate "$candidate_root"

chown root:root "$sealed_root"
chmod 0700 "$sealed_root"
cat > /etc/henukit-deploy/materials-seal.env <<EOF
HENUKIT_MATERIALS_SEALED_ROOT=$sealed_root
HENUKIT_MATERIALS_SOURCE_REPOSITORY=$source
HENUKIT_MATERIALS_SOURCE_REF=refs/heads/main
HENUKIT_MATERIALS_SOURCE_SHA=$accepted_sha
EOF
chown root:root /etc/henukit-deploy/materials-seal.env
chmod 0600 /etc/henukit-deploy/materials-seal.env

if su candidate -s /bin/bash -c '/fixture/henukit-materials-seal --attempt .attempt.Ab1Cd2Ef3G' > /tmp/unprivileged.out 2>&1; then
  echo 'unprivileged seal unexpectedly succeeded' >&2
  exit 1
fi
grep -F 'must run as root' /tmp/unprivileged.out >/dev/null

chmod 0660 /etc/henukit-deploy/materials-seal.env
if /fixture/henukit-materials-seal --attempt "$attempt" > /tmp/writable-config.out 2>&1; then
  echo 'group-writable config unexpectedly succeeded' >&2
  exit 1
fi
grep -F 'configuration must not be writable by group or other' /tmp/writable-config.out >/dev/null
chmod 0600 /etc/henukit-deploy/materials-seal.env

unsafe_parent=/opt/henukit-materials-candidate-owned-parent
unsafe_sealed_root="$unsafe_parent/sealed"
mkdir -p "$unsafe_sealed_root"
chown candidate:candidate "$unsafe_parent"
chmod 0755 "$unsafe_parent"
chown root:root "$unsafe_sealed_root"
chmod 0700 "$unsafe_sealed_root"
cat > /etc/henukit-deploy/materials-seal.env <<EOF
HENUKIT_MATERIALS_SEALED_ROOT=$unsafe_sealed_root
HENUKIT_MATERIALS_SOURCE_REPOSITORY=$source
HENUKIT_MATERIALS_SOURCE_REF=refs/heads/main
HENUKIT_MATERIALS_SOURCE_SHA=$accepted_sha
EOF
chown root:root /etc/henukit-deploy/materials-seal.env
chmod 0600 /etc/henukit-deploy/materials-seal.env
if /fixture/henukit-materials-seal --attempt "$attempt" > /tmp/attacker-parent.out 2>&1; then
  echo 'attacker-owned sealed-root ancestor unexpectedly succeeded' >&2
  exit 1
fi
grep -F 'sealed root has an ancestor not owned by the fixed root account' /tmp/attacker-parent.out >/dev/null
test ! -e "$unsafe_sealed_root/$release_id"
cat > /etc/henukit-deploy/materials-seal.env <<EOF
HENUKIT_MATERIALS_SEALED_ROOT=$sealed_root
HENUKIT_MATERIALS_SOURCE_REPOSITORY=$source
HENUKIT_MATERIALS_SOURCE_REF=refs/heads/main
HENUKIT_MATERIALS_SOURCE_SHA=$accepted_sha
EOF
chown root:root /etc/henukit-deploy/materials-seal.env
chmod 0600 /etc/henukit-deploy/materials-seal.env

chmod 0770 "$sealed_root"
if /fixture/henukit-materials-seal --attempt "$attempt" > /tmp/writable-root.out 2>&1; then
  echo 'group-writable sealed root unexpectedly succeeded' >&2
  exit 1
fi
grep -F 'sealed root must not be writable by group or other' /tmp/writable-root.out >/dev/null
chmod 0700 "$sealed_root"

mkdir "$sealed_root/$release_id"
printf 'malicious preseed\\n' > "$sealed_root/$release_id/sentinel"
chown -R candidate:candidate "$sealed_root/$release_id"
if /fixture/henukit-materials-seal --attempt "$attempt" > /tmp/foreign-release.out 2>&1; then
  echo 'foreign pre-seeded release unexpectedly succeeded' >&2
  exit 1
fi
grep -F 'existing sealed release must be owned by the fixed root account' /tmp/foreign-release.out >/dev/null
test "$(cat "$sealed_root/$release_id/sentinel")" = 'malicious preseed'
test "$(stat -c '%u:%g' "$sealed_root/$release_id")" != '0:0'
test "$(cat "$public_sentinel")" = 'public sentinel'
test "$(cat "$study_sentinel")" = 'study sentinel'
rm -rf "$sealed_root/$release_id"

ln -s /does-not-exist "$sealed_root/$release_id"
if /fixture/henukit-materials-seal --attempt "$attempt" > /tmp/dangling-release.out 2>&1; then
  echo 'dangling pre-seeded release symlink unexpectedly succeeded' >&2
  exit 1
fi
grep -F 'existing sealed release must not be a symbolic link' /tmp/dangling-release.out >/dev/null
test -L "$sealed_root/$release_id"
test "$(cat "$public_sentinel")" = 'public sentinel'
test "$(cat "$study_sentinel")" = 'study sentinel'
rm "$sealed_root/$release_id"

/fixture/henukit-materials-seal --attempt "$attempt" > /tmp/seal.out &
seal_pid=$!
while kill -0 "$seal_pid" 2>/dev/null; do
  for entry in "$sealed_root"/*; do
    [[ -e "$entry" ]] || continue
    [[ "$(basename "$entry")" != .* ]] || continue
    test -f "$entry/sealed-release.json"
    test -f "$entry/inventory.json"
    test -f "$entry/public/materials/outline.pdf"
  done
done
wait "$seal_pid"
actual_attempt="$(node -e 'process.stdout.write(JSON.parse(require("node:fs").readFileSync("/tmp/seal.out", "utf8")).attempt_locator)')"
actual_release="$(node -e 'process.stdout.write(JSON.parse(require("node:fs").readFileSync("/tmp/seal.out", "utf8")).release_id)')"
test "$actual_attempt" = "$attempt"
test "$actual_release" = "$release_id"
release="$sealed_root/$release_id"
test "$(stat -c '%u:%g:%a' "$release")" = '0:0:700'
test "$(stat -c '%u:%g:%a' "$release/sealed-release.json")" = '0:0:400'
test "$(stat -c '%u:%g:%a' "$release/inventory.json")" = '0:0:400'
test "$(stat -c '%u:%g:%a' "$release/public/materials/outline.pdf")" = '0:0:400'
test ! -e "$release/slides"
audit="$sealed_root/.audit/$release_id/\${attempt#.attempt.}.json"
test "$(stat -c '%u:%g:%a' "$sealed_root/.audit")" = '0:0:700'
test "$(stat -c '%u:%g:%a' "$(dirname "$audit")")" = '0:0:700'
test "$(stat -c '%u:%g:%a' "$audit")" = '0:0:400'
test "$(node -e 'process.stdout.write(JSON.parse(require("node:fs").readFileSync(process.argv[1], "utf8")).attempt_locator)' "$audit")" = "$attempt"
test "$(sha256sum "$release/public/materials/outline.pdf" | awk '{print $1}')" = "$asset_sha"
test "$(cat "$public_sentinel")" = 'public sentinel'
test "$(cat "$study_sentinel")" = 'study sentinel'
if find "$sealed_root" -mindepth 1 -maxdepth 1 -name '.incoming.*' -print -quit | grep -q .; then
  echo 'partial incoming seal directory remained visible' >&2
  exit 1
fi
`,
      { mode: 0o700 },
    );
    image = run("docker", ["build", "--quiet", context]).stdout.trim();
    assert.match(image, /^sha256:[a-f0-9]{64}$/);
    run("docker", ["run", "--rm", "--network", "none", image]);
  } finally {
    if (image) run("docker", ["image", "rm", "--force", image], { allowFailure: true });
    rmSync(context, { recursive: true, force: true });
  }
});
