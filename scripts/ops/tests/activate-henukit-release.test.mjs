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
const releaseSha = "a".repeat(40);

function executable(path, contents) {
  writeFileSync(path, contents, { mode: 0o755 });
  chmodSync(path, 0o755);
}

function fixture({
  blockerState = "closed",
  gatewayDeploymentFails = false,
  preparationFails = false,
} = {}) {
  const root = mkdtempSync(join(tmpdir(), "activate-henukit-release-"));
  const bin = join(root, "bin");
  const state = join(root, "state");
  const log = join(root, "calls.log");
  const watcher = join(bin, "watcher");
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

  executable(
    join(bin, "gh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'gh %s\n' "$*" >> "$FAKE_CALL_LOG"
printf '%s\n' "$FAKE_BLOCKER_STATE"
`,
  );
  executable(
    watcher,
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'watcher %s\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$FAKE_PREPARATION_FAILS" == "1" ]]; then
  printf 'Account production-boundary manifest did not pass\n' >&2
  exit 1
fi
approval="$HENUKIT_STATE_ROOT/approvals/$FAKE_RELEASE_SHA"
if [[ -f "$approval" ]]; then
  rm "$approval"
  printf '%s\n' "$FAKE_RELEASE_SHA" > "$HENUKIT_STATE_ROOT/last-activated-sha"
else
  printf '/verified/backup.dump\n' > "$HENUKIT_STATE_ROOT/prepared/$FAKE_RELEASE_SHA"
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
elif [[ "$*" == *"deploy-epay-gateway-patches.sh"* && "$FAKE_GATEWAY_DEPLOYMENT_FAILS" == "1" ]]; then
  exit 1
else
  cat >/dev/null || true
fi
`,
  );

  return {
    root,
    state,
    log,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      FAKE_BLOCKER_STATE: blockerState,
      FAKE_CALL_LOG: log,
      FAKE_GATEWAY_DEPLOYMENT_FAILS: gatewayDeploymentFails ? "1" : "0",
      FAKE_PREPARATION_FAILS: preparationFails ? "1" : "0",
      FAKE_RELEASE_SHA: releaseSha,
      HENUKIT_STATE_ROOT: state,
      HENUKIT_RELEASE_ROOT: releases,
      HENUKIT_WATCHER: watcher,
    },
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
  assert.equal((readFileSync(setup.log, "utf8").match(/^watcher /gm) ?? []).length, 2);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /ssh root@metaview\.top .*deploy-epay-gateway-patches\.sh.*--execute/);
  assert.ok(calls.indexOf("deploy-epay-gateway-patches.sh") < calls.lastIndexOf("watcher --once"));
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
});
