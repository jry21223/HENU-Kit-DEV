import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const workflow = readFileSync(join(repositoryRoot, ".github", "workflows", "deploy-webhook.yml"), "utf8");
const sealLinuxTest = readFileSync(join(repositoryRoot, "scripts", "ops", "tests", "henukit-materials-seal-linux.test.mjs"), "utf8");

test("deploy-webhook CI gates materials preparation, sealing, and unit-template boundaries", () => {
  for (const path of [
    "scripts/ops/prepare-henukit-materials.mjs",
    "scripts/ops/seal-henukit-materials.mjs",
    "scripts/ops/tests/prepare-henukit-materials.test.mjs",
    "scripts/ops/tests/henukit-materials-prepare-wrapper.test.mjs",
    "scripts/ops/tests/henukit-materials-systemd.test.mjs",
    "scripts/ops/tests/henukit-materials-seal.test.mjs",
    "scripts/ops/tests/henukit-materials-seal-wrapper.test.mjs",
    "scripts/ops/tests/henukit-materials-seal-linux.test.mjs",
    "docs/adr/0023-materials-latest-arrival-queue.md",
    "docs/adr/0024-materials-sealed-release-boundary.md",
    "docs/development/306-materials-secure-preparation.md",
  ]) {
    assert.match(workflow, new RegExp(`- ${path.replaceAll(".", "\\.")}$`, "m"), path);
  }
  assert.match(workflow, /uses: actions\/setup-node@v4/);
  assert.match(workflow, /node --check scripts\/ops\/prepare-henukit-materials\.mjs/);
  assert.match(workflow, /node --check scripts\/ops\/seal-henukit-materials\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/prepare-henukit-materials\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-prepare-wrapper\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-systemd\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-seal\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-seal-wrapper\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-seal-linux\.test\.mjs/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-prepare/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-seal/);
  assert.match(workflow, /sh -n services\/deploy-webhook\/deploy\/henukit-materials-seal/);
  assert.match(sealLinuxTest, /process\.env\.CI === "true" && !dockerAvailable/);
  assert.match(sealLinuxTest, /Docker is required to verify root-owned materials sealing in CI/);
});
