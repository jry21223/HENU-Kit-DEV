import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const command = fileURLToPath(
  new URL("../activate-henukit-release.sh", import.meta.url),
);
const runbook = readFileSync(
  new URL("../../../docs/operations/henukit-artifact-deployment.md", import.meta.url),
  "utf8",
);
const releaseSha = "a".repeat(40);

function executable(path, contents) {
  writeFileSync(path, contents, { mode: 0o755 });
  chmodSync(path, 0o755);
}

function fixture({
  blockerState = "closed",
  branchSha = releaseSha,
  gatewayDeploymentFails = false,
  metadataDuplicate = false,
  metadataReleaseSha = releaseSha,
  metadataSymlink = false,
  platformMigrations,
  preparationFails = false,
  runConclusion = "success",
} = {}) {
  const root = mkdtempSync(join(tmpdir(), "activate-henukit-release-"));
  const bin = join(root, "bin");
  const state = join(root, "state");
  const log = join(root, "calls.log");
  const watcher = join(bin, "watcher");
  const envFile = join(root, "henukit.env");
  const tokenFile = join(root, "github.token");
  mkdirSync(bin);
  mkdirSync(join(state, "approvals"), { recursive: true });
  mkdirSync(join(state, "prepared"), { recursive: true });
  const releases = join(root, "releases");
  const release = join(releases, releaseSha);
  mkdirSync(join(release, "bin"), { recursive: true });
  mkdirSync(join(release, "infra", "epay-gateway", "patches"), { recursive: true });
  executable(join(release, "bin", "deploy-epay-gateway-patches.sh"), "#!/usr/bin/env bash\nexit 0\n");
  writeFileSync(join(release, "infra", "epay-gateway", "patches", "0001.patch"), "patch\n");
  writeFileSync(log, "");
  writeFileSync(envFile, "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=0\n", { mode: 0o600 });
  writeFileSync(tokenFile, "test-token\n", { mode: 0o600 });

  executable(
    join(bin, "gh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'gh %s\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$*" == *"/issues/"* ]]; then
  printf '%s\n' "$FAKE_BLOCKER_STATE"
elif [[ "$*" == *"/branches/"* ]]; then
  printf '%s\n' "$FAKE_BRANCH_SHA"
elif [[ "$1 $2" == "run list" ]]; then
  printf '%s\tcompleted\t%s\n' "$FAKE_RELEASE_SHA" "$FAKE_RUN_CONCLUSION"
else
  exit 70
fi
`,
  );
  executable(
    watcher,
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'watcher %s\n' "$*" >> "$FAKE_CALL_LOG"
printf 'migrations %s\n' "\${HENUKIT_PLATFORM_MIGRATIONS:-}" >> "$FAKE_CALL_LOG"
if [[ "$FAKE_PREPARATION_FAILS" == "1" ]]; then
  printf 'Account production-boundary manifest did not pass\n' >&2
  exit 1
fi
approval="$HENUKIT_STATE_ROOT/approvals/$FAKE_RELEASE_SHA"
if [[ -f "$approval" ]]; then
  rm "$approval"
  printf '%s\n' "$FAKE_RELEASE_SHA" > "$HENUKIT_STATE_ROOT/last-activated-sha"
else
  backup="$HENUKIT_STATE_ROOT/platform-backup-\${FAKE_RELEASE_SHA:0:12}.dump"
  printf 'verified backup\n' > "$backup"
  if [[ "$FAKE_METADATA_SYMLINK" == "1" ]]; then
    printf 'release_sha=%s\n' "$FAKE_METADATA_RELEASE_SHA" > "$backup.metadata-target"
    ln -s "$backup.metadata-target" "$backup.meta"
  else
    printf 'release_sha=%s\n' "$FAKE_METADATA_RELEASE_SHA" > "$backup.meta"
    if [[ "$FAKE_METADATA_DUPLICATE" == "1" ]]; then
      printf 'release_sha=%s\n' "$FAKE_RELEASE_SHA" >> "$backup.meta"
    fi
  fi
  printf '%s\n' "$backup" > "$HENUKIT_STATE_ROOT/prepared/$FAKE_RELEASE_SHA"
fi
`,
  );
  executable(
    join(bin, "ssh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'ssh %s\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$*" == *"mktemp -d"* ]]; then
  printf '/tmp/henukit-epay-release.test\n'
elif [[ "$*" == *"bash -s -- /root/epay-gateway"* ]]; then
  cat >/dev/null
  printf 'henukit-production\nhenukit-production-secret-32bytes\n'
elif [[ "$*" == *"deploy-epay-gateway-patches.sh"* && "$FAKE_GATEWAY_DEPLOYMENT_FAILS" == "1" ]]; then
  exit 1
else
  cat >/dev/null || true
