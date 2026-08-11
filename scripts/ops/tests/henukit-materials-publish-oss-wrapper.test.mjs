import assert from "node:assert/strict";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const templatePath = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-publish-oss");

function shellLiteral(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
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
  const staged = join(root, "henukit-materials-publish-oss");
  writeFileSync(staged, template, { mode: 0o700 });
  chmodSync(staged, 0o700);
  writeFileSync(
    join(root, "materials-oss-canary"),
    [
      "#!/bin/sh",
      `printf '%s' \"$#\" > ${shellLiteral(outputPath)}.argc`,
      `printf '%s\\n' \"$@\" > ${shellLiteral(outputPath)}.argv`,
      `env > ${shellLiteral(outputPath)}.env`,
    ].join("\n"),
    { mode: 0o700 },
  );
  chmodSync(join(root, "materials-oss-canary"), 0o700);
  return staged;
}

function writeConfig(path, root, overrides = {}) {
  const sealedRoot = join(root, "sealed");
  const auditRoot = overrides.auditRoot ?? join(root, "audit");
  mkdirSync(sealedRoot, { recursive: true, mode: 0o700 });
  mkdirSync(auditRoot, { recursive: true, mode: 0o700 });
  writeFileSync(
    path,
    [
      `HENUKIT_MATERIALS_SEALED_ROOT=${sealedRoot}`,
      `HENUKIT_MATERIALS_OSS_AUDIT_ROOT=${auditRoot}`,
      "HENUKIT_MATERIALS_OSS_BUCKET=henukit",
      "HENUKIT_MATERIALS_OSS_REGION=cn-beijing",
      "HENUKIT_MATERIALS_OSS_ENDPOINT=oss-cn-beijing-internal.aliyuncs.com",
      "HENUKIT_MATERIALS_OSS_RAM_ROLE=henukit-materials-oss-publisher",
      "",
    ].join("\n"),
    { mode: 0o600 },
  );
}

