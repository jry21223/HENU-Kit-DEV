import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { chmodSync, existsSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const packager = fileURLToPath(
  new URL("../package-henukit-runtime.sh", import.meta.url),
);
const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const releaseSha = "a".repeat(40);

test("the materials mount stays an explicit bind when runtime packaging disables interpolation", () => {
  const compose = readFileSync(join(repoRoot, "docker-compose.henukit.yml"), "utf8");
  assert.match(
    compose,
    /- type: bind\n\s+source: \$\{HENUKIT_MATERIALS_PUBLIC_ROOT:-\/opt\/henukit-materials\/public\}\n\s+target: \/srv\/materials\n\s+read_only: true/,
  );
});

test("runtime binaries omit VCS metadata so unchanged materials remain rollback-compatible", () => {
  const source = readFileSync(packager, "utf8");
  const buildLines = source.split("\n").filter((line) => line.trim().startsWith("go build "));
  assert.equal(buildLines.length, 5);
  for (const line of buildLines) {
    assert.match(line, /go build -buildvcs=false -trimpath /);
  }
});

test("the shared runtime packager produces the same fixed-SHA operator payload for local and Actions builds", () => {
  const outputDirectory = mkdtempSync(join(tmpdir(), "henukit-runtime-package-"));
  const binDirectory = join(outputDirectory, "bin");
  const runtimeArchive = join(outputDirectory, `henukit-runtime-${releaseSha}.tar.gz`);
  mkdirSync(binDirectory);
  const docker = join(binDirectory, "docker");
  writeFileSync(
    docker,
    "#!/usr/bin/env bash\nset -Eeuo pipefail\nif [[ \"$1\" == \"compose\" ]]; then printf 'services:\\n  portal:\\n    image: henukit-portal:test\\n'; exit 0; fi\n[[ \"$1\" == \"run\" ]]\noutput=\nhost_output=\nwhile [[ $# -gt 0 ]]; do\n  if [[ \"$1\" == \"--volume\" && \"$2\" == *\":/out\" ]]; then output=\"${2%:/out}\"; fi\n  if [[ \"$1\" == \"--volume\" && \"$2\" == *\":/host-out\" ]]; then host_output=\"${2%:/host-out}\"; fi\n  shift\ndone\n[[ -n \"$output\" && -n \"$host_output\" ]]\nfor name in henukit-deploy-webhook materials-oss-canary materials-oss-release library-activate-public-release; do printf '#!/bin/sh\\nexit 0\\n' > \"$output/$name\"; chmod 0755 \"$output/$name\"; done\nprintf '#!/bin/sh\\nexit 0\\n' > \"$host_output/food-sanitize-post-image\"\nchmod 0755 \"$host_output/food-sanitize-post-image\"\n",
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
    "./bin/adopt-henukit-degraded-baseline.sh",
    "./bin/rotate-henukit-release-signers.sh",
    "./bin/henukit-release-images.sh",
    "./bin/verify-henukit-local-release.sh",
    "./bin/food-sanitize-post-image",
    "./bin/import-legacy-portal-food-images.mjs",
    "./docker-compose.henukit.release.yml",
    "./release-gates/account-production-boundary.env",
    "./materials-runtime/install.sh",
    "./materials-runtime/SHA256SUMS",
    "./materials-runtime/bin/henukit-deploy-webhook",
    "./materials-runtime/bin/materials-oss-canary",
    "./materials-runtime/bin/materials-oss-release",
    "./materials-runtime/bin/library-activate-public-release",
    "./materials-runtime/libexec/henukit-materials-activate",
    "./materials-runtime/libexec/activate-henukit-materials.mjs",
    "./materials-runtime/systemd/henukit-materials-webhook.service",
    "./materials-runtime/systemd/henukit-materials-webhook.path",
    "./materials-runtime/systemd/henukit-materials-runner.service",
  ]) {
    assert.match(files, new RegExp(`^${file.replaceAll(".", "\\.").replaceAll("/", "\\/")}$`, "m"));
  }
  assert.doesNotMatch(files, /convert-henukit-slides|import-henukit-materials|migrations\/study/);
  const materialsChecksums = execFileSync(
    "tar",
    ["-xOzf", runtimeArchive, "./materials-runtime/SHA256SUMS"],
    { encoding: "utf8" },
  );
  for (const path of [
    "bin/henukit-deploy-webhook",
    "bin/materials-oss-release",
    "install.sh",
  ]) {
    assert.match(materialsChecksums, new RegExp(`^[0-9a-f]{64}  ${path.replaceAll(".", "\\.")}$`, "m"));
  }
  const materialsInstaller = execFileSync(
    "tar",
    ["-xOzf", runtimeArchive, "./materials-runtime/install.sh"],
    { encoding: "utf8" },
  );
  assert.match(materialsInstaller, /retire_root_file "\/usr\/local\/libexec\/henukit\/import-henukit-materials\.mjs" "Study importer"/);
  assert.match(materialsInstaller, /retire_root_file "\/etc\/henukit-deploy\/materials-postgresql\.conf" "Study PostgreSQL credential" "600"/);
  assert.doesNotMatch(materialsInstaller, /pg_dump|pg_restore|migrations\/study/);
  assert.equal(
    execFileSync("tar", ["-xOzf", runtimeArchive, "./RELEASE_SHA"], { encoding: "utf8" }).trim(),
    releaseSha,
  );
});
