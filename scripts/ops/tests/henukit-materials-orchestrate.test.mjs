import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const templatePath = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-orchestrate");
const acceptedSHA = "a".repeat(40);
const releaseID = `${acceptedSHA}-${"b".repeat(16)}`;
const receiptSHA = "c".repeat(64);

function shellLiteral(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function executable(path, body) {
  writeFileSync(path, body, { mode: 0o700 });
  chmodSync(path, 0o700);
}

function createFixture() {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-orchestrate-"));
  const state = join(root, "state");
  const sealed = join(root, "sealed");
  mkdirSync(state, { mode: 0o700 });
  mkdirSync(join(state, "candidates"), { mode: 0o700 });
  mkdirSync(sealed, { mode: 0o700 });
  const webhookConfig = join(root, "materials-webhook.env");
  const sealConfig = join(root, "materials-seal.env");
  writeFileSync(webhookConfig, [
    "HENUKIT_WEBHOOK_REPOSITORY=jry21223/HENU-Final-Review",
    "HENUKIT_WEBHOOK_BRANCH=main",
    `HENUKIT_WEBHOOK_STATE_DIR=${state}`,
    "HENUKIT_MATERIALS_SOURCE_REPOSITORY=https://example.test/HENU-Final-Review.git",
    `HENUKIT_MATERIALS_CANDIDATE_ROOT=${join(state, "candidates")}`,
    "",
  ].join("\n"), { mode: 0o600 });
  writeFileSync(sealConfig, [
    `HENUKIT_MATERIALS_SEALED_ROOT=${sealed}`,
    "HENUKIT_MATERIALS_SOURCE_REPOSITORY=https://example.test/HENU-Final-Review.git",
    "HENUKIT_MATERIALS_SOURCE_REF=refs/heads/main",
    "",
  ].join("\n"), { mode: 0o600 });

  const calls = join(root, "calls");
  const prepare = join(root, "henukit-materials-prepare");
  const seal = join(root, "henukit-materials-seal");
  const activate = join(root, "henukit-materials-activate");
  const runuser = join(root, "runuser");
  executable(prepare, `#!/bin/sh\nprintf 'prepare %s\\n' "$*" >> ${shellLiteral(calls)}\nprintf '{"reviewed_assets":1}\\nattempt_locator=.attempt.Ab1Cd2Ef3G\\n'\n`);
  executable(seal, `#!/bin/sh\nprintf 'seal %s\\n' "$*" >> ${shellLiteral(calls)}\nprintf '{"attempt_locator":".attempt.Ab1Cd2Ef3G","release_id":"${releaseID}","receipt_sha256":"${receiptSHA}"}\\n'\n`);
  executable(activate, `#!/bin/sh\nprintf 'activate %s\\n' "$*" >> ${shellLiteral(calls)}\n`);
  executable(runuser, "#!/bin/sh\nshift 3\nexec \"$@\"\n");

  let template = readFileSync(templatePath, "utf8");
  template = template.replace('readonly webhook_config="/etc/henukit-deploy/materials-webhook.env"', `readonly webhook_config=${shellLiteral(webhookConfig)}`);
  template = template.replace('readonly seal_config="/etc/henukit-deploy/materials-seal.env"', `readonly seal_config=${shellLiteral(sealConfig)}`);
  template = template.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  template = template.replace('readonly runtime_owner="0"', `readonly runtime_owner="${process.getuid()}"`);
  template = template.replace('readonly prepare_wrapper="/usr/local/libexec/henukit/henukit-materials-prepare"', `readonly prepare_wrapper=${shellLiteral(prepare)}`);
  template = template.replace('readonly seal_wrapper="/usr/local/libexec/henukit/henukit-materials-seal"', `readonly seal_wrapper=${shellLiteral(seal)}`);
  template = template.replace('readonly activate_wrapper="/usr/local/libexec/henukit/henukit-materials-activate"', `readonly activate_wrapper=${shellLiteral(activate)}`);
  template = template.replace('readonly runuser_bin="/usr/sbin/runuser"', `readonly runuser_bin=${shellLiteral(runuser)}`);
  const wrapper = join(root, "henukit-materials-orchestrate");
  executable(wrapper, template);
  return { root, wrapper, calls };
}

function invoke(fixture, sha = acceptedSHA) {
  return spawnSync(fixture.wrapper, [
    "--sha", sha,
    "--delivery", "delivery-1",
    "--repository", "jry21223/HENU-Final-Review",
    "--ref", "refs/heads/main",
  ], {
    encoding: "utf8",
    env: { ...process.env, PATH: fixture.root, BASH_ENV: join(fixture.root, "attacker") },
  });
}

test("root materials orchestration binds one accepted event through prepare, seal, and activate", () => {
  const fixture = createFixture();
  try {
    const result = invoke(fixture);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(readFileSync(fixture.calls, "utf8"), [
      `prepare --sha ${acceptedSHA} --delivery delivery-1 --repository jry21223/HENU-Final-Review --ref refs/heads/main`,
      `seal --attempt .attempt.Ab1Cd2Ef3G --sha ${acceptedSHA}`,
      `activate --release-id ${releaseID} --receipt-sha256 ${receiptSHA}`,
      "",
    ].join("\n"));
  } finally {
    rmSync(fixture.root, { recursive: true, force: true });
  }
});

test("root materials orchestration accepts only a valid SHA from the verified event boundary", () => {
  const fixture = createFixture();
  try {
    const result = invoke(fixture, "not-a-sha");
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /event SHA is invalid/);
    assert.throws(() => readFileSync(fixture.calls, "utf8"), /ENOENT/);
  } finally {
    rmSync(fixture.root, { recursive: true, force: true });
  }
});

test("materials installer replaces retired sync scripts with the fixed orchestration chain", () => {
  const installer = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "install.sh"), "utf8");
  for (const required of [
    "henukit-materials-orchestrate",
    "henukit-materials-prepare",
    "henukit-materials-seal",
    "henukit-materials-activate",
    "prepare-henukit-materials.mjs",
    "seal-henukit-materials.mjs",
    "activate-henukit-materials.mjs",
    "materials-seal.env",
    "materials-activate.env",
  ]) assert.match(installer, new RegExp(required.replaceAll(".", "\\.")), required);
  assert.match(installer, /python3 -c 'import pptx'/);
  assert.match(installer, /command -v soffice/);
  assert.match(installer, /command -v libreoffice/);
  assert.doesNotMatch(installer, /scripts\/ops\/(?:sync-henukit-materials\.sh|henukit-materials-sync\.sh)/);
  assert.match(installer, /if \(\( enable_materials_sync \)\); then\n  for command in node python3 psql;/);
  assert.match(installer, /systemctl disable --now henukit-materials-webhook\.path/);
});
