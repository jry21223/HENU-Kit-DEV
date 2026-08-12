import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  renameSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const command = fileURLToPath(
  new URL("../rotate-henukit-release-signers.sh", import.meta.url),
);
const candidateSha = "b".repeat(40);
const source = readFileSync(command, "utf8");

test("rotation durably publishes intent, before-image, and terminal audit", () => {
  assert.match(
    source,
    /durable_move\(\)[\s\S]*sync -f "\$source"[\s\S]*mv -T "\$source" "\$target"[\s\S]*sync -f "\$target"[\s\S]*sync -f "\$directory"/,
  );
  assert.equal(
    (source.match(/durable_move /g) ?? []).length,
    3,
    "intent, before-image, and terminal audit must all use durable publication",
  );
  assert.match(source, /install -d[\s\S]*"\$rotations"[\s\S]*sync -f "\$state_root"/);
});

function fingerprint(publicKey) {
  return execFileSync("ssh-keygen", ["-l", "-E", "sha256", "-f", publicKey], {
    encoding: "utf8",
  }).trim().split(/\s+/)[1];
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "rotate-henukit-signers-"));
  const etc = join(root, "etc");
  const state = join(root, "state");
  mkdirSync(etc, { mode: 0o700 });
  mkdirSync(state, { mode: 0o700 });
  chmodSync(etc, 0o700);
  chmodSync(state, 0o700);

  const oldKey = join(root, "old");
  const newKey = join(root, "new");
  execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-f", oldKey]);
  execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-f", newKey]);
  chmodSync(`${newKey}.pub`, 0o400);

  const allowedSigners = join(etc, "release-signers");
  const oldPublic = readFileSync(`${oldKey}.pub`, "utf8").trim().split(/\s+/).slice(0, 2).join(" ");
  writeFileSync(allowedSigners, `henukit-release ${oldPublic}\n`, { mode: 0o440 });
  chmodSync(allowedSigners, 0o440);

  const args = [
    "--allowed-signers", allowedSigners,
    "--new-public-key", `${newKey}.pub`,
    "--principal", "henukit-release",
    "--retain-fingerprint", fingerprint(`${oldKey}.pub`),
    "--candidate-sha", candidateSha,
  ];
  return {
    root,
    state,
    allowedSigners,
    args,
    oldFingerprint: fingerprint(`${oldKey}.pub`),
    newFingerprint: fingerprint(`${newKey}.pub`),
    env: {
      ...process.env,
      HENUKIT_SIGNER_ROTATION_STATE_ROOT: state,
      HENUKIT_TRUST_ANCHOR: root,
    },
  };
}

test("preflight is read-only and execute atomically retains old and adds new signer", () => {
  const setup = fixture();
  const before = readFileSync(setup.allowedSigners, "utf8");

  const preflight = execFileSync(command, [...setup.args, "--preflight"], {
    encoding: "utf8",
    env: setup.env,
  });
  assert.match(preflight, /preflight passed/i);
  assert.equal(readFileSync(setup.allowedSigners, "utf8"), before);

  const output = execFileSync(command, [...setup.args, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });
  assert.match(output, new RegExp(setup.newFingerprint.replace(/[+]/g, "\\+")));
  const installed = readFileSync(setup.allowedSigners, "utf8");
  assert.equal(installed.trim().split("\n").length, 2);
  assert.match(installed, /henukit-release ssh-ed25519/);

  const audit = join(setup.state, "rotations", `${candidateSha}.rotated`);
  assert.equal(existsSync(audit), true);
  const record = readFileSync(audit, "utf8");
  assert.match(record, new RegExp(`old_fingerprint=${setup.oldFingerprint.replace(/[+]/g, "\\+")}`));
  assert.match(record, new RegExp(`new_fingerprint=${setup.newFingerprint.replace(/[+]/g, "\\+")}`));
});

test("an interrupted exact rotation resumes without adding a duplicate signer", () => {
  const setup = fixture();
  execFileSync(command, [...setup.args, "--execute"], { env: setup.env });
  execFileSync(command, [...setup.args, "--preflight"], { env: setup.env });
  const audit = join(setup.state, "rotations", `${candidateSha}.rotated`);
  renameSync(audit, `${audit}.rotating`);

  const output = execFileSync(command, [...setup.args, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.match(output, /already installed|completed/i);
  assert.equal(readFileSync(setup.allowedSigners, "utf8").trim().split("\n").length, 2);
  assert.equal(existsSync(audit), true);
  assert.equal(existsSync(`${audit}.rotating`), false);
});

test("a completed rotation refuses resume when its before-image is missing", () => {
  const setup = fixture();
  execFileSync(command, [...setup.args, "--execute"], { env: setup.env });
  const backup = join(
    setup.state,
    "rotations",
    `${candidateSha}.allowed-signers.before`,
  );
  unlinkSync(backup);

  const result = spawnSync(command, [...setup.args, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /before-image|backup/i);
});

test("execute locks before reading the transition and concurrent callers cannot overwrite terminal audit", async () => {
  assert.ok(
    source.indexOf("flock -n 9") < source.indexOf("while IFS= read -r line"),
    "the execute lock must precede allowed-signers transition reads",
  );
  const setup = fixture();
  const results = await Promise.all(
    Array.from({ length: 8 }, () => new Promise((resolve) => {
      const child = spawn(command, [...setup.args, "--execute"], {
        env: setup.env,
        encoding: "utf8",
      });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (chunk) => { stdout += chunk; });
      child.stderr.on("data", (chunk) => { stderr += chunk; });
      child.on("close", (status) => resolve({ status, stdout, stderr }));
    })),
  );

  assert.equal(results.filter(({ stdout }) => /rotation completed/.test(stdout)).length, 1);
  assert.ok(results.every(({ status, stdout, stderr }) =>
    status === 0 || /another signer rotation|already completed/.test(`${stdout}${stderr}`)));
  const audit = join(setup.state, "rotations", `${candidateSha}.rotated`);
  assert.equal(existsSync(audit), true);
});

test("preflight rejects a pre-installed signer whose rotation audit is missing", () => {
  const setup = fixture();
  execFileSync(command, [...setup.args, "--execute"], { env: setup.env });
  const audit = join(setup.state, "rotations", `${candidateSha}.rotated`);
  unlinkSync(audit);

  const result = spawnSync(command, [...setup.args, "--preflight"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /without a matching rotation audit/i);
});

test("rotation rejects a missing retained fingerprint without changing trust", () => {
  const setup = fixture();
  const before = readFileSync(setup.allowedSigners, "utf8");
  const args = [...setup.args];
  args[args.indexOf("--retain-fingerprint") + 1] = `SHA256:${"A".repeat(43)}`;

  const result = spawnSync(command, [...args, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /retained fingerprint/i);
  assert.equal(readFileSync(setup.allowedSigners, "utf8"), before);
});

test("rotation rejects writable or symlinked trust directories", () => {
  for (const kind of ["writable", "symlink"]) {
    const setup = fixture();
    if (kind === "writable") {
      chmodSync(setup.state, 0o777);
    } else {
      const realState = `${setup.state}-real`;
      renameSync(setup.state, realState);
      symlinkSync(realState, setup.state, "dir");
    }

    const result = spawnSync(command, [...setup.args, "--execute"], {
      encoding: "utf8",
      env: setup.env,
    });

    assert.notEqual(result.status, 0, kind);
    assert.match(result.stderr, /state|trust|directory/i, kind);
    assert.equal(readFileSync(setup.allowedSigners, "utf8").trim().split("\n").length, 1);
  }
});
