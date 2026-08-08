import assert from "node:assert/strict";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
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

function stageWrapper(root, configPath, outputPath) {
  let wrapper = readFileSync(wrapperPath, "utf8");
  wrapper = wrapper.replace(
    'readonly config_path="/etc/henukit-deploy/materials-webhook.env"',
    `readonly config_path=${bashLiteral(configPath)}`,
  );
  wrapper = wrapper.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  const stagedPath = join(root, "henukit-materials-prepare");
  writeFileSync(stagedPath, wrapper, { mode: 0o700 });
  chmodSync(stagedPath, 0o700);
  writeFileSync(
    join(root, "prepare-henukit-materials.mjs"),
    [
      'import { writeFileSync } from "node:fs";',
      `writeFileSync(${JSON.stringify(outputPath)}, JSON.stringify({ argv: process.argv.slice(2), env: process.env }));`,
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
