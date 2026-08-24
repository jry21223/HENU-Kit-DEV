import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
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
const recoveryBaselineSha = "c".repeat(40);

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
  preparationFails = false,
  existingApproval = false,
  existingApprovalBaseline = recoveryBaselineSha,
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
  chmodSync(state, 0o700);
  chmodSync(join(state, "approvals"), 0o700);
  chmodSync(join(state, "prepared"), 0o700);
  const releases = join(root, "releases");
  const release = join(releases, releaseSha);
  mkdirSync(join(release, "bin"), { recursive: true });
  mkdirSync(join(release, "infra", "epay-gateway", "patches"), { recursive: true });
  executable(join(release, "bin", "deploy-epay-gateway-patches.sh"), "#!/usr/bin/env bash\nexit 0\n");
  writeFileSync(join(release, "infra", "epay-gateway", "patches", "0001.patch"), "patch\n");
  writeFileSync(log, "");
  writeFileSync(
    envFile,
    "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=0\nQUIZCRAFT_CORE_URL=http://host.docker.internal:10089\n",
    { mode: 0o600 },
  );
  writeFileSync(tokenFile, "test-token\n", { mode: 0o600 });
  if (existingApproval) {
    const backup = join(state, `platform-backup-${releaseSha.slice(0, 12)}.dump`);
    writeFileSync(backup, "verified backup\n", { mode: 0o600 });
    writeFileSync(`${backup}.meta`, `release_sha=${releaseSha}\n`, { mode: 0o600 });
    writeFileSync(join(state, "prepared", releaseSha), `${backup}\n`, { mode: 0o600 });
    writeFileSync(
      join(state, "prepared", `${releaseSha}.recovery-baseline`),
      `${existingApprovalBaseline}\n`,
      { mode: 0o600 },
    );
    writeFileSync(join(state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });
  }

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
  chmod 0600 "$backup" "$backup.meta" "$HENUKIT_STATE_ROOT/prepared/$FAKE_RELEASE_SHA"
