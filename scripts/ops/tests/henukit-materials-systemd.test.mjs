import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const deployDir = join(repositoryRoot, "services", "deploy-webhook", "deploy");

function read(relativePath) {
  return readFileSync(join(deployDir, relativePath), "utf8");
}

test("materials unit templates keep receiver and candidate consumer unprivileged and confined", () => {
  const receiver = read("systemd/henukit-materials-webhook.service");
  const runner = read("systemd/henukit-materials-runner.service");
  const path = read("systemd/henukit-materials-webhook.path");

  assert.match(receiver, /^User=henukit-deploy$/m);
  assert.match(receiver, /^Group=henukit-deploy$/m);
  assert.match(receiver, /^ExecStart=\/usr\/local\/bin\/henukit-deploy-webhook materials-serve$/m);
  assert.match(runner, /^User=henukit-deploy$/m);
  assert.match(runner, /^Group=henukit-deploy$/m);
  assert.match(runner, /^Environment=HOME=\/var\/lib\/henukit-deploy$/m);
  assert.match(runner, /^ExecStart=\/usr\/local\/bin\/henukit-deploy-webhook materials-run$/m);
  assert.match(runner, /^StateDirectory=henukit-materials-webhook$/m);
  assert.match(runner, /^StateDirectoryMode=0750$/m);
  assert.match(runner, /^NoNewPrivileges=yes$/m);
  assert.match(runner, /^ProtectSystem=strict$/m);
  assert.match(runner, /^ReadWritePaths=\/var\/lib\/henukit-materials-webhook$/m);
  assert.doesNotMatch(runner, /^(?:User|Group)=root$/m);
  assert.doesNotMatch(runner, /(?:docker|psql|\/opt\/henukit-materials)/i);
  assert.match(path, /^PathExists=\/var\/lib\/henukit-materials-webhook\/queue\/latest\.json$/m);
  assert.match(path, /^PathExists=\/var\/lib\/henukit-materials-webhook\/running\.json$/m);
  assert.doesNotMatch(path, /^PathExistsGlob=/m);
});

test("materials environment binds preparation without legacy deploy or database controls", () => {
  const environment = read("materials.env.example");

  assert.match(environment, /^HENUKIT_MATERIALS_SOURCE_REPOSITORY=https:\/\/github\.com\/jry21223\/HENU-Final-Review\.git$/m);
  assert.match(environment, /^HENUKIT_MATERIALS_CANDIDATE_ROOT=\/var\/lib\/henukit-materials-webhook\/candidates$/m);
  assert.match(environment, /^HENUKIT_MATERIALS_PREPARATION_TIMEOUT=2h$/m);
  assert.doesNotMatch(environment, /HENUKIT_DEPLOY_COMMAND|HENUKIT_MATERIALS_DATABASE_URL|HENUKIT_MATERIALS_ROOT|HENUKIT_MATERIALS_REPO_REF|HENUKIT_MATERIALS_RELEASE_DIR|HENUKIT_MATERIALS_ENV_FILE/);
});
