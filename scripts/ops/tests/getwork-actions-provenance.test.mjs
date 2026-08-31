import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const manifestBuilder = fileURLToPath(
  new URL("../create-getwork-actions-manifest.sh", import.meta.url),
);

function sha256(content) {
  return createHash("sha256").update(content).digest("hex");
}

test("the Actions handoff manifest binds only the exact getWork image and runtime", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-getwork-actions-manifest-"));
  const releaseSha = "a".repeat(40);
  const imageName = `henukit-getwork-mcp-${releaseSha}.docker.tar.gz`;
  const runtimeName = `henukit-runtime-${releaseSha}.tar.gz`;
  const image = "pinned-getwork-image\n";
  const runtime = "pinned-runtime\n";
  const output = join(root, `henukit-getwork-actions-${releaseSha}.manifest`);

  writeFileSync(join(root, imageName), image);
  writeFileSync(join(root, `${imageName}.sha256`), `${sha256(image)}  ${imageName}\n`);
  writeFileSync(join(root, runtimeName), runtime);
  writeFileSync(join(root, `${runtimeName}.sha256`), `${sha256(runtime)}  ${runtimeName}\n`);

  execFileSync(manifestBuilder, [
    "--sha",
    releaseSha,
    "--artifact-dir",
    root,
    "--output",
    output,
  ]);

  assert.equal(
    readFileSync(output, "utf8"),
    [
      "format=henukit-getwork-actions-release-v1",
      `release_sha=${releaseSha}`,
      "source_repository=jry21223/HENU-Kit-DEV",
      "source_ref=refs/heads/main",
      "signer_workflow=.github/workflows/deploy-henukit.yml",
      "builder_platform=linux/amd64",
      `artifact_sha256=${sha256(image)}  ${imageName}`,
      `artifact_sha256=${sha256(runtime)}  ${runtimeName}`,
      "",
    ].join("\n"),
  );
});

test("the Actions handoff manifest rejects a checksum that does not match its archive", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-getwork-actions-tamper-"));
  const releaseSha = "b".repeat(40);
  const imageName = `henukit-getwork-mcp-${releaseSha}.docker.tar.gz`;
  const runtimeName = `henukit-runtime-${releaseSha}.tar.gz`;
  const output = join(root, `henukit-getwork-actions-${releaseSha}.manifest`);

  writeFileSync(join(root, imageName), "tampered-image\n");
  writeFileSync(join(root, `${imageName}.sha256`), `${"0".repeat(64)}  ${imageName}\n`);
  writeFileSync(join(root, runtimeName), "pinned-runtime\n");
  writeFileSync(
    join(root, `${runtimeName}.sha256`),
    `${sha256("pinned-runtime\n")}  ${runtimeName}\n`,
  );

  const result = spawnSync(manifestBuilder, [
    "--sha",
    releaseSha,
    "--artifact-dir",
    root,
    "--output",
    output,
  ], { encoding: "utf8" });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /checksum (?:does not bind|verification failed)/);
});
