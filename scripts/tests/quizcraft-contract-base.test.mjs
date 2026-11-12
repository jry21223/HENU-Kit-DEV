import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repo = fileURLToPath(new URL("../../", import.meta.url));
// Exact PR #653 base that first contained the external TrainingAuthoring ref.
const regressionBase = "928c0edb69c15beaecee84d418574d2cab33fda8";
const openapi = "packages/api-contracts/openapi";
const snapshot = path.join(repo, "scripts/snapshot-openapi-base.sh");
const check = (base, candidate) => {
  const result = spawnSync("go", [
  "run", "github.com/oasdiff/oasdiff@v1.23.0", "breaking", "--fail-on", "ERR",
  "--unmatch-path", "^/auth/(login|callback)$", base, candidate,
], { cwd: path.join(repo, "products/quizcraft/go-service"), encoding: "utf8", timeout: 120_000 });
  assert.ifError(result.error);
  return result;
};

function temporary(t) {
  const directory = mkdtempSync(path.join(os.tmpdir(), "quizcraft-base-test."));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  return directory;
}

test("exact #653 base resolves its external training schema for the real breaking check", t => {
  const directory = temporary(t);
  execFileSync("bash", [snapshot, regressionBase, directory], { cwd: repo });
  const base = path.join(directory, openapi, "quizcraft.yaml");
  const result = check(base, base);
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.equal(readFileSync(path.join(directory, openapi, "quizcraft-training.yaml"), "utf8"),
    execFileSync("git", ["show", `${regressionBase}:${openapi}/quizcraft-training.yaml`], { cwd: repo, encoding: "utf8" }));
});

test("the complete exact-base snapshot still rejects a real removed API", t => {
  const base = temporary(t);
  execFileSync("bash", [snapshot, regressionBase, base], { cwd: repo });
  const candidate = temporary(t);
  cpSync(path.join(base, openapi), path.join(candidate, "openapi"), { recursive: true });
  const candidateSpec = path.join(candidate, "openapi/quizcraft.yaml");
  const source = readFileSync(candidateSpec, "utf8");
  const start = source.indexOf("  /api/v1/banks:\n");
  const end = source.indexOf("\n  /", start + 1);
  assert.ok(start > 0 && end > start, "known public bank route fixture must exist");
  writeFileSync(candidateSpec, source.slice(0, start) + source.slice(end + 1));
  const result = check(path.join(base, openapi, "quizcraft.yaml"), candidateSpec);
  assert.notEqual(result.status, 0, "breaking removal must still fail");
  assert.match(result.stdout + result.stderr, /api-path-removed-without-deprecation/);
  assert.match(result.stdout + result.stderr, /api\/v1\/banks/);
});

test("snapshot uses the requested commit even after checkout and branch advance", t => {
  const fixture = temporary(t);
  execFileSync("bash", [snapshot, regressionBase, fixture], { cwd: repo });
  execFileSync("git", ["init", "--quiet"], { cwd: fixture });
  execFileSync("git", ["add", "."], { cwd: fixture });
  const commit = message => execFileSync("git", ["-c", "user.name=Contract Test", "-c", "user.email=contract-test@example.invalid", "commit", "--quiet", "-m", message], { cwd: fixture });
  commit("base OpenAPI fixture");
  const exactBase = execFileSync("git", ["rev-parse", "HEAD"], { cwd: fixture, encoding: "utf8" }).trim();
  const trainingPath = path.join(fixture, openapi, "quizcraft-training.yaml");
  const original = readFileSync(trainingPath, "utf8");
  writeFileSync(trainingPath, original.replace("QuizCraft Training Owner API", "Advanced branch edition"));
  execFileSync("git", ["add", "."], { cwd: fixture });
  commit("advance the candidate branch");
  writeFileSync(trainingPath, "candidate working-copy data must not enter the base snapshot\n");
  const output = temporary(t);
  execFileSync("bash", [snapshot, exactBase, output], { cwd: fixture });
  assert.equal(readFileSync(path.join(output, openapi, "quizcraft-training.yaml"), "utf8"), original);
});

test("symbolic refs, missing commits and nonempty destinations fail closed", t => {
  const directory = temporary(t);
  for (const base of ["HEAD", "0".repeat(40)]) {
    const result = spawnSync("bash", [snapshot, base, directory], { cwd: repo, encoding: "utf8" });
    assert.ifError(result.error);
    assert.notEqual(result.status, 0);
  }
  writeFileSync(path.join(directory, "candidate.yaml"), "candidate data");
  const result = spawnSync("bash", [snapshot, regressionBase, directory], { cwd: repo, encoding: "utf8" });
  assert.ifError(result.error);
  assert.notEqual(result.status, 0);
  assert.equal(readFileSync(path.join(directory, "candidate.yaml"), "utf8"), "candidate data");
});
