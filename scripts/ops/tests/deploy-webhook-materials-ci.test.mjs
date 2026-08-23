import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const workflow = readFileSync(join(repositoryRoot, ".github", "workflows", "deploy-webhook.yml"), "utf8");
const sealLinuxTest = readFileSync(join(repositoryRoot, "scripts", "ops", "tests", "henukit-materials-seal-linux.test.mjs"), "utf8");
const server = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "cmd", "server", "main.go"), "utf8");
const installer = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "install.sh"), "utf8");
const runtimeInstaller = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "install-materials-runtime.sh"), "utf8");
const runtimePackager = readFileSync(join(repositoryRoot, "scripts", "ops", "package-henukit-runtime.sh"), "utf8");
const orchestrator = readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "henukit-materials-orchestrate"), "utf8");
const materialsUnits = [
  "henukit-materials-runner.service",
  "henukit-materials-webhook.path",
  "henukit-materials-webhook.service",
].map((name) => readFileSync(join(repositoryRoot, "services", "deploy-webhook", "deploy", "systemd", name), "utf8")).join("\n");

function goFunction(name) {
  const start = server.indexOf(`func ${name}(`);
  const end = server.indexOf("\nfunc ", start + 1);
  return server.slice(start, end === -1 ? undefined : end);
}

test("deploy-webhook CI gates the complete materials release boundary", () => {
  for (const path of [
    "docker-compose.henukit.yml",
    "infra/nginx/henukit.conf.example",
    "scripts/ops/prepare-henukit-materials.mjs",
    "scripts/ops/seal-henukit-materials.mjs",
    "scripts/ops/activate-henukit-materials.mjs",
    "scripts/ops/sync-henukit-materials.sh",
    "scripts/ops/henukit-materials-sync.sh",
    "scripts/ops/build-henukit-library-activation-bundle.mjs",
    "services/library/db/legacy-study-migrations/000001_materials_oss_release.up.sql",
    "scripts/ops/import-henukit-materials.mjs",
    "scripts/ops/tests/prepare-henukit-materials.test.mjs",
    "scripts/ops/tests/henukit-materials-prepare-wrapper.test.mjs",
    "scripts/ops/tests/henukit-materials-systemd.test.mjs",
    "scripts/ops/tests/henukit-materials-seal.test.mjs",
    "scripts/ops/tests/henukit-materials-seal-wrapper.test.mjs",
    "scripts/ops/tests/henukit-materials-seal-linux.test.mjs",
    "scripts/ops/tests/henukit-materials-activate-wrapper.test.mjs",
    "scripts/ops/tests/activate-henukit-materials.test.mjs",
    "scripts/ops/tests/build-henukit-library-activation-bundle.test.mjs",
    "scripts/ops/tests/materials-study-migration.test.mjs",
    "scripts/ops/tests/import-henukit-materials.test.mjs",
    "scripts/ops/tests/henukit-materials-nginx.test.mjs",
    "scripts/ops/tests/henukit-materials-orchestrate.test.mjs",
    "scripts/ops/tests/retired-henukit-materials-sync.test.mjs",
    "scripts/ops/tests/henukit-materials-publish-oss-wrapper.test.mjs",
    "scripts/ops/tests/henukit-materials-publish-release-oss-wrapper.test.mjs",
    "docs/adr/0023-materials-latest-arrival-queue.md",
    "docs/adr/0024-materials-sealed-release-boundary.md",
    "docs/adr/0025-materials-atomic-activation.md",
    "docs/adr/0026-materials-oss-canary-publication.md",
    "docs/adr/0028-materials-complete-oss-release.md",
    "docs/development/306-materials-secure-preparation.md",
    "docs/operations/henukit-materials-oss-canary.md",
    "docs/operations/henukit-materials-oss-release.md",
  ]) {
    assert.match(workflow, new RegExp(`- ${path.replaceAll(".", "\\.")}$`, "m"), path);
  }
  assert.match(workflow, /uses: actions\/setup-node@v4/);
  assert.match(workflow, /node --check scripts\/ops\/prepare-henukit-materials\.mjs/);
  assert.match(workflow, /node --check scripts\/ops\/seal-henukit-materials\.mjs/);
  assert.match(workflow, /node --check scripts\/ops\/activate-henukit-materials\.mjs/);
  assert.match(workflow, /node --check scripts\/ops\/build-henukit-library-activation-bundle\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/prepare-henukit-materials\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-prepare-wrapper\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-systemd\.test\.mjs/);
  assert.match(workflow, /Verify materials runner can drop preparation privileges/);
  assert.match(workflow, /ExecStart=\/usr\/sbin\/runuser -u henukit-deploy -- \/usr\/bin\/id -u/);
  assert.match(workflow, /systemctl start "\$unit"/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-seal\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-seal-wrapper\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-seal-linux\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-activate-wrapper\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/activate-henukit-materials\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/build-henukit-library-activation-bundle\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/materials-study-migration\.test\.mjs/);
  assert.doesNotMatch(workflow, /convert-henukit-slides/);
  assert.equal(
    [...workflow.matchAll(/scripts\/ops\/tests\/import-henukit-materials\.test\.mjs/g)].length,
    3,
    "the importer test must trigger both workflow events and run in the Node suite",
  );
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-nginx\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-orchestrate\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/retired-henukit-materials-sync\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-publish-oss-wrapper\.test\.mjs/);
  assert.match(workflow, /scripts\/ops\/tests\/henukit-materials-publish-release-oss-wrapper\.test\.mjs/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-publish-oss/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-publish-release-oss/);
  assert.match(workflow, /sh -n services\/deploy-webhook\/deploy\/henukit-materials-publish-oss/);
  assert.match(workflow, /sh -n services\/deploy-webhook\/deploy\/henukit-materials-publish-release-oss/);
  assert.match(workflow, /\.\/cmd\/materials-oss-canary/);
  assert.match(workflow, /\.\/cmd\/materials-oss-release/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-prepare/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-seal/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-activate/);
  assert.match(workflow, /services\/deploy-webhook\/deploy\/henukit-materials-orchestrate/);
  assert.match(workflow, /sh -n services\/deploy-webhook\/deploy\/henukit-materials-seal/);
  assert.match(workflow, /sh -n services\/deploy-webhook\/deploy\/henukit-materials-activate/);
  assert.match(workflow, /bash -n services\/deploy-webhook\/deploy\/henukit-materials-orchestrate/);
  assert.match(workflow, /docker compose -f docker-compose\.henukit\.yml config --quiet/);
  assert.match(workflow, /nginx:1\.27-alpine nginx -t/);
  assert.match(
    workflow,
    /sudo "\$\(command -v go\)" test -count=1 \.\/internal\/state -run '\^TestPrivilegedMaterialsConsumer'/,
  );
  assert.match(goFunction("serveMaterials"), /state\.NewMaterialsLatestArrival\(/);
  assert.doesNotMatch(goFunction("serveMaterials"), /PrivilegedConsumer/);
  assert.match(goFunction("runMaterials"), /state\.NewMaterialsLatestArrivalPrivilegedConsumer\(/);
  assert.match(sealLinuxTest, /process\.env\.CI === "true" && !dockerAvailable/);
  assert.match(sealLinuxTest, /Docker is required to verify root-owned materials sealing in CI/);
  assert.match(runtimeInstaller, /henukit-materials-publish-release-oss/);
  assert.match(runtimeInstaller, /materials-oss-release/);
  assert.match(runtimePackager, /materials-oss-release/);
  assert.match(
    installer,
    /--enable-materials-sync is retired; deploy the signed runtime artifact with deploy-henukit-artifact\.sh/,
  );
  assert.doesNotMatch(installer, /materials-oss-canary/);
  assert.doesNotMatch(installer, /if \(\( enable_materials_sync \)\)|go build.*materials/);
  assert.doesNotMatch(orchestrator, /henukit-materials-publish-oss|materials-oss-canary|materials-oss\.env/);
  assert.doesNotMatch(materialsUnits, /henukit-materials-publish-oss|materials-oss-canary|materials-oss\.env/);
});
