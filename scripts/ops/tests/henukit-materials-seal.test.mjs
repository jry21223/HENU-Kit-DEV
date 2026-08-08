import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { createHash } from "node:crypto";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const sealTemplate = join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-seal");
const sealProgram = join(repositoryRoot, "scripts", "ops", "seal-henukit-materials.mjs");

function sha256(content) {
  return createHash("sha256").update(content).digest("hex");
}

function git(repository, args) {
  return execFileSync("git", args, { cwd: repository, encoding: "utf8" }).trim();
}

function commit(repository) {
  git(repository, ["add", "."]);
  git(repository, ["-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "fixture"]);
  return git(repository, ["rev-parse", "HEAD"]);
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-seal-"));
  const repository = join(root, "source");
  const attempt = ".attempt.Ab1Cd2Ef3G";
  const candidate = join(root, "untrusted-candidates", attempt, "candidate");
  const sealedRoot = join(root, "sealed");
  const publicSentinel = join(root, "served-public", "keep.txt");
  const studySentinel = join(root, "study-catalog.json");
  const bytes = Buffer.from("%PDF-1.4\\nsealed fixture material\\n");
  const publicPath = "materials/outline.pdf";

  mkdirSync(join(repository, "materials"), { recursive: true });
  writeFileSync(join(repository, publicPath), bytes);
  writeFileSync(
    join(repository, "manifest.json"),
    JSON.stringify({
      version: 1,
      subjects: [{
        name: "离散数学",
        assets: [{
          role: "复习讲义",
          title: "离散数学_复习讲义_提纲.pdf",
          publicPath,
          bytes: bytes.length,
          sha256: sha256(bytes),
        }],
      }],
    }),
  );
  git(root, ["init", "--initial-branch=main", repository]);
  const sha = commit(repository);
  mkdirSync(candidate, { recursive: true });
  writeFileSync(join(candidate, "READY"), "untrusted completion marker\\n");
  mkdirSync(sealedRoot, { recursive: true, mode: 0o700 });
  chmodSync(sealedRoot, 0o700);
  mkdirSync(dirname(publicSentinel), { recursive: true });
  writeFileSync(publicSentinel, "served sentinel");
  writeFileSync(studySentinel, "study sentinel");

  return {
    root,
    repository,
    attempt,
    candidate,
    sealedRoot,
    publicSentinel,
    studySentinel,
    bytes,
    publicPath,
    sha,
  };
}

function bashLiteral(value) {
  return `'${value.replaceAll("'", "'\\\\''")}'`;
}

function stageSeal(root, configPath, { renameBarrier } = {}) {
  let template = readFileSync(sealTemplate, "utf8");
  template = template.replace(
    'readonly config_path="/etc/henukit-deploy/materials-seal.env"',
    `readonly config_path=${bashLiteral(configPath)}`,
  );
  template = template.replace('readonly config_owner="0"', `readonly config_owner="${process.getuid()}"`);
  const stagedTemplate = join(root, "henukit-materials-seal");
  writeFileSync(stagedTemplate, template, { mode: 0o700 });
  chmodSync(stagedTemplate, 0o700);
  let program = readFileSync(sealProgram, "utf8");
  if (renameBarrier) {
    const rename = "    try {\n      renameSync(provisional, finalPath);";
    const pausedRename = `    try {\n      {\n        const barrierDescriptor = openSync(${JSON.stringify(renameBarrier.ready)}, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, 0o600);\n        closeSync(barrierDescriptor);\n        const barrierDeadline = Date.now() + 5_000;\n        while (!existsSync(${JSON.stringify(renameBarrier.proceed)})) {\n          if (Date.now() >= barrierDeadline) fail("test rename barrier timed out");\n          Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 25);\n        }\n      }\n      renameSync(provisional, finalPath);`;
    assert.notEqual(program.indexOf(rename), -1, "test barrier must target the atomic rename boundary");
    program = program.replace(rename, pausedRename);
  }
  writeFileSync(join(root, "seal-henukit-materials.mjs"), program, { mode: 0o600 });
  return stagedTemplate;
}

function writeConfig(path, setup, sha = setup.sha) {
  writeFileSync(
    path,
    [
      `HENUKIT_MATERIALS_SEALED_ROOT=${setup.sealedRoot}`,
      `HENUKIT_MATERIALS_SOURCE_REPOSITORY=${setup.repository}`,
      "HENUKIT_MATERIALS_SOURCE_REF=refs/heads/main",
      `HENUKIT_MATERIALS_SOURCE_SHA=${sha}`,
      "",
    ].join("\n"),
    { mode: 0o600 },
  );
}

function releaseID(setup, sha = setup.sha) {
  return `${sha}-${sha256(readFileSync(join(setup.repository, "manifest.json"))).slice(0, 16)}`;
}

function runSeal(setup, configPath, { attempt = setup.attempt, env = {} } = {}) {
  return spawnSync(stageSeal(setup.root, configPath), ["--attempt", attempt], {
    encoding: "utf8",
    env: { ...process.env, ...env },
  });
}

function startSeal(setup, configPath, { attempt = setup.attempt, renameBarrier } = {}) {
  return spawn(stageSeal(setup.root, configPath, { renameBarrier }), ["--attempt", attempt], {
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function waitForPath(path, timeoutMilliseconds = 5_000) {
  return new Promise((resolvePromise, reject) => {
    const deadline = Date.now() + timeoutMilliseconds;
    function poll() {
      if (existsSync(path)) {
        resolvePromise();
        return;
      }
      if (Date.now() >= deadline) {
        reject(new Error(`timed out waiting for ${path}`));
        return;
      }
      setTimeout(poll, 10);
    }
    poll();
  });
}

function collectChild(child) {
  return new Promise((resolvePromise, reject) => {
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.once("error", reject);
    child.once("close", (status, signal) => resolvePromise({ status, signal, stdout, stderr }));
  });
}

test("seals source-derived raw assets through the constrained attempt CLI", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);

    const result = runSeal(setup, configPath);

    assert.equal(result.status, 0, result.stderr);
    const output = JSON.parse(result.stdout);
    assert.equal(output.attempt_locator, setup.attempt);
    assert.equal(output.release_id, releaseID(setup));
    assert.match(output.receipt_sha256, /^[a-f0-9]{64}$/);
    const release = join(setup.sealedRoot, output.release_id);
    const inventory = JSON.parse(readFileSync(join(release, "inventory.json"), "utf8"));
    const receipt = JSON.parse(readFileSync(join(release, "sealed-release.json"), "utf8"));
    assert.deepEqual(readFileSync(join(release, "public", "materials", "outline.pdf")), setup.bytes);
    assert.equal(Object.hasOwn(receipt, "attempt_locator"), false);
    assert.equal(Object.hasOwn(inventory, "attempt_locator"), false);
    assert.deepEqual(
      JSON.parse(readFileSync(join(setup.sealedRoot, ".audit", output.release_id, "Ab1Cd2Ef3G.json"), "utf8")),
      {
        version: 1,
        release_id: output.release_id,
        receipt_sha256: output.receipt_sha256,
        attempt_locator: setup.attempt,
      },
    );
    assert.deepEqual(inventory.slides, { status: "deferred", source_slide_assets: 0 });
    assert.equal(existsSync(join(release, "slides")), false);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("untrusted candidate content, including a symlinked attempt directory, is never read", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const outside = join(setup.root, "attacker-controlled");
    mkdirSync(outside);
    writeFileSync(join(outside, "READY"), "not an accepted SHA\\n");
    rmSync(setup.candidate, { recursive: true });
    symlinkSync(outside, setup.candidate);

    const result = runSeal(setup, configPath);

    assert.equal(result.status, 0, result.stderr);
    const output = JSON.parse(result.stdout);
    assert.equal(output.release_id, releaseID(setup));
    assert.deepEqual(
      readFileSync(join(setup.sealedRoot, output.release_id, "public", "materials", "outline.pdf")),
      setup.bytes,
    );
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("an identical configured source is idempotent and a different receipt is never overwritten", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const first = runSeal(setup, configPath);
    assert.equal(first.status, 0, first.stderr);
    const firstOutput = JSON.parse(first.stdout);
    const release = join(setup.sealedRoot, firstOutput.release_id);
    const originalReceipt = readFileSync(join(release, "sealed-release.json"), "utf8");

    const second = runSeal(setup, configPath);
    assert.equal(second.status, 0, second.stderr);
    assert.deepEqual(JSON.parse(second.stdout), firstOutput);
    assert.equal(readFileSync(join(release, "sealed-release.json"), "utf8"), originalReceipt);

    const inventory = join(release, "inventory.json");
    chmodSync(inventory, 0o600);
    writeFileSync(inventory, "{\"unexpected\":true}\\n");
    const conflict = runSeal(setup, configPath);
    assert.notEqual(conflict.status, 0);
    assert.match(conflict.stderr, /sealed release ID already exists with a different receipt/);
    assert.equal(readFileSync(inventory, "utf8"), "{\"unexpected\":true}\\n");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a different attempt appends root-owned audit correlation without changing canonical release identity", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const first = runSeal(setup, configPath);
    assert.equal(first.status, 0, first.stderr);
    const firstOutput = JSON.parse(first.stdout);
    const release = join(setup.sealedRoot, firstOutput.release_id);
    const receipt = readFileSync(join(release, "sealed-release.json"), "utf8");
    const inventory = readFileSync(join(release, "inventory.json"), "utf8");
    const secondAttempt = ".attempt.K9Lm8N7p6Q";

    const second = runSeal(setup, configPath, { attempt: secondAttempt });

    assert.equal(second.status, 0, second.stderr);
    const secondOutput = JSON.parse(second.stdout);
    assert.equal(secondOutput.attempt_locator, secondAttempt);
    assert.equal(secondOutput.release_id, firstOutput.release_id);
    assert.equal(secondOutput.receipt_sha256, firstOutput.receipt_sha256);
    assert.equal(readFileSync(join(release, "sealed-release.json"), "utf8"), receipt);
    assert.equal(readFileSync(join(release, "inventory.json"), "utf8"), inventory);
    assert.deepEqual(readdirSync(join(setup.sealedRoot, ".audit", firstOutput.release_id)).sort(), [
      "Ab1Cd2Ef3G.json",
      "K9Lm8N7p6Q.json",
    ]);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a concurrent identical release records audit correlation for both accepted attempts", async () => {
  const setup = fixture();
  let paused;
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const pausedAttempt = ".attempt.K9Lm8N7p6Q";
    const barrier = {
      ready: join(setup.root, "rename-ready"),
      proceed: join(setup.root, "rename-proceed"),
    };

    paused = startSeal(setup, configPath, { attempt: pausedAttempt, renameBarrier: barrier });
    await waitForPath(barrier.ready);

    const winner = runSeal(setup, configPath);
    assert.equal(winner.status, 0, winner.stderr);
    const winnerOutput = JSON.parse(winner.stdout);

    writeFileSync(barrier.proceed, "continue\n", { mode: 0o600 });
    const concurrent = await collectChild(paused);
    paused = undefined;

    assert.equal(concurrent.status, 0, concurrent.stderr);
    const concurrentOutput = JSON.parse(concurrent.stdout);
    assert.equal(concurrentOutput.release_id, winnerOutput.release_id);
    assert.equal(concurrentOutput.receipt_sha256, winnerOutput.receipt_sha256);
    assert.equal(concurrentOutput.attempt_locator, pausedAttempt);
    assert.deepEqual(readdirSync(join(setup.sealedRoot, ".audit", winnerOutput.release_id)).sort(), [
      "Ab1Cd2Ef3G.json",
      "K9Lm8N7p6Q.json",
    ]);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    if (paused && paused.exitCode === null) paused.kill("SIGKILL");
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a writable audit root cannot append correlation or change the sealed identity", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const first = runSeal(setup, configPath);
    assert.equal(first.status, 0, first.stderr);
    const firstOutput = JSON.parse(first.stdout);
    const release = join(setup.sealedRoot, firstOutput.release_id);
    const receipt = readFileSync(join(release, "sealed-release.json"), "utf8");
    const auditRoot = join(setup.sealedRoot, ".audit");
    chmodSync(auditRoot, 0o770);

    const second = runSeal(setup, configPath, { attempt: ".attempt.K9Lm8N7p6Q" });

    assert.notEqual(second.status, 0);
    assert.match(second.stderr, /sealed audit root must not be writable by group or other/);
    assert.equal(readFileSync(join(release, "sealed-release.json"), "utf8"), receipt);
    assert.deepEqual(readdirSync(join(auditRoot, firstOutput.release_id)), ["Ab1Cd2Ef3G.json"]);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a writable pre-seeded release is rejected before idempotence and left untouched", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const malicious = join(setup.sealedRoot, releaseID(setup));
    mkdirSync(malicious);
    chmodSync(malicious, 0o770);
    writeFileSync(join(malicious, "sealed-sentinel"), "malicious preseed");

    const result = runSeal(setup, configPath);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /existing sealed release must not be writable by group or other/);
    assert.equal(readFileSync(join(malicious, "sealed-sentinel"), "utf8"), "malicious preseed");
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a dangling pre-seeded release symlink is rejected before it can be replaced", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    const malicious = join(setup.sealedRoot, releaseID(setup));
    symlinkSync(join(setup.root, "attacker-controlled-target"), malicious);

    const result = runSeal(setup, configPath);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /existing sealed release must not be a symbolic link/);
    assert.equal(lstatSync(malicious).isSymbolicLink(), true);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a writable sealed root is rejected before any provisional receipt is created", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    chmodSync(setup.sealedRoot, 0o770);

    const result = runSeal(setup, configPath);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /sealed root must not be writable by group or other/);
    assert.deepEqual(readdirSync(setup.sealedRoot), []);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a source ref advance fails closed against the root-configured exact SHA", () => {
  const setup = fixture();
  try {
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup);
    writeFileSync(join(setup.repository, "newer-source.txt"), "newer source ref");
    commit(setup.repository);

    const result = runSeal(setup, configPath);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /configured SHA [a-f0-9]{40} does not match resolved refs\/heads\/main/);
    assert.deepEqual(readdirSync(setup.sealedRoot), []);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("a duplicate reviewed source manifest is rejected before a sealed receipt exists", () => {
  const setup = fixture();
  try {
    const manifestPath = join(setup.repository, "manifest.json");
    const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
    manifest.subjects[0].assets.push({ ...manifest.subjects[0].assets[0], title: "重复资料" });
    writeFileSync(manifestPath, JSON.stringify(manifest));
    const updatedSHA = commit(setup.repository);
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup, updatedSHA);

    const result = runSeal(setup, configPath);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /duplicate reviewed asset path/);
    assert.deepEqual(readdirSync(setup.sealedRoot), []);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("canonical inventory order is UTF-8 bytewise and does not depend on caller locale", () => {
  const setup = fixture();
  try {
    const upperPath = "materials/B.pdf";
    const lowerPath = "materials/a.pdf";
    const upper = Buffer.from("upper bytewise path");
    const lower = Buffer.from("lower bytewise path");
    writeFileSync(join(setup.repository, upperPath), upper);
    writeFileSync(join(setup.repository, lowerPath), lower);
    writeFileSync(
      join(setup.repository, "manifest.json"),
      JSON.stringify({
        version: 1,
        subjects: [{
          name: "离散数学",
          assets: [
            { role: "复习讲义", title: "a", publicPath: lowerPath, bytes: lower.length, sha256: sha256(lower) },
            { role: "复习讲义", title: "B", publicPath: upperPath, bytes: upper.length, sha256: sha256(upper) },
          ],
        }],
      }),
    );
    const updatedSHA = commit(setup.repository);
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup, updatedSHA);

    const first = runSeal(setup, configPath, { env: { LC_ALL: "C" } });
    assert.equal(first.status, 0, first.stderr);
    const second = runSeal(setup, configPath, { env: { LC_ALL: "zh_CN.UTF-8" } });
    assert.equal(second.status, 0, second.stderr);
    assert.deepEqual(JSON.parse(second.stdout), JSON.parse(first.stdout));
    const output = JSON.parse(first.stdout);
    const inventory = JSON.parse(readFileSync(join(setup.sealedRoot, output.release_id, "inventory.json"), "utf8"));
    assert.deepEqual(inventory.assets.map((asset) => asset.public_path), [upperPath, lowerPath]);
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("source slide decks remain raw assets and derived Slides are explicitly deferred", () => {
  const setup = fixture();
  try {
    const pptxPath = "materials/outline.pptx";
    const pptx = Buffer.from("source slide deck");
    writeFileSync(join(setup.repository, pptxPath), pptx);
    writeFileSync(
      join(setup.repository, "manifest.json"),
      JSON.stringify({
        version: 1,
        subjects: [{
          name: "离散数学",
          assets: [{
            role: "课件PPT",
            title: "离散数学_课件PPT_提纲.pptx",
            publicPath: pptxPath,
            bytes: pptx.length,
            sha256: sha256(pptx),
          }],
        }],
      }),
    );
    const updatedSHA = commit(setup.repository);
    const configPath = join(setup.root, "materials-seal.env");
    writeConfig(configPath, setup, updatedSHA);
    writeFileSync(join(setup.candidate, "untrusted-slide.json"), "{not JSON");

    const result = runSeal(setup, configPath);

    assert.equal(result.status, 0, result.stderr);
    const output = JSON.parse(result.stdout);
    const release = join(setup.sealedRoot, output.release_id);
    const inventory = JSON.parse(readFileSync(join(release, "inventory.json"), "utf8"));
    assert.deepEqual(inventory.slides, { status: "deferred", source_slide_assets: 1 });
    assert.equal(existsSync(join(release, "slides")), false);
    assert.deepEqual(readFileSync(join(release, "public", "materials", "outline.pptx")), pptx);
    assert.equal(readFileSync(setup.publicSentinel, "utf8"), "served sentinel");
    assert.equal(readFileSync(setup.studySentinel, "utf8"), "study sentinel");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});
