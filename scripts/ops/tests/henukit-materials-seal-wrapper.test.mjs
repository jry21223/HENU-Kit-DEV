import assert from "node:assert/strict";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const templatePath = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-seal");
const programPath = join(repositoryRoot, "scripts", "ops", "seal-henukit-materials.mjs");

function bashLiteral(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function stageWrapper(root, configPath, outputPath) {
  let template = readFileSync(templatePath, "utf8");
  template = template.replace(
    'readonly config_path="/etc/henukit-deploy/materials-seal.env"',
    `readonly config_path=${bashLiteral(configPath)}`,
  );
  template = template.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  const staged = join(root, "henukit-materials-seal");
  writeFileSync(staged, template, { mode: 0o700 });
  chmodSync(staged, 0o700);
  writeFileSync(
    join(root, "seal-henukit-materials.mjs"),
    [
      'import { writeFileSync } from "node:fs";',
      `writeFileSync(${JSON.stringify(outputPath)}, JSON.stringify({ argv: process.argv.slice(2), env: process.env }));`,
    ].join("\n"),
    { mode: 0o600 },
  );
  return staged;
}

function writeConfig(path, root) {
  const sealedRoot = join(root, "sealed-root");
  mkdirSync(sealedRoot, { recursive: true, mode: 0o700 });
  writeFileSync(
    path,
    [
      `HENUKIT_MATERIALS_SEALED_ROOT=${sealedRoot}`,
      "HENUKIT_MATERIALS_SOURCE_REPOSITORY=https://example.test/HENU-Final-Review.git",
      "HENUKIT_MATERIALS_SOURCE_REF=refs/heads/main",
      "",
    ].join("\n"),
    { mode: 0o600 },
  );
}

function runWrapper(wrapper, args, env = {}) {
  return spawnSync(wrapper, args, {
    encoding: "utf8",
    env: {
      ...process.env,
      HENUKIT_DEPLOY_COMMAND: "/tmp/attacker-command",
      HENUKIT_MATERIALS_DATABASE_URL: "postgres://attacker.example/test",
      HENUKIT_MATERIALS_PUBLIC_ROOT: "/tmp/attacker-public",
      NODE_OPTIONS: "--trace-warnings",
      ...env,
    },
  });
}

test("materials sealing wrapper fixes every authority-bearing input in root-owned configuration", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-seal-wrapper-"));
  try {
    const configPath = join(root, "materials-seal.env");
    const outputPath = join(root, "invocation.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);

    const result = runWrapper(wrapper, ["--attempt", ".attempt.Ab1Cd2Ef3G", "--sha", "a".repeat(40)]);

    assert.equal(result.status, 0, result.stderr);
    const invocation = JSON.parse(readFileSync(outputPath, "utf8"));
    assert.deepEqual(invocation.argv, [
      "--attempt", ".attempt.Ab1Cd2Ef3G",
      "--sealed-root", join(root, "sealed-root"),
      "--repository", "https://example.test/HENU-Final-Review.git",
      "--ref", "refs/heads/main",
      "--sha", "a".repeat(40),
      "--sealed-owner", String(process.getuid()),
    ]);
    assert.deepEqual(
      [
        invocation.env.HENUKIT_DEPLOY_COMMAND,
        invocation.env.HENUKIT_MATERIALS_DATABASE_URL,
        invocation.env.HENUKIT_MATERIALS_PUBLIC_ROOT,
        invocation.env.NODE_OPTIONS,
      ],
      [undefined, undefined, undefined, undefined],
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials sealing wrapper ignores a caller-controlled command lookup environment", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-seal-wrapper-"));
  try {
    const configPath = join(root, "materials-seal.env");
    const outputPath = join(root, "invocation.json");
    const attackerBin = join(root, "attacker-bin");
    const attackerMarker = join(root, "attacker-ran");
    const attackerBashEnv = join(root, "attacker-bash-env");
    mkdirSync(attackerBin);
    const attackerCommand = `#!/bin/sh\n/usr/bin/touch ${bashLiteral(attackerMarker)}\nexit 97\n`;
    for (const command of ["bash", "dirname", "stat", "id", "env"]) {
      const path = join(attackerBin, command);
      writeFileSync(path, attackerCommand, { mode: 0o700 });
      chmodSync(path, 0o700);
    }
    writeFileSync(attackerBashEnv, attackerCommand, { mode: 0o600 });
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);

    const result = runWrapper(wrapper, ["--attempt", ".attempt.Ab1Cd2Ef3G", "--sha", "a".repeat(40)], {
      PATH: attackerBin,
      BASH_ENV: attackerBashEnv,
      ENV: attackerBashEnv,
    });

    assert.equal(result.status, 0, result.stderr);
    assert.equal(existsSync(attackerMarker), false);
    const invocation = JSON.parse(readFileSync(outputPath, "utf8"));
    assert.equal(invocation.env.PATH, "/usr/bin:/bin");
    assert.equal(invocation.env.BASH_ENV, undefined);
    assert.equal(invocation.env.ENV, undefined);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials sealing wrapper rejects caller-selected paths and unsafe root configuration", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-seal-wrapper-"));
  try {
    const configPath = join(root, "materials-seal.env");
    const outputPath = join(root, "invocation.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);

    for (const item of [
      {
        name: "caller-selected candidate path",
        args: ["--attempt", ".attempt.Ab1Cd2Ef3G", "--candidate-dir", "/tmp/attacker"],
        expected: /expected --attempt/,
      },
      {
        name: "invalid locator",
        args: ["--attempt", "/tmp/attacker", "--sha", "a".repeat(40)],
        expected: /attempt locator is invalid/,
      },
    ]) {
      const result = runWrapper(wrapper, item.args);
      assert.notEqual(result.status, 0, item.name);
      assert.match(result.stderr, item.expected, item.name);
      assert.equal(existsSync(outputPath), false, item.name);
    }

    chmodSync(configPath, 0o660);
    const writableConfig = runWrapper(wrapper, ["--attempt", ".attempt.Ab1Cd2Ef3G", "--sha", "a".repeat(40)]);
    assert.notEqual(writableConfig.status, 0);
    assert.match(writableConfig.stderr, /must not be writable by group or other/);
    assert.equal(existsSync(outputPath), false);

    chmodSync(configPath, 0o600);
    const symlinkConfig = join(root, "materials-seal-link.env");
    symlinkSync(configPath, symlinkConfig);
    const linkedWrapper = stageWrapper(root, symlinkConfig, outputPath);
    const linkedConfig = runWrapper(linkedWrapper, ["--attempt", ".attempt.Ab1Cd2Ef3G", "--sha", "a".repeat(40)]);
    assert.notEqual(linkedConfig.status, 0);
    assert.match(linkedConfig.stderr, /configuration must be a regular file/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("sealed-release template stays root-only and cannot become a B01 runner target", () => {
  const template = readFileSync(templatePath, "utf8");
  const program = readFileSync(programPath, "utf8");
  const receiver = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "systemd", "henukit-materials-webhook.service"), "utf8");
  const runner = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "systemd", "henukit-materials-runner.service"), "utf8");
  const installer = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "install-materials-runtime.sh"), "utf8");

  assert.notEqual(statSync(templatePath).mode & 0o111, 0, "the root-only template remains executable when intentionally installed later");
  assert.match(template, /^readonly config_path="\/etc\/henukit-deploy\/materials-seal\.env"$/m);
  assert.match(template, /^readonly config_owner="0"$/m);
  assert.match(template, /^#!\/bin\/sh$/m);
  assert.match(template, /^export PATH="\/usr\/bin:\/bin"$/m);
  assert.match(template, /^readonly PATH$/m);
  assert.match(template, /readonly stat_bin="\/usr\/bin\/stat"/);
  assert.match(template, /readonly stat_bin="\/bin\/stat"/);
  assert.match(template, /readonly id_bin="\/usr\/bin\/id"/);
  assert.match(template, /readonly env_bin="\/usr\/bin\/env"/);
  assert.match(template, /^\[ "\$\("\$id_bin" -u\)" = "\$config_owner" \] \|\| fail "must run as root"$/m);
  assert.match(template, /^exec "\$env_bin" -i \\$/m);
  assert.doesNotMatch(template, /HENUKIT_MATERIALS_SOURCE_SHA/);
  assert.match(template, /--sha "\$accepted_sha"/);
  assert.match(template, /--sealed-owner "\$config_owner"/);
  assert.doesNotMatch(template, /HENUKIT_MATERIALS_CANDIDATE_ROOT|--candidate-root/);
  assert.doesNotMatch(template, /HENUKIT_MATERIALS_DATABASE_URL|HENUKIT_MATERIALS_PUBLIC_ROOT|--public|--database|--activate|--approve/);
  assert.doesNotMatch(program, /candidateRoot|parseCandidateRelease|candidateManifest|candidateSlides|candidatePublic/);
  assert.doesNotMatch(program, /convert-henukit-slides|LibreOffice|soffice|python/i);
  assert.match(program, /Buffer\.compare/);
  assert.match(program, /function fsyncOutputDirectories/);
  assert.match(program, /fsyncOutputDirectories\(provisional\)/);
  assert.doesNotMatch(receiver, /henukit-materials-seal/);
  assert.doesNotMatch(runner, /henukit-materials-seal/);
  assert.match(installer, /libexec\/henukit-materials-seal\|\/usr\/local\/libexec\/henukit\/henukit-materials-seal\|0700/);
});
