import assert from "node:assert/strict";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const templatePath = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-publish-release-oss");

function shellLiteral(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function writeConfig(path, root, extra = "") {
  const sealedRoot = join(root, "sealed");
  const auditRoot = join(root, "audit");
  mkdirSync(sealedRoot, { recursive: true, mode: 0o700 });
  mkdirSync(auditRoot, { recursive: true, mode: 0o700 });
  writeFileSync(path, [
    `HENUKIT_MATERIALS_SEALED_ROOT=${sealedRoot}`,
    `HENUKIT_MATERIALS_OSS_AUDIT_ROOT=${auditRoot}`,
    "HENUKIT_MATERIALS_OSS_BUCKET=henukit",
    "HENUKIT_MATERIALS_OSS_REGION=cn-beijing",
    "HENUKIT_MATERIALS_OSS_ENDPOINT=oss-cn-beijing-internal.aliyuncs.com",
    "HENUKIT_MATERIALS_OSS_RAM_ROLE=henukit-materials-oss-publisher",
    extra,
    "",
  ].join("\n"), { mode: 0o600 });
}

function stageWrapper(root, configPath, outputPath) {
  let template = readFileSync(templatePath, "utf8");
  template = template.replace(
    'readonly config_path="/etc/henukit-deploy/materials-oss.env"',
    `readonly config_path=${shellLiteral(configPath)}`,
  );
  template = template.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  template = template.replace('readonly runtime_owner="0"', `readonly runtime_owner="${process.getuid()}"`);
  template = template.replace('readonly trusted_ancestor="/"', `readonly trusted_ancestor=${shellLiteral(root)}`);
  const staged = join(root, "henukit-materials-publish-release-oss");
  writeFileSync(staged, template, { mode: 0o700 });
  chmodSync(staged, 0o700);
  writeFileSync(join(root, "materials-oss-release"), [
    "#!/bin/sh",
    `printf '%s' "$#" > ${shellLiteral(outputPath)}.argc`,
    `printf '%s\n' "$@" > ${shellLiteral(outputPath)}.argv`,
    `env > ${shellLiteral(outputPath)}.env`,
  ].join("\n"), { mode: 0o700 });
  chmodSync(join(root, "materials-oss-release"), 0o700);
  return staged;
}

test("complete OSS release wrapper fixes authority and exposes only sealed release identity", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-publish-release-oss-"));
  try {
    const configPath = join(root, "materials-oss.env");
    const outputPath = join(root, "invocation");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);
    const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
    const receiptSHA256 = "c".repeat(64);

    const result = spawnSync(wrapper, ["--release-id", releaseID, "--receipt-sha256", receiptSHA256], {
      encoding: "utf8",
      env: {
        ...process.env,
        ALIBABA_CLOUD_ACCESS_KEY_ID: "caller-access-key",
        ALIBABA_CLOUD_ACCESS_KEY_SECRET: "caller-secret",
        ALIBABA_CLOUD_SECURITY_TOKEN: "caller-token",
        HTTPS_PROXY: "http://caller-proxy.invalid",
        HENUKIT_MATERIALS_OSS_BUCKET: "attacker-bucket",
        HENUKIT_MATERIALS_OSS_ENDPOINT: "attacker.example",
      },
    });

    assert.equal(result.status, 0, result.stderr);
    assert.equal(readFileSync(`${outputPath}.argc`, "utf8"), "4");
    assert.deepEqual(readFileSync(`${outputPath}.argv`, "utf8").trim().split("\n"), [
      "--release-id", releaseID, "--receipt-sha256", receiptSHA256,
    ]);
    const environment = readFileSync(`${outputPath}.env`, "utf8");
    assert.match(environment, new RegExp(`^HENUKIT_MATERIALS_SEALED_ROOT=${realpathSync(join(root, "sealed"))}$`, "m"));
    assert.match(environment, new RegExp(`^HENUKIT_MATERIALS_OSS_AUDIT_ROOT=${realpathSync(join(root, "audit"))}$`, "m"));
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_BUCKET=henukit$/m);
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_REGION=cn-beijing$/m);
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_ENDPOINT=oss-cn-beijing-internal\.aliyuncs\.com$/m);
    assert.match(environment, /^ALIBABA_CLOUD_IMDSV1_DISABLE=true$/m);
    assert.doesNotMatch(environment, /caller-access-key|caller-secret|caller-token|caller-proxy|attacker-bucket|attacker\.example/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("complete OSS release wrapper rejects caller argv and credential-bearing configuration", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-publish-release-oss-"));
  try {
    const configPath = join(root, "materials-oss.env");
    const outputPath = join(root, "invocation");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);
    const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;

    const injectedArg = spawnSync(wrapper, [
      "--release-id", releaseID,
      "--receipt-sha256", "c".repeat(64),
      "--endpoint", "attacker.example",
    ], { encoding: "utf8" });
    assert.notEqual(injectedArg.status, 0);
    assert.equal(existsSync(`${outputPath}.argv`), false);

    writeConfig(configPath, root, "HTTPS_PROXY=http://caller-proxy.invalid");
    const injectedConfig = spawnSync(wrapper, [
      "--release-id", releaseID,
      "--receipt-sha256", "c".repeat(64),
    ], { encoding: "utf8" });
    assert.notEqual(injectedConfig.status, 0);
    assert.match(injectedConfig.stderr, /unsupported key/);
    assert.equal(existsSync(`${outputPath}.argv`), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
