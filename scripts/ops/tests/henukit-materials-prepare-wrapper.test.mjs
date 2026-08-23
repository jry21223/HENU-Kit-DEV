import assert from "node:assert/strict";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const wrapperPath = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-prepare");
const acceptedSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

function bashLiteral(value) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

function stageWrapper(
  root,
  configPath,
  outputPath,
  { failCleanup = false, failPreparation = false, missingCleanupCommand = false } = {},
) {
  let wrapper = readFileSync(wrapperPath, "utf8");
  wrapper = wrapper.replace(
    'readonly config_path="/etc/henukit-deploy/materials-webhook.env"',
    `readonly config_path=${bashLiteral(configPath)}`,
  );
  wrapper = wrapper.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  const remove = join(root, "rm");
  if (!missingCleanupCommand) {
    writeFileSync(
      remove,
      failCleanup
        ? "#!/bin/sh\nexit 9\n"
        : "#!/bin/sh\n[ \"$#\" -eq 4 ] && [ \"$1\" = -rf ] && [ \"$2\" = --one-file-system ] && [ \"$3\" = -- ]\nexec /bin/rm -rf -- \"$4\"\n",
      { mode: 0o700 },
    );
    chmodSync(remove, 0o700);
  }
  wrapper = wrapper.replace('readonly rm_bin="/usr/bin/rm"', `readonly rm_bin=${bashLiteral(remove)}`);
  const stagedPath = join(root, "henukit-materials-prepare");
  writeFileSync(stagedPath, wrapper, { mode: 0o700 });
  chmodSync(stagedPath, 0o700);
  writeFileSync(
    join(root, "prepare-henukit-materials.mjs"),
    [
      'import { mkdirSync, writeFileSync } from "node:fs";',
      'const candidate = process.argv[process.argv.indexOf("--candidate-dir") + 1];',
      'mkdirSync(`${candidate}/public`, { recursive: true });',
      'writeFileSync(`${candidate}/public/asset.pdf`, "prepared bytes");',
      `writeFileSync(${JSON.stringify(outputPath)}, JSON.stringify({ argv: process.argv.slice(2), env: process.env }));`,
      failPreparation ? "process.exit(7);" : "",
    ].join("\n"),
    { mode: 0o600 },
  );
  return stagedPath;
}

function writeConfig(path, root) {
  const stateDir = join(root, "state");
  mkdirSync(stateDir, { recursive: true, mode: 0o700 });
  writeFileSync(
    path,
    [
      "HENUKIT_WEBHOOK_REPOSITORY=jry21223/HENU-Final-Review",
      "HENUKIT_WEBHOOK_BRANCH=main",
      `HENUKIT_WEBHOOK_STATE_DIR=${stateDir}`,
      "HENUKIT_MATERIALS_SOURCE_REPOSITORY=https://example.test/HENU-Final-Review.git",
      `HENUKIT_MATERIALS_CANDIDATE_ROOT=${join(stateDir, "candidates")}`,
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
      NODE_OPTIONS: "--trace-warnings",
      ...env,
    },
  });
}

