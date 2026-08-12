import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const builder = fileURLToPath(
  new URL("../build-henukit-release-local.sh", import.meta.url),
);
const source = readFileSync(builder, "utf8");
const releaseSha = "a".repeat(40);

test("the local builder is WSL-only and locks every artifact to the current clean main SHA", () => {
  execFileSync("bash", ["-n", builder]);
  assert.match(source, /builder must run on Linux; use the x86_64 WSL builder/);
  assert.match(source, /builder must run on WSL2; a generic Linux host is not an authorized builder/);
  assert.match(source, /git -C "\$repo_root" status --porcelain --untracked-files=all/);
  assert.match(source, /ls-remote --exit-code origin refs\/heads\/main/);
  assert.equal(
    (source.match(/remote_main_sha/g) ?? []).length,
    4,
    "the remote main head is checked before and after construction",
  );
  assert.equal(
    (source.match(/assert_source_snapshot/g) ?? []).length,
    5,
    "the exact clean source tree is rechecked before signing and publishing",
  );
  assert.match(source, /docker build[\s\S]*--platform linux\/amd64/);
  assert.match(source, /Docker server must be linux\/amd64/);
  assert.match(source, /--output-dir must not be group- or world-writable/);
  assert.match(source, /--handoff-group <deployment-reader-group>/);
  assert.match(source, /chgrp -R -- "\$handoff_group" "\$incoming"/);
  assert.match(source, /find "\$incoming" -type d -exec chmod 0550/);
  assert.match(source, /find "\$incoming" -type f -exec chmod 0440/);
  assert.match(source, /--signing-key must remain outside the artifact handoff tree/);
  assert.match(source, /ssh-keygen -Y sign/);
  assert.match(source, /public key[\s\S]*ssh-agent/i);
  assert.match(source, /ssh-add -l -E sha256/);
  assert.match(source, /signing_public_key/);
  assert.match(source, /"\$runtime_packager" --sha/);
  assert.match(source, /"\$verifier"[\s\S]*--allowed-signers/);
  assert.match(source, /refusing to overwrite existing artifact directory/);
});

test("the builder rejects a generic Linux amd64 host before it can read a signing key", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-not-wsl-"));
  const bin = join(root, "bin");
  mkdirSync(bin);
  const fakeUname = join(bin, "uname");
  writeFileSync(
    fakeUname,
    "#!/usr/bin/env bash\ncase \"${1:-}\" in\n  -s) printf Linux ;;\n  -m) printf x86_64 ;;\n  -r) printf 6.8.0-generic ;;\n  *) exit 64 ;;\nesac\n",
  );
  chmodSync(fakeUname, 0o755);

  const result = spawnSync(
    builder,
    [
      "--sha", releaseSha,
      "--output-dir", join(root, "output"),
      "--signing-key", join(root, "missing-key"),
      "--handoff-group", "henukit-release-deployers",
    ],
    {
      encoding: "utf8",
      env: { ...process.env, PATH: `${bin}:${process.env.PATH}` },
    },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /WSL2.*authorized builder/i);
  assert.doesNotMatch(result.stderr, /signing-key/i);
});

test("the builder rejects WSL1 before it can read a signing key", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-wsl1-"));
  const bin = join(root, "bin");
  mkdirSync(bin);
  const fakeUname = join(bin, "uname");
  writeFileSync(
    fakeUname,
    "#!/usr/bin/env bash\ncase \"${1:-}\" in\n  -s) printf Linux ;;\n  -m) printf x86_64 ;;\n  -r) printf 4.4.0-19041-Microsoft ;;\n  *) exit 64 ;;\nesac\n",
  );
  chmodSync(fakeUname, 0o755);

  const result = spawnSync(
    builder,
    [
      "--sha", releaseSha,
      "--output-dir", join(root, "output"),
      "--signing-key", join(root, "missing-key"),
      "--handoff-group", "henukit-release-deployers",
    ],
    {
      encoding: "utf8",
      env: { ...process.env, PATH: `${bin}:${process.env.PATH}` },
    },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /WSL2.*authorized builder/i);
  assert.doesNotMatch(result.stderr, /signing-key/i);
});
