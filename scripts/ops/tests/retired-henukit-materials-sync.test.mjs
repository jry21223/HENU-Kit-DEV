import assert from "node:assert/strict";
import { existsSync, mkdtempSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const scripts = [
  new URL("../sync-henukit-materials.sh", import.meta.url).pathname,
  new URL("../henukit-materials-sync.sh", import.meta.url).pathname,
];

for (const script of scripts) {
  test(`${script.split("/").at(-1)} is a fail-closed retirement shim`, () => {
    const temporary = mkdtempSync(join(tmpdir(), "retired-materials-sync-"));
    const root = join(temporary, "must-not-be-created");
    const result = spawnSync("/bin/sh", [script], {
      encoding: "utf8",
      env: {
        ...process.env,
        HENUKIT_MATERIALS_ROOT: root,
        HENUKIT_MATERIALS_REPO_URL: `file://${join(temporary, "missing-source")}`,
      },
    });

    assert.equal(result.status, 64, result.stderr);
    assert.match(result.stderr, /retired.*canonical.*prepare.*seal.*activate/i);
    assert.equal(existsSync(root), false, "retired sync touched its configured root");
  });
}
