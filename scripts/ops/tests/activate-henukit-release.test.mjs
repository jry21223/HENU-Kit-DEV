import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { forwarderContract } from "./active-alias-fixture.mjs";

const command = fileURLToPath(
  new URL("../activate-henukit-release.sh", import.meta.url),
);
const releaseSha = "a".repeat(40);
const recoveryBaselineSha = "c".repeat(40);

function executable(path, contents) {
  writeFileSync(path, contents, { mode: 0o755 });
  chmodSync(path, 0o755);
}

function signedLocalArtifacts(root) {
  const artifacts = join(root, "signed-local-artifacts");
  mkdirSync(artifacts);
  writeFileSync(
    join(artifacts, `henukit-release-${releaseSha}.manifest`),
    "verified-by-fixture\n",
  );
  return artifacts;
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
  existingApprovalSource = "local",
  runConclusion = "success",
  runAttempt = "1",
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
  writeFileSync(join(release, "RELEASE_SHA"), releaseSha + "\n", { mode: 0o444 });
  mkdirSync(join(releases, recoveryBaselineSha));
  writeFileSync(join(releases, recoveryBaselineSha, "RELEASE_SHA"), recoveryBaselineSha + "\n", { mode: 0o444 });
  const current = join(root, "current");
  const compatible = join(root, "compat-current");
  symlinkSync(release, current, "dir");
  symlinkSync(release, compatible, "dir");
  forwarderContract(state, releases, compatible, current);
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
    writeFileSync(
      join(state, "prepared", `${releaseSha}.artifact-source`),
      existingApprovalSource === "actions"
        ? "actions:123:1\n"
        : `local:${createHash("sha256").update("verified-by-fixture\n").digest("hex")}\n`,
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
  printf '123\t%s\t%s\tcompleted\t%s\n' "$FAKE_RUN_ATTEMPT" "$FAKE_RELEASE_SHA" "$FAKE_RUN_CONCLUSION"
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
  if [[ "\${FAKE_WATCHER_ALIAS_DRIFT:-0}" == "1" ]]; then
    rm "$HENUKIT_COMPAT_CURRENT_LINK"
    ln -s "$HENUKIT_RELEASE_ROOT/cccccccccccccccccccccccccccccccccccccccc" "$HENUKIT_COMPAT_CURRENT_LINK"
  fi
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
      FAKE_RUN_ATTEMPT: runAttempt,
      FAKE_RUN_CONCLUSION: runConclusion,
      HENUKIT_STATE_ROOT: state,
      HENUKIT_TRUST_ANCHOR: root,
      HENUKIT_RELEASE_ROOT: releases,
      HENUKIT_CURRENT_LINK: current,
      HENUKIT_COMPAT_CURRENT_LINK: compatible,
      HENUKIT_ENV_FILE: envFile,
      GH_TOKEN_FILE: tokenFile,
      HENUKIT_WATCHER: watcher,
      HENUKIT_RETAINED_RELEASE_ADOPTER: join(bin, "adopt-retained-release"),
    },
  };
}

test("automatic activation refuses a workflow attempt different from its authorization", () => {
  const setup = fixture({ runAttempt: "2" });
  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8", env: { ...setup.env, HENUKIT_EXPECTED_ACTIONS_IDENTITY: "actions:123:1" },
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /identity differs from the automatic authorization/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh |watcher /);
});

test("same-SHA Notice configuration uses the original watcher without EasyPay or activation replay", () => {
  const setup = fixture();
  writeFileSync(join(setup.state, "last-activated-sha"), releaseSha + "\n", { mode: 0o600 });
  writeFileSync(join(setup.state, "direct-deploy.lock"), "", { mode: 0o600 });
  const plan = join(setup.root, "notice-plan.json");
  writeFileSync(plan, "{}\n", { mode: 0o600 }); // plan itself is verified inside the watcher/helper
  const result = spawnSync(command, [releaseSha, "--notice-url", "apply", "--plan", plan, "--execute"], {
    encoding: "utf8", env: setup.env,
  });
  assert.equal(result.status, 0, result.stderr);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /watcher --notice-url apply --sha a{40} --plan /);
  assert.doesNotMatch(calls, /gh |ssh |watcher --once/);
});

test("already-active requires both real aliases before skipping activation", () => {
  const setup = fixture();
  writeFileSync(join(setup.state, "last-activated-sha"), releaseSha + "\n", { mode: 0o600 });
  unlinkSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK);
  symlinkSync(join(setup.env.HENUKIT_RELEASE_ROOT, recoveryBaselineSha), setup.env.HENUKIT_COMPAT_CURRENT_LINK, "dir");
  const result = spawnSync(command, [releaseSha, "--execute"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh |watcher /);
});

test("a non-SHA baseline stops before remote environment or approval writes", () => {
  const setup = fixture();
  const invalid = join(setup.env.HENUKIT_RELEASE_ROOT, "non-sha");
  mkdirSync(invalid);
  writeFileSync(join(invalid, "RELEASE_SHA"), "non-sha\n", { mode: 0o444 });
  unlinkSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK);
  symlinkSync(invalid, setup.env.HENUKIT_COMPAT_CURRENT_LINK, "dir");
  const environment = readFileSync(setup.env.HENUKIT_ENV_FILE);
  const result = spawnSync(command, [releaseSha, "--execute"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh |watcher /);
  assert.deepEqual(readFileSync(setup.env.HENUKIT_ENV_FILE), environment);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
});

