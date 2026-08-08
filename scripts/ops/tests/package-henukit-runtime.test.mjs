import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { chmodSync, existsSync, mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const packager = fileURLToPath(
  new URL("../package-henukit-runtime.sh", import.meta.url),
);
const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const releaseSha = "a".repeat(40);

test("the shared runtime packager produces the same fixed-SHA operator payload for local and Actions builds", () => {
  const outputDirectory = mkdtempSync(join(tmpdir(), "henukit-runtime-package-"));
  const binDirectory = join(outputDirectory, "bin");
  const runtimeArchive = join(outputDirectory, `henukit-runtime-${releaseSha}.tar.gz`);
  mkdirSync(binDirectory);
  const docker = join(binDirectory, "docker");
  writeFileSync(
    docker,
    "#!/usr/bin/env bash\nset -Eeuo pipefail\n[[ \"$1\" == \"compose\" ]]\nprintf 'services:\\n  portal:\\n    image: henukit-portal:test\\n'\n",
    { mode: 0o755 },
  );
  chmodSync(docker, 0o755);

  execFileSync(packager, ["--sha", releaseSha, "--output-dir", outputDirectory], {
    cwd: repoRoot,
    env: { ...process.env, PATH: `${binDirectory}:${process.env.PATH}` },
    stdio: "pipe",
  });

  assert.equal(existsSync(runtimeArchive), true);
  assert.equal(existsSync(`${runtimeArchive}.sha256`), true);
  const files = execFileSync("tar", ["-tzf", runtimeArchive], { encoding: "utf8" });
  for (const file of [
    "./RELEASE_SHA",
    "./bin/deploy-henukit-artifact.sh",
    "./bin/watch-henukit-actions.sh",
    "./bin/henukit-release-images.sh",
    "./bin/verify-henukit-local-release.sh",
    "./docker-compose.henukit.release.yml",
    "./release-gates/account-production-boundary.env",
  ]) {
    assert.match(files, new RegExp(`^${file.replaceAll(".", "\\.").replaceAll("/", "\\/")}$`, "m"));
  }
  assert.equal(
    execFileSync("tar", ["-xOzf", runtimeArchive, "./RELEASE_SHA"], { encoding: "utf8" }).trim(),
    releaseSha,
  );
});
