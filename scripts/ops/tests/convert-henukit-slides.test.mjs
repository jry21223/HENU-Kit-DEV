import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const converter = join(repoRoot, "scripts", "ops", "convert-henukit-slides.py");

test("slide conversion removes its private scratch tree before publication", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-slide-conversion-"));
  const mirror = join(root, "public");
  const output = join(root, "slides");
  const scratch = join(output, ".conversion-tmp");
  const manifest = join(root, "manifest.json");
  mkdirSync(mirror);
  mkdirSync(scratch, { recursive: true });
  writeFileSync(join(scratch, "sentinel"), "private conversion work\n");
  writeFileSync(manifest, '{"version":1,"subjects":[]}\n');

  execFileSync("python3", [converter, "--mirror", mirror, "--out", output, "--manifest", manifest], {
    cwd: repoRoot,
    encoding: "utf8",
  });

  assert.equal(existsSync(scratch), false);
});
