import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  mkdtempSync,
  mkdirSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const gateSource = fileURLToPath(
  new URL("../oauth-continuation-release-gate.sh", import.meta.url),
);

function git(root, args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
}

function createGateRepository() {
  const root = mkdtempSync(join(tmpdir(), "henukit-oauth-gate-repo-"));
  const ops = join(root, "scripts", "ops");
  mkdirSync(ops, { recursive: true });
  const gate = join(ops, "oauth-continuation-release-gate.sh");
  copyFileSync(gateSource, gate);
  chmodSync(gate, 0o755);
  writeFileSync(join(root, "package.json"), '{"private":true}\n');
  git(root, ["init"]);
  git(root, ["config", "user.name", "HENU Kit Test"]);
  git(root, ["config", "user.email", "test@henukit.invalid"]);
  git(root, ["add", "."]);
  git(root, ["commit", "-m", "fixture"]);
  const releaseSha = git(root, ["rev-parse", "HEAD"]);
  const sourceTree = git(root, ["rev-parse", `${releaseSha}^{tree}`]);
  return { gate, releaseSha, root, sourceTree };
}

function receipt(releaseSha, sourceTree) {
  return `format=henukit-oauth-continuation-gate-v1\nrelease_sha=${releaseSha}\nsource_tree=${sourceTree}\nresult=pass\n`;
}

test("the OAuth release receipt is bound to the exact clean checkout", () => {
  const { gate, releaseSha, root, sourceTree } = createGateRepository();
  const valid = join(root, "valid.env");
  writeFileSync(valid, receipt(releaseSha, sourceTree));
  execFileSync(gate, ["verify", "--sha", releaseSha, "--receipt", valid]);

  const untrackedPath = join(root, "untracked-source.go");
  writeFileSync(untrackedPath, "package injected\n");
  const untracked = spawnSync(
    gate,
    ["verify", "--sha", releaseSha, "--receipt", valid],
    { encoding: "utf8" },
  );
  assert.notEqual(untracked.status, 0);
  assert.match(untracked.stderr, /unexpected untracked source file/);
  unlinkSync(untrackedPath);

  writeFileSync(join(root, "package.json"), '{"private":false}\n');
  const dirty = spawnSync(
    gate,
    ["verify", "--sha", releaseSha, "--receipt", valid],
    { encoding: "utf8" },
  );
  assert.notEqual(dirty.status, 0);
  assert.match(dirty.stderr, /tracked files do not match requested SHA/);

  writeFileSync(join(root, "package.json"), '{"private":true}\n');
  writeFileSync(join(root, "tracked.txt"), "new clean commit\n");
  git(root, ["add", "tracked.txt"]);
  git(root, ["commit", "-m", "move head"]);
  const wrongHead = spawnSync(
    gate,
    ["verify", "--sha", releaseSha, "--receipt", valid],
    { encoding: "utf8" },
  );
  assert.notEqual(wrongHead.status, 0);
  assert.match(wrongHead.stderr, /checkout HEAD does not match requested SHA/);
});

test("the OAuth release receipt rejects a wrong tree and symlinks", () => {
  const { gate, releaseSha, root, sourceTree } = createGateRepository();
  const valid = join(root, "valid.env");
  writeFileSync(valid, receipt(releaseSha, sourceTree));

  const wrongTree = join(root, "wrong-tree.env");
  writeFileSync(wrongTree, receipt(releaseSha, "b".repeat(40)));
  const rejected = spawnSync(
    gate,
    ["verify", "--sha", releaseSha, "--receipt", wrongTree],
    { encoding: "utf8" },
  );
  assert.notEqual(rejected.status, 0);
  assert.match(rejected.stderr, /does not match requested SHA and source tree/);

  const linked = join(root, "linked.env");
  symlinkSync(valid, linked);
  const linkedResult = spawnSync(
    gate,
    ["verify", "--sha", releaseSha, "--receipt", linked],
    { encoding: "utf8" },
  );
  assert.notEqual(linkedResult.status, 0);
  assert.match(linkedResult.stderr, /regular non-symlink/);
});