test("watcher marker without both aliases is not reported as activated", () => {
  const setup = fixture();
  setup.env.FAKE_WATCHER_ALIAS_DRIFT = "1";
  const result = spawnSync(command, [releaseSha, "--execute"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /complete active-alias contract/);
  assert.doesNotMatch(result.stdout, new RegExp(`release ${releaseSha} activated`));
});

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
  const artifacts = signedLocalArtifacts(setup.root);

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
  const artifacts = signedLocalArtifacts(setup.root);
  const previousSha = "c".repeat(40);

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

test("one command threads Actions degraded-baseline recovery through both watcher passes", () => {
  const setup = fixture();
  const previousSha = "c".repeat(40);

  execFileSync(
    command,
    [releaseSha, "--recover-degraded-baseline", previousSha, "--execute"],
    { encoding: "utf8", env: setup.env },
  );
  const calls = readFileSync(setup.log, "utf8");

  assert.equal(
    (calls.match(new RegExp(`watcher --once --recover-degraded-baseline ${previousSha}`, "g")) ?? []).length,
    2,
  );
  assert.equal((calls.match(/^gh run list/gm) ?? []).length, 3);
});

test("explicit local recovery resumes one valid unconsumed approval without preparing twice", () => {
  const setup = fixture({ existingApproval: true });
  const artifacts = signedLocalArtifacts(setup.root);
  const previousSha = "c".repeat(40);

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

test("explicit Actions recovery resumes one baseline-bound unconsumed approval", () => {
  const setup = fixture({ existingApproval: true, existingApprovalSource: "actions" });

  const output = execFileSync(
    command,
    [releaseSha, "--recover-degraded-baseline", recoveryBaselineSha, "--execute"],
    { encoding: "utf8", env: setup.env },
  );
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, /resuming valid unconsumed approval/i);
  assert.equal((calls.match(/^watcher /gm) ?? []).length, 1);
  assert.match(calls, new RegExp(`watcher --once --recover-degraded-baseline ${recoveryBaselineSha}`));
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("Actions recovery rejects an approval prepared from signed local artifacts", () => {
  const setup = fixture({ existingApproval: true, existingApprovalSource: "local" });

  const result = spawnSync(
    command,
    [releaseSha, "--recover-degraded-baseline", recoveryBaselineSha, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to actions artifacts/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher |^ssh /m);
});

test("Actions recovery rejects an approval prepared by an earlier run attempt", () => {
  const setup = fixture({
    existingApproval: true,
    existingApprovalSource: "actions",
    runAttempt: "2",
  });

  const result = spawnSync(
    command,
    [releaseSha, "--recover-degraded-baseline", recoveryBaselineSha, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to actions artifacts/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher |^ssh /m);
});

test("signed local recovery rejects an approval prepared from Actions artifacts", () => {
  const setup = fixture({ existingApproval: true, existingApprovalSource: "actions" });
  const artifacts = signedLocalArtifacts(setup.root);

  const result = spawnSync(
    command,
    [
      releaseSha,
      "--local-artifacts", artifacts,
      "--recover-degraded-baseline", recoveryBaselineSha,
      "--execute",
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to local artifacts/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^watcher |^ssh /m);
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

  const artifacts = signedLocalArtifacts(setup.root);
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

test("recreated private state cannot silently bootstrap missing alias authority", () => {
  const setup = fixture();
  rmSync(setup.state, { recursive: true, force: true });

  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /reviewed versioned bootstrap/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh |watcher /);
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

test("full approval entry refuses an active scoped release before external or environment writes", () => {
  const setup = fixture();
  writeFileSync(join(setup.state, "scoped-food-release"), "private scoped receipt\n", { mode: 0o600 });
  const before = readFileSync(setup.env.HENUKIT_ENV_FILE);
  const result = spawnSync(command, [releaseSha, "--execute"], { env: setup.env, encoding: "utf8" });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /scoped Food release/);
  assert.deepEqual(readFileSync(setup.env.HENUKIT_ENV_FILE), before);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /watcher|ssh|gateway/);
});

test("preservation rejects an unavailable existing authority before credential helpers", () => {
  const setup = fixture();
  const before = lstatSync(setup.env.HENUKIT_ENV_FILE);
  const contents = readFileSync(setup.env.HENUKIT_ENV_FILE);
  const result = spawnSync(command, [releaseSha, "--preserve-existing", "--execute"], {
    encoding: "utf8", env: setup.env,
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /preserv|existing/i);
  assert.deepEqual(readFileSync(setup.env.HENUKIT_ENV_FILE), contents);
  assert.equal(lstatSync(setup.env.HENUKIT_ENV_FILE).ino, before.ino);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh |watcher /);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
});

test("prepared preservation cannot downgrade through the original activator after receipt loss", () => {
  const setup = fixture();
  const backup = join(setup.state, "preserved-baseline.dump");
  writeFileSync(backup, "verified backup", { mode: 0o600 });
  writeFileSync(`${backup}.meta`, `release_sha=${releaseSha}\npreservation_mode=preserve-existing:${"d".repeat(64)}\n`, { mode: 0o600 });
  writeFileSync(join(setup.state, "prepared", releaseSha), `${backup}\n`, { mode: 0o600 });
  const before = lstatSync(setup.env.HENUKIT_ENV_FILE).ino;
  const result = spawnSync(command, [releaseSha, "--execute"], {
    encoding: "utf8", env: { ...setup.env, HENUKIT_BACKUP_ROOT: setup.state },
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /preserv/);
  assert.equal(lstatSync(setup.env.HENUKIT_ENV_FILE).ino, before);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh |watcher /);
});