test("OSS canary wrapper exposes only sealed identity and asset digest while fixing storage authority", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-publish-oss-wrapper-"));
  try {
    const configPath = join(root, "materials-oss.env");
    const outputPath = join(root, "invocation");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);
    const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
    const receiptSha256 = "c".repeat(64);
    const assetSha256 = "d".repeat(64);

    const result = spawnSync(wrapper, [
      "--release-id", releaseID,
      "--receipt-sha256", receiptSha256,
      "--asset-sha256", assetSha256,
    ], {
      encoding: "utf8",
      env: {
        ...process.env,
        ALIBABA_CLOUD_ACCESS_KEY_ID: "caller-access-key",
        ALIBABA_CLOUD_ACCESS_KEY_SECRET: "caller-secret",
        ALIBABA_CLOUD_SECURITY_TOKEN: "caller-token",
        HENUKIT_MATERIALS_OSS_BUCKET: "attacker-bucket",
        HENUKIT_MATERIALS_OSS_ENDPOINT: "attacker.example",
      },
    });

    assert.equal(result.status, 0, result.stderr);
    assert.equal(readFileSync(`${outputPath}.argc`, "utf8"), "6");
    assert.deepEqual(readFileSync(`${outputPath}.argv`, "utf8").trim().split("\n"), [
      "--release-id", releaseID,
      "--receipt-sha256", receiptSha256,
      "--asset-sha256", assetSha256,
    ]);
    const environment = readFileSync(`${outputPath}.env`, "utf8");
    assert.match(environment, new RegExp(`^HENUKIT_MATERIALS_SEALED_ROOT=${realpathSync(join(root, "sealed"))}$`, "m"));
    assert.match(environment, new RegExp(`^HENUKIT_MATERIALS_OSS_AUDIT_ROOT=${realpathSync(join(root, "audit"))}$`, "m"));
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_BUCKET=henukit$/m);
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_REGION=cn-beijing$/m);
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_ENDPOINT=oss-cn-beijing-internal\.aliyuncs\.com$/m);
    assert.match(environment, /^HENUKIT_MATERIALS_OSS_RAM_ROLE=henukit-materials-oss-publisher$/m);
    assert.doesNotMatch(environment, /caller-access-key|caller-secret|caller-token|attacker-bucket|attacker\.example/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("OSS canary wrapper keeps the mutable publication audit outside the immutable sealed root", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-publish-oss-wrapper-"));
  try {
    const configPath = join(root, "materials-oss.env");
    const outputPath = join(root, "invocation");
    writeConfig(configPath, root, { auditRoot: join(root, "sealed") });
    const wrapper = stageWrapper(root, configPath, outputPath);

    const result = spawnSync(wrapper, [
      "--release-id", `${"a".repeat(40)}-${"b".repeat(16)}`,
      "--receipt-sha256", "c".repeat(64),
      "--asset-sha256", "d".repeat(64),
    ], { encoding: "utf8" });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /audit root must be separate from the sealed root/);

		writeConfig(configPath, root, { auditRoot: join(root, "sealed", "audit") });
		const nested = spawnSync(wrapper, [
			"--release-id", `${"a".repeat(40)}-${"b".repeat(16)}`,
			"--receipt-sha256", "c".repeat(64),
			"--asset-sha256", "d".repeat(64),
		], { encoding: "utf8" });
		assert.notEqual(nested.status, 0);
		assert.match(nested.stderr, /audit root must be separate from the sealed root/);

		writeConfig(configPath, root, { auditRoot: join(root, "writable", "audit") });
		chmodSync(join(root, "writable"), 0o777);
		const writableAncestor = spawnSync(wrapper, [
			"--release-id", `${"a".repeat(40)}-${"b".repeat(16)}`,
			"--receipt-sha256", "c".repeat(64),
			"--asset-sha256", "d".repeat(64),
		], { encoding: "utf8" });
		assert.notEqual(writableAncestor.status, 0);
		assert.match(writableAncestor.stderr, /must not be writable by group or other/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("OSS canary wrapper rejects caller authority and credential-bearing configuration", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-publish-oss-wrapper-"));
  try {
    const configPath = join(root, "materials-oss.env");
    const outputPath = join(root, "invocation");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);
    const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;

    const callerBucket = spawnSync(wrapper, [
      "--release-id", releaseID,
      "--receipt-sha256", "c".repeat(64),
      "--bucket", "attacker-bucket",
    ], { encoding: "utf8" });
    assert.notEqual(callerBucket.status, 0);
    assert.match(callerBucket.stderr, /expected --release-id ID --receipt-sha256 SHA256 --asset-sha256 SHA256/);
    assert.equal(existsSync(`${outputPath}.argv`), false);

    writeFileSync(
      configPath,
      `${readFileSync(configPath, "utf8")}ALIBABA_CLOUD_ACCESS_KEY_ID=must-not-be-supported\n`,
      { mode: 0o600 },
    );
    const credentialConfig = spawnSync(wrapper, [
      "--release-id", releaseID,
      "--receipt-sha256", "c".repeat(64),
      "--asset-sha256", "d".repeat(64),
    ], { encoding: "utf8" });
    assert.notEqual(credentialConfig.status, 0);
    assert.match(credentialConfig.stderr, /configuration contains an unsupported key/);
    assert.equal(existsSync(`${outputPath}.argv`), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("OSS canary wrapper requires private root-owned configuration", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-publish-oss-wrapper-"));
  try {
    const configPath = join(root, "materials-oss.env");
    const outputPath = join(root, "invocation");
    writeConfig(configPath, root);
    chmodSync(configPath, 0o640);
    const wrapper = stageWrapper(root, configPath, outputPath);

    const result = spawnSync(wrapper, [
      "--release-id", `${"a".repeat(40)}-${"b".repeat(16)}`,
      "--receipt-sha256", "c".repeat(64),
      "--asset-sha256", "d".repeat(64),
    ], { encoding: "utf8" });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /configuration must have mode 0600/);
    assert.equal(existsSync(`${outputPath}.argv`), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
