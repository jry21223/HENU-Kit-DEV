import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  renameSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const command = fileURLToPath(
  new URL("../adopt-henukit-degraded-baseline.sh", import.meta.url),
);
const baselineSha = "c".repeat(40);
const candidateSha = "a".repeat(40);

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "adopt-henukit-baseline-"));
  const releases = join(root, "releases");
  const release = join(releases, baselineSha);
  const state = join(root, "state");
  mkdirSync(join(release, "bin"), { recursive: true });
  mkdirSync(state);
  writeFileSync(join(release, "RELEASE_SHA"), `${baselineSha}\n`);
  writeFileSync(
    join(release, "docker-compose.henukit.release.yml"),
    "services:\n  portal:\n",
  );
  writeFileSync(
    join(release, "bin", "deploy-henukit-artifact.sh"),
    "#!/usr/bin/env bash\nexit 0\n",
    { mode: 0o555 },
  );
  chmodSync(join(release, "bin", "deploy-henukit-artifact.sh"), 0o555);
  const current = join(root, "current");
  symlinkSync(release, current, "dir");
  execFileSync("chown", ["-R", "1001:1001", release]);
  return {
    current,
    release,
    state,
    env: {
      ...process.env,
      HENUKIT_CURRENT_LINK: current,
      HENUKIT_RELEASE_ROOT: releases,
      HENUKIT_STATE_ROOT: state,
      HENUKIT_TRUST_ANCHOR: root,
    },
  };
}

test("explicit adoption makes one complete exact degraded release root-owned and auditable", () => {
  const setup = fixture();

  const output = execFileSync(
    command,
    [
      "--sha", baselineSha,
      "--candidate-sha", candidateSha,
      "--expected-owner-uid", "1001",
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /ownership adoption completed/i);
  assert.equal(lstatSync(setup.release).uid, 0);
  assert.equal(lstatSync(join(setup.release, "RELEASE_SHA")).uid, 0);
  const audit = readFileSync(
    join(setup.state, "degraded-recoveries", `${candidateSha}.baseline-adopted`),
    "utf8",
  );
  assert.match(audit, new RegExp(`candidate_sha=${candidateSha}`));
  assert.match(audit, new RegExp(`previous_sha=${baselineSha}`));
  assert.match(audit, /previous_owner_uid=1001/);
});

test("adoption rejects an unexpected owner before changing the release", () => {
  const setup = fixture();

  const result = spawnSync(
    command,
    [
      "--sha", baselineSha,
      "--candidate-sha", candidateSha,
      "--expected-owner-uid", "1002",
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /unexpected owner/i);
  assert.equal(lstatSync(setup.release).uid, 1001);
});

test("adoption resumes a root-owned release only from its immutable in-progress audit", () => {
  const setup = fixture();
  const args = [
    "--sha", baselineSha,
    "--candidate-sha", candidateSha,
    "--expected-owner-uid", "1001",
    "--execute",
  ];
  execFileSync(command, args, { encoding: "utf8", env: setup.env });
  const audit = join(
    setup.state,
    "degraded-recoveries",
    `${candidateSha}.baseline-adopted`,
  );
  const intent = `${audit}.adopting`;
  renameSync(audit, intent);

  const output = execFileSync(command, args, {
    encoding: "utf8",
    env: setup.env,
  });

  assert.match(output, /ownership adoption completed/i);
  assert.equal(lstatSync(setup.release).uid, 0);
  assert.match(readFileSync(audit, "utf8"), /previous_owner_uid=1001/);
});

test("an unaudited root-owned release is rejected instead of silently trusted", () => {
  const setup = fixture();
  execFileSync("chown", ["-R", "0:0", setup.release]);

  const result = spawnSync(
    command,
    [
      "--sha", baselineSha,
      "--candidate-sha", candidateSha,
      "--expected-owner-uid", "1001",
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /without the required adoption audit/i);
});

test("adoption rejects a writable audit directory before trusting its records", () => {
  const setup = fixture();
  const auditDirectory = join(setup.state, "degraded-recoveries");
  mkdirSync(auditDirectory, { mode: 0o770 });
  chmodSync(auditDirectory, 0o770);

  const result = spawnSync(
    command,
    [
      "--sha", baselineSha,
      "--candidate-sha", candidateSha,
      "--expected-owner-uid", "1001",
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /audit directory is not root-owned and private/i);
  assert.equal(lstatSync(setup.release).uid, 1001);
});

test("adoption never publishes an audit when retained bytes drift from its intent", () => {
  const setup = fixture();
  const args = [
    "--sha", baselineSha,
    "--candidate-sha", candidateSha,
    "--expected-owner-uid", "1001",
    "--execute",
  ];
  execFileSync(command, args, { encoding: "utf8", env: setup.env });
  const audit = join(setup.state, "degraded-recoveries", `${candidateSha}.baseline-adopted`);
  const intent = `${audit}.adopting`;
  renameSync(audit, intent);
  writeFileSync(join(setup.release, "docker-compose.henukit.release.yml"), "services: {}\n");

  const result = spawnSync(command, args, { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /content changed during ownership adoption/i);
  assert.equal(existsSync(audit), false);
});