test("materials preparation wrapper binds candidate inputs to root-owned configuration", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-wrapper-"));
  try {
    const configPath = join(root, "materials-webhook.env");
    const outputPath = join(root, "prepared.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);

    const result = runWrapper(wrapper, [
      "--sha", acceptedSHA,
      "--delivery", "delivery-1",
      "--repository", "jry21223/HENU-Final-Review",
      "--ref", "refs/heads/main",
    ]);

    assert.equal(result.status, 0, result.stderr);
    const invocation = JSON.parse(readFileSync(outputPath, "utf8"));
    assert.deepEqual(invocation.argv.slice(0, 6), [
      "--repository", "https://example.test/HENU-Final-Review.git",
      "--ref", "refs/heads/main",
      "--sha", acceptedSHA,
    ]);
    const candidateIndex = invocation.argv.indexOf("--candidate-dir");
    assert.notEqual(candidateIndex, -1);
    assert.match(invocation.argv[candidateIndex + 1], new RegExp("/state/candidates/.attempt\\.[A-Za-z0-9]+/candidate$"));
    assert.deepEqual(readdirSync(join(root, "state", "candidates")), []);
    assert.deepEqual(
      [
        invocation.env.HENUKIT_DEPLOY_COMMAND,
        invocation.env.HENUKIT_MATERIALS_DATABASE_URL,
        invocation.env.NODE_OPTIONS,
      ],
      [undefined, undefined, undefined],
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials preparation wrapper rejects events and arguments outside its fixed boundary", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-wrapper-"));
  try {
    const configPath = join(root, "materials-webhook.env");
    const outputPath = join(root, "prepared.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath);

    for (const item of [
      {
        name: "repository mismatch",
        args: ["--sha", acceptedSHA, "--delivery", "delivery-2", "--repository", "attacker/controlled-repository", "--ref", "refs/heads/main"],
        expected: /does not match configured webhook repository/,
      },
      {
        name: "ref mismatch",
        args: ["--sha", acceptedSHA, "--delivery", "delivery-3", "--repository", "jry21223/HENU-Final-Review", "--ref", "refs/heads/other"],
        expected: /does not match configured webhook branch/,
      },
      {
        name: "caller-selected candidate path",
        args: ["--sha", acceptedSHA, "--delivery", "delivery-4", "--repository", "jry21223/HENU-Final-Review", "--ref", "refs/heads/main", "--candidate-dir", "/tmp/attacker"],
        expected: /expected --sha/,
      },
    ]) {
      const result = runWrapper(wrapper, item.args);
      assert.notEqual(result.status, 0, item.name);
      assert.match(result.stderr, item.expected, item.name);
      assert.equal(existsSync(outputPath), false, item.name);
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials preparation wrapper releases its private attempt when preparation fails", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-wrapper-"));
  try {
    const configPath = join(root, "materials-webhook.env");
    const outputPath = join(root, "prepared.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath, { failPreparation: true });

    const result = runWrapper(wrapper, [
      "--sha", acceptedSHA,
      "--delivery", "delivery-failed",
      "--repository", "jry21223/HENU-Final-Review",
      "--ref", "refs/heads/main",
    ]);

    assert.notEqual(result.status, 0);
    assert.deepEqual(readdirSync(join(root, "state", "candidates")), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials preparation wrapper fails closed before returning a locator when cleanup fails", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-wrapper-"));
  try {
    const configPath = join(root, "materials-webhook.env");
    const outputPath = join(root, "prepared.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath, { failCleanup: true });

    const result = runWrapper(wrapper, [
      "--sha", acceptedSHA,
      "--delivery", "delivery-cleanup-failed",
      "--repository", "jry21223/HENU-Final-Review",
      "--ref", "refs/heads/main",
    ]);

    assert.notEqual(result.status, 0);
    assert.doesNotMatch(result.stdout, /attempt_locator=/);
    assert.match(result.stderr, /keep the materials runner disabled.*production runbook/i);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials preparation wrapper gives an actionable safe error when preparation and cleanup both fail", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-wrapper-"));
  try {
    const configPath = join(root, "materials-webhook.env");
    const outputPath = join(root, "prepared.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath, {
      failCleanup: true,
      failPreparation: true,
    });

    const result = runWrapper(wrapper, [
      "--sha", acceptedSHA,
      "--delivery", "delivery-double-failed",
      "--repository", "jry21223/HENU-Final-Review",
      "--ref", "refs/heads/main",
    ]);

    assert.notEqual(result.status, 0);
    assert.doesNotMatch(result.stdout, /attempt_locator=/);
    assert.match(result.stderr, /keep the materials runner disabled.*production runbook/i);
    assert.doesNotMatch(result.stderr, /\.attempt\.|delivery-double-failed|state\/candidates/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("materials preparation wrapper keeps the runner disabled when its fixed cleanup command is unavailable", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-wrapper-"));
  try {
    const configPath = join(root, "materials-webhook.env");
    const outputPath = join(root, "prepared.json");
    writeConfig(configPath, root);
    const wrapper = stageWrapper(root, configPath, outputPath, { missingCleanupCommand: true });

    const result = runWrapper(wrapper, [
      "--sha", acceptedSHA,
      "--delivery", "delivery-missing-cleanup",
      "--repository", "jry21223/HENU-Final-Review",
      "--ref", "refs/heads/main",
    ]);

    assert.notEqual(result.status, 0);
    assert.doesNotMatch(result.stdout, /attempt_locator=/);
    assert.match(result.stderr, /keep the materials runner disabled.*reinstall the signed runtime/i);
    assert.deepEqual(readdirSync(join(root, "state", "candidates")), []);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