fi
`,
  );

  const environment = {
    ...process.env,
    PATH: `${bin}:${process.env.PATH}`,
    FAKE_BLOCKER_STATE: blockerState,
    FAKE_BRANCH_SHA: branchSha,
    FAKE_CALL_LOG: log,
    FAKE_GATEWAY_DEPLOYMENT_FAILS: gatewayDeploymentFails ? "1" : "0",
    FAKE_METADATA_DUPLICATE: metadataDuplicate ? "1" : "0",
    FAKE_METADATA_RELEASE_SHA: metadataReleaseSha,
    FAKE_METADATA_SYMLINK: metadataSymlink ? "1" : "0",
    FAKE_PREPARATION_FAILS: preparationFails ? "1" : "0",
    FAKE_RELEASE_SHA: releaseSha,
    FAKE_RUN_CONCLUSION: runConclusion,
    HENUKIT_STATE_ROOT: state,
    HENUKIT_RELEASE_ROOT: releases,
    HENUKIT_ENV_FILE: envFile,
    GH_TOKEN_FILE: tokenFile,
    HENUKIT_WATCHER: watcher,
  };
  delete environment.HENUKIT_PLATFORM_MIGRATIONS;
  if (platformMigrations !== undefined) {
    environment.HENUKIT_PLATFORM_MIGRATIONS = platformMigrations;
  }

  return {
    root,
    state,
    log,
    env: environment,
  };
}

test("one command prepares, exact-SHA approves, and activates a release", () => {
  const setup = fixture();

  const output = execFileSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.match(output, new RegExp(`release ${releaseSha} activated`));
  assert.equal(
    readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(),
    releaseSha,
  );
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.match(readFileSync(join(setup.root, "henukit.env"), "utf8"), /EASYPAY_ENABLED=1/);
  assert.equal((readFileSync(setup.log, "utf8").match(/^watcher /gm) ?? []).length, 2);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /ssh root@metaview\.top .*deploy-epay-gateway-patches\.sh.*--execute/);
  assert.ok(calls.indexOf("deploy-epay-gateway-patches.sh") < calls.lastIndexOf("watcher --once"));
  assert.equal((calls.match(/gh api .*\/branches\//g) ?? []).length, 3);
  assert.equal((calls.match(/^gh run list/gm) ?? []).length, 3);
  assert.match(
    calls,
    /migrations 000014_account_portfolio_ticket_access\.up\.sql,000015_account_portfolio_membership_access\.up\.sql,000016_account_portfolio_points_access\.up\.sql,000017_account_portfolio_order_access\.up\.sql,000018_account_operator_role_grant_audit\.up\.sql,000019_operations_operator_role\.up\.sql/,
  );
});

test("the one-command runbook keeps migrations owned by the activation default", () => {
  const section = runbook
    .split("### One-command Account payment release")[1]
    .split("### ")[0];

  assert.doesNotMatch(section, /HENUKIT_PLATFORM_MIGRATIONS=/);
  assert.match(section, /`000014` through `000019`/);
  assert.match(section, /grants it\s+to no user/);
});

test("one command preserves an explicit reviewed migration override", () => {
  const setup = fixture({ platformMigrations: "000017_account_portfolio_order_access.up.sql" });

  execFileSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  const calls = readFileSync(setup.log, "utf8");
  assert.equal(
    (calls.match(/^migrations 000017_account_portfolio_order_access\.up\.sql$/gm) ?? []).length,
    2,
  );
});

test("one command refuses while QuizCraft cutover blocker remains open", () => {
  const setup = fixture({ blockerState: "open" });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /blocker issue #166 must be closed/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher /m);
});

test("one command never creates approval when preparation or mock gates fail", () => {
  const setup = fixture({ preparationFails: true });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.equal((readFileSync(setup.log, "utf8").match(/^watcher /gm) ?? []).length, 1);
  assert.match(readFileSync(join(setup.root, "henukit.env"), "utf8"), /EASYPAY_ENABLED=0/);
});

test("one command rejects prepared metadata bound to another full SHA", () => {
  const setup = fixture({ metadataReleaseSha: `${releaseSha.slice(0, 12)}${"b".repeat(28)}` });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /backup evidence is not bound/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /deploy-epay-gateway-patches/);
});

test("one command rejects symlinked prepared backup metadata", () => {
  const setup = fixture({ metadataSymlink: true });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /no readable metadata/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /deploy-epay-gateway-patches/);
});

test("one command rejects duplicate release SHA metadata", () => {
  const setup = fixture({ metadataDuplicate: true });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /exactly one release SHA/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /deploy-epay-gateway-patches/);
});

test("one command never creates approval when the EasyPay gateway patch fails", () => {
  const setup = fixture({ gatewayDeploymentFails: true });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.equal((readFileSync(setup.log, "utf8").match(/^watcher /gm) ?? []).length, 1);
  assert.match(readFileSync(join(setup.root, "henukit.env"), "utf8"), /EASYPAY_ENABLED=0/);
});

test("one command refuses an old SHA before preparing or modifying MetaView", () => {
  const setup = fixture({ branchSha: "b".repeat(40) });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not the current main head/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher |^ssh /m);
});

test("one command refuses a failed newest workflow before preparing or modifying MetaView", () => {
  const setup = fixture({ runConclusion: "failure" });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not the newest completed successful/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher |^ssh /m);
});
