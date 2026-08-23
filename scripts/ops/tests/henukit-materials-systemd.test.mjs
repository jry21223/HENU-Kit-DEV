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

test("materials unit templates keep the receiver unprivileged and confine the privileged fixed-flow runner", () => {
  const receiver = read("systemd/henukit-materials-webhook.service");
  const runner = read("systemd/henukit-materials-runner.service");
  const path = read("systemd/henukit-materials-webhook.path");
  const preparationWrapper = read("henukit-materials-prepare");

  assert.match(receiver, /^User=henukit-deploy$/m);
  assert.match(receiver, /^Group=henukit-deploy$/m);
  assert.match(receiver, /^ExecStart=\/usr\/local\/bin\/henukit-deploy-webhook materials-serve$/m);
  assert.doesNotMatch(
    runner,
    /^User=/m,
    "the system runner must use the service-manager root default so PrivateDevices and NoNewPrivileges retain CAP_SETUID for the intentional preparation drop",
  );
  assert.doesNotMatch(runner, /^Group=/m, "the root-default runner does not need a redundant group directive");
  assert.match(runner, /^Environment=HOME=\/root$/m);
  assert.match(runner, /^ExecStart=\/usr\/local\/bin\/henukit-deploy-webhook materials-run$/m);
  assert.doesNotMatch(runner, /^StateDirectory=/m, "root runner must not take ownership away from the receiver");
  assert.match(runner, /^NoNewPrivileges=yes$/m);
  assert.match(
    runner,
    /^RestrictSUIDSGID=yes$/m,
    "the root runner must retain SUID/SGID hardening after the credential-path fix",
  );
  assert.match(runner, /^ProtectSystem=strict$/m);
  assert.match(runner, /^ReadWritePaths=\/var\/lib\/henukit-materials-webhook \/opt\/henukit-materials$/m);
  assert.doesNotMatch(receiver, /(?:henukit-materials-seal|docker|psql|\/opt\/henukit-materials|HENUKIT_MATERIALS_DATABASE_URL|HENUKIT_MATERIALS_PUBLIC_ROOT)/i);
  assert.doesNotMatch(preparationWrapper, /(?:henukit-materials-seal|docker|psql|\/opt\/henukit-materials|HENUKIT_MATERIALS_DATABASE_URL|HENUKIT_MATERIALS_PUBLIC_ROOT)/i);
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