fi
`,
  );
  executable(
    join(bin, "adopt-retained-release"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'adopt-retained-release %s\n' "$*" >> "$FAKE_CALL_LOG"
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

  return {
    root,
    state,
    log,
    env: {
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
      HENUKIT_TRUST_ANCHOR: root,
      HENUKIT_RELEASE_ROOT: releases,
      HENUKIT_ENV_FILE: envFile,
      GH_TOKEN_FILE: tokenFile,
      HENUKIT_WATCHER: watcher,
      HENUKIT_RETAINED_RELEASE_ADOPTER: join(bin, "adopt-retained-release"),
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
  assert.match(readFileSync(join(setup.root, "henukit.env"), "utf8"), /EASYPAY_ENABLED=1/);
  assert.match(
    readFileSync(join(setup.root, "henukit.env"), "utf8"),
    /^QUIZCRAFT_CORE_URL=http:\/\/quizcraft:10089$/m,
    "containerized releases must atomically replace the retired host listener URL",
  );
  assert.doesNotMatch(
    readFileSync(join(setup.root, "henukit.env"), "utf8"),
    /host\.docker\.internal:10089/,
  );
  assert.equal((readFileSync(setup.log, "utf8").match(/^watcher /gm) ?? []).length, 2);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /ssh root@metaview\.top .*deploy-epay-gateway-patches\.sh.*--execute/);
  assert.ok(calls.indexOf("deploy-epay-gateway-patches.sh") < calls.lastIndexOf("watcher --once"));
  assert.equal((calls.match(/gh api .*\/branches\//g) ?? []).length, 3);
  assert.equal((calls.match(/^gh run list/gm) ?? []).length, 3);
});

test("one command keeps the Account payment gates when its fixed-SHA artifacts come from the signed local path", () => {
  const setup = fixture();
  const artifacts = join(setup.root, "signed-local-artifacts");
  mkdirSync(artifacts);

  const output = execFileSync(
    command,
    [releaseSha, "--local-artifacts", artifacts, "--execute"],
    { encoding: "utf8", env: setup.env },
  );
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, new RegExp(`release ${releaseSha} activated`));
  assert.equal((calls.match(/^watcher /gm) ?? []).length, 2);
  assert.equal(
    (calls.match(/watcher --local-artifacts .* --sha a{40}/g) ?? []).length,
    2,
  );
  assert.equal((calls.match(/^gh run list/gm) ?? []).length, 0);
  assert.equal((calls.match(/gh api .*\/branches\//g) ?? []).length, 3);
  assert.match(calls, /ssh root@metaview\.top .*deploy-epay-gateway-patches\.sh.*--execute/);
});

test("normal activation audits an explicitly named historical release owner before approval", () => {
  const setup = fixture();
  setup.env.HENUKIT_RETAINED_RELEASE_OWNER_UID = "1002";
  writeFileSync(
    join(setup.state, "last-activated-sha"),
    `${recoveryBaselineSha}\n`,
    { mode: 0o600 },
  );

  execFileSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");
  const adoption = new RegExp(
    `adopt-retained-release --sha ${recoveryBaselineSha} `
      + `--candidate-sha ${releaseSha} --expected-owner-uid 1002`,
    "g",
  );

  assert.equal((calls.match(adoption) ?? []).length, 2);
  assert.match(calls, /adopt-retained-release .*--preflight/);
  assert.match(calls, /adopt-retained-release .*--execute/);
  assert.ok(calls.indexOf("watcher --once") < calls.indexOf("adopt-retained-release"));
  assert.ok(calls.lastIndexOf("adopt-retained-release") < calls.indexOf("deploy-epay-gateway-patches.sh"));
  assert.ok(calls.lastIndexOf("adopt-retained-release") < calls.lastIndexOf("watcher --once"));
});

test("one command threads an explicit degraded-baseline recovery through both watcher passes", () => {
  const setup = fixture();
  const artifacts = join(setup.root, "signed-local-artifacts");
  const previousSha = "c".repeat(40);
  mkdirSync(artifacts);

  execFileSync(
    command,
    [
      releaseSha,
      "--local-artifacts", artifacts,
      "--recover-degraded-baseline", previousSha,
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );
  const calls = readFileSync(setup.log, "utf8");

  assert.equal(
    (calls.match(new RegExp(`--recover-degraded-baseline ${previousSha}`, "g")) ?? []).length,
    2,
  );
});

test("explicit local recovery resumes one valid unconsumed approval without preparing twice", () => {
  const setup = fixture({ existingApproval: true });
  const artifacts = join(setup.root, "signed-local-artifacts");
  const previousSha = "c".repeat(40);
  mkdirSync(artifacts);

  const output = execFileSync(
    command,
    [
      releaseSha,
      "--local-artifacts", artifacts,
      "--recover-degraded-baseline", previousSha,
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, /resuming valid unconsumed approval/i);
  assert.equal((calls.match(/^watcher /gm) ?? []).length, 1);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("normal activation never silently reuses an existing approval", () => {
  const setup = fixture({ existingApproval: true });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /approval already exists/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher |^ssh /m);
});

test("recovery never reuses an approval prepared for a different baseline", () => {
  const setup = fixture({
    existingApproval: true,
    existingApprovalBaseline: "d".repeat(40),
  });

  const artifacts = join(setup.root, "signed-local-artifacts");
  mkdirSync(artifacts);
  const result = spawnSync(
    command,
    [
      releaseSha,
      "--local-artifacts", artifacts,
      "--recover-degraded-baseline", recoveryBaselineSha,
      "--execute",
    ],
    {
    encoding: "utf8",
    env: setup.env,
    },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to recovery baseline/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /deploy-epay|--execute/);
});

test("activation rejects a writable approval directory before reading or writing state", () => {
  const setup = fixture();
  chmodSync(join(setup.state, "approvals"), 0o770);

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /trusted private parent chain/i);
  assert.equal(readFileSync(setup.log, "utf8"), "");
});

test("first activation safely creates missing private state directories", () => {
  const setup = fixture();
  rmSync(setup.state, { recursive: true, force: true });

  const output = execFileSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.match(output, new RegExp(`release ${releaseSha} activated`));
  for (const directory of [
    setup.state,
    join(setup.state, "approvals"),
    join(setup.state, "prepared"),
  ]) {
    assert.equal(lstatSync(directory).mode & 0o777, 0o700);
  }
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
  assert.match(result.stderr, /metadata is missing or untrusted/i);
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
