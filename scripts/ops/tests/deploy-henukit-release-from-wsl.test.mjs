import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(
  new URL("../deploy-henukit-release-from-wsl.sh", import.meta.url),
);
const releaseSha = "a".repeat(40);
const deploySource = readFileSync(script, "utf8");

function heredoc(name) {
  const match = deploySource.match(new RegExp(`<<'${name}'\\n([\\s\\S]*?)\\n${name}`));
  assert.ok(match, `missing ${name} heredoc`);
  return match[1];
}

function writeExecutable(path, body) {
  writeFileSync(path, body, { mode: 0o755 });
  chmodSync(path, 0o755);
}

function fixture({
  configuredAlias = false,
  kernelRelease = "5.15.167.4-microsoft-standard-WSL2",
  proxyCommand = "",
  proxyJump = "",
  remoteArtifactState = "absent",
  remoteTrustFails = false,
  activationFails = false,
  watcherActive = false,
  verifierFails = false,
} = {}) {
  const root = mkdtempSync(join(tmpdir(), "henukit-wsl-deploy-"));
  const bin = join(root, "bin");
  const artifacts = join(root, `henukit-release-${releaseSha}`);
  const signers = join(root, "release-signers");
  const verifier = join(root, "verify-local");
  const log = join(root, "calls.log");
  mkdirSync(bin);
  mkdirSync(artifacts);
  writeFileSync(join(artifacts, "RELEASE_SHA"), `${releaseSha}\n`, { mode: 0o400 });
  writeFileSync(signers, "fixture signer\n", { mode: 0o600 });
  writeFileSync(log, "");

  writeExecutable(
    verifier,
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'verify %s\n' "$*" >> "$FAKE_CALL_LOG"
[[ "$FAKE_VERIFIER_FAILS" != "1" ]]
`,
  );
  writeExecutable(
    join(bin, "uname"),
    `#!/usr/bin/env bash
case "\${1:-}" in
  -s) printf Linux ;;
  -m) printf x86_64 ;;
  -r) printf '%s' "$FAKE_KERNEL_RELEASE" ;;
  *) exit 64 ;;
esac
`,
  );
  writeExecutable(
    join(bin, "git"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
case "$*" in
  *"rev-parse HEAD"*) printf '%s\n' "$FAKE_RELEASE_SHA" ;;
  *"status --porcelain --untracked-files=all"*) ;;
  *"ls-remote --exit-code origin refs/heads/main"*) printf '%s\trefs/heads/main\n' "$FAKE_RELEASE_SHA" ;;
  *) printf 'unexpected git call: %s\n' "$*" >&2; exit 64 ;;
esac
`,
  );
  writeExecutable(
    join(bin, "ssh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "\${1:-}" == "-G" ]]; then
  printf 'hostname %s\n' "${configuredAlias ? "production.internal" : "henu-prod"}"
  printf 'user root\nport 22222\nstricthostkeychecking yes\n'
  [[ -z "$FAKE_PROXY_JUMP" ]] || printf 'proxyjump %s\n' "$FAKE_PROXY_JUMP"
  [[ -z "$FAKE_PROXY_COMMAND" ]] || printf 'proxycommand %s\n' "$FAKE_PROXY_COMMAND"
  exit 0
fi
payload="$(cat)"
printf 'ssh %s :: %s\n' "$*" "\${payload//$'\n'/ }" >> "$FAKE_CALL_LOG"
if [[ "$payload" == *"REMOTE_TRUST_FIXTURE_MARKER"* ]]; then
  printf 'test fixture marker leaked into production payload\n' >&2
  exit 64
fi
if [[ "$payload" == *"trusted_root_file()"* && "$FAKE_REMOTE_TRUST_FAILS" == "1" ]]; then
  exit 70
fi
if [[ "$payload" == *"printf 'verified-existing"* ]]; then
  printf '%s\n' "$FAKE_REMOTE_ARTIFACT_STATE"
  exit 0
fi
if [[ "$payload" == *'/usr/local/sbin/activate-henukit-release \\'* ]]; then
  if [[ "$FAKE_WATCHER_ACTIVE" == "1" ]]; then
    [[ "$payload" != *'systemctl stop "$watcher_service"'* ]]
    [[ "$payload" == *'quiesce.request'* ]]
    [[ "$payload" == *'quiesced_file'* ]]
    [[ "$payload" == *'trap restore_watcher EXIT'* ]]
    [[ "$payload" == *'systemctl is-active --quiet "$watcher_service" || exit 74'* ]]
  fi
  [[ "$FAKE_ACTIVATION_FAILS" != "1" ]]
fi
`,
  );
  writeExecutable(
    join(bin, "rsync"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'rsync %s\n' "$*" >> "$FAKE_CALL_LOG"
`,
  );

  return {
    artifacts,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      FAKE_CALL_LOG: log,
      FAKE_KERNEL_RELEASE: kernelRelease,
      FAKE_PROXY_COMMAND: proxyCommand,
      FAKE_PROXY_JUMP: proxyJump,
      FAKE_REMOTE_ARTIFACT_STATE: remoteArtifactState,
      FAKE_REMOTE_TRUST_FAILS: remoteTrustFails ? "1" : "0",
      FAKE_ACTIVATION_FAILS: activationFails ? "1" : "0",
      FAKE_WATCHER_ACTIVE: watcherActive ? "1" : "0",
      FAKE_RELEASE_SHA: releaseSha,
      FAKE_VERIFIER_FAILS: verifierFails ? "1" : "0",
      HENUKIT_LOCAL_ARTIFACT_VERIFIER: verifier,
    },
    log,
    signers,
  };
}

function args(setup, mode = "--preflight") {
  return [
    "--sha", releaseSha,
    "--artifact-dir", setup.artifacts,
    "--allowed-signers", setup.signers,
    "--remote-env-file", "/opt/henukit/.env.henukit",
    "--account-operator-role", "operations-operator",
    mode,
  ];
}

function remoteSandbox() {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "henukit-remote-sandbox-")));
  const bin = join(root, "bin");
  const sbin = join(root, "usr", "local", "sbin");
  const etc = join(root, "etc", "henukit");
  const state = join(root, "var", "lib", "henukit-actions-watch");
  const incoming = join(root, "opt", "henukit-incoming");
  mkdirSync(bin);
  mkdirSync(sbin, { recursive: true });
  mkdirSync(etc, { recursive: true });
  mkdirSync(state, { recursive: true });
  mkdirSync(incoming, { recursive: true });
  for (const helper of [
    "watch-henukit-actions",
    "verify-henukit-local-release.sh",
    "henukit-release-images.sh",
  ]) {
    writeExecutable(join(sbin, helper), "#!/usr/bin/env bash\nexit 0\n");
  }
  writeExecutable(
    join(sbin, "activate-henukit-release"),
    `#!/usr/bin/env bash
[[ "\${HENUKIT_PLATFORM_MIGRATIONS-}" == "\${FAKE_EXPECT_PLATFORM_MIGRATIONS-}" ]] || exit 93
exit "\${FAKE_ACTIVATE_STATUS:-0}"
`,
  );
  writeFileSync(join(etc, "release-signers"), "fixture\n", { mode: 0o400 });
  writeFileSync(join(etc, "github-actions-read.token"), "fixture\n", { mode: 0o400 });
  const envFile = join(root, "opt", "henukit", ".env.henukit");
  mkdirSync(join(root, "opt", "henukit"), { recursive: true });
  writeFileSync(envFile, "PORTAL_API_MODE=live\n", { mode: 0o400 });
  const serviceState = join(root, "service-state");
  writeFileSync(serviceState, "active\n");
  writeFileSync(join(state, "watcher.instance"), "4242\n", { mode: 0o600 });
  writeExecutable(
    join(bin, "id"),
    `#!/usr/bin/env bash
[[ "\${1:-}" == "-u" ]] && printf '%s\n' "\${FAKE_REMOTE_UID:-0}"
`,
  );
  writeExecutable(
    join(bin, "stat"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
format="$2"
target="$3"
if [[ "$format" == "%u" ]]; then
  [[ "$target" == "\${FAKE_BAD_OWNER_PATH:-}" ]] && printf '1000\n' || printf '0\n'
else
  [[ "$target" == "\${FAKE_BAD_MODE_PATH:-}" ]] && printf '777\n' || printf '755\n'
fi
`,
  );
  writeExecutable(
    join(bin, "systemctl"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
command="$1"
case "$command" in
  cat) exit 0 ;;
  is-active)
    if [[ "$(cat "$FAKE_SERVICE_STATE")" == "active" ]]; then
      if [[ -f "$FAKE_QUIESCE_FILE" ]]; then
        cp "$FAKE_QUIESCE_FILE" "$FAKE_QUIESCED_FILE"
        chmod 0600 "$FAKE_QUIESCED_FILE"
        printf 'inactive\n' > "$FAKE_SERVICE_STATE"
        exit 1
      fi
      exit 0
    fi
    exit 1
    ;;
  start)
    [[ "\${FAKE_START_FAIL:-0}" != "1" ]] || exit 1
    printf 'active\n' > "$FAKE_SERVICE_STATE"
    ;;
  show) printf '4242\n' ;;
  *) exit 64 ;;
esac
`,
  );
  writeExecutable(
    join(bin, "flock"),
    `#!/usr/bin/env bash
[[ "\${FAKE_TRANSPORT_LOCK_HELD:-0}" != "1" ]]
`,
  );
  const mapBlock = (name) => heredoc(name)
    .replaceAll("/usr/local/sbin", sbin)
    .replaceAll("/etc/henukit", etc)
    .replaceAll("/var/lib/henukit-actions-watch", state);
  return {
    activateBlock: mapBlock("REMOTE_ACTIVATE"),
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      FAKE_ACTIVATE_STATUS: "0",
      FAKE_EXPECT_PLATFORM_MIGRATIONS: "",
      FAKE_BAD_MODE_PATH: "",
      FAKE_BAD_OWNER_PATH: "",
      FAKE_QUIESCE_FILE: join(state, "quiesce.request"),
      FAKE_QUIESCED_FILE: join(state, "quiesced"),
      FAKE_REMOTE_UID: "0",
      FAKE_SERVICE_STATE: serviceState,
      FAKE_START_FAIL: "0",
      FAKE_TRANSPORT_LOCK_HELD: "0",
    },
    envFile,
    incoming,
    preflightBlock: mapBlock("REMOTE_PREFLIGHT"),
    root,
    sbin,
    serviceState,
    state,
  };
}

function runRemote(block, argv, env) {
  return spawnSync("sh", ["-s", "--", ...argv], {
    encoding: "utf8",
    env,
    input: block,
  });
}

test("preflight rejects an unresolved henu-prod alias before transfer or activation", () => {
  const setup = fixture();
  const result = spawnSync(script, args(setup), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /henu-prod.*configured alias/i);
});

test("the real remote preflight rejects non-root execution, writable ancestors, and symlinked helpers", () => {
  for (const fault of ["uid", "mode", "symlink"]) {
    const setup = remoteSandbox();
    if (fault === "uid") setup.env.FAKE_REMOTE_UID = "1000";
    if (fault === "mode") setup.env.FAKE_BAD_MODE_PATH = join(setup.root, "usr", "local");
    if (fault === "symlink") {
      const helper = join(setup.sbin, "verify-henukit-local-release.sh");
      unlinkSync(helper);
      symlinkSync("/bin/true", helper);
    }
    const result = runRemote(
      setup.preflightBlock,
      [
        setup.envFile,
        setup.incoming,
        join(setup.incoming, `henukit-release-${releaseSha}`),
        releaseSha,
      ],
      setup.env,
    );
    assert.notEqual(result.status, 0, `${fault} unexpectedly passed`);
  }
});

test("the real remote activation restores the watcher after activation failure", () => {
  const setup = remoteSandbox();
  setup.env.FAKE_ACTIVATE_STATUS = "42";
  const result = runRemote(
    setup.activateBlock,
    [releaseSha, join(setup.incoming, `henukit-release-${releaseSha}`), setup.envFile, "operations-operator", ""],
    setup.env,
  );

  assert.equal(result.status, 42, result.stderr);
  assert.equal(readFileSync(setup.serviceState, "utf8").trim(), "active");
  assert.equal(existsSync(join(setup.state, "quiesce.request")), false);
  assert.equal(existsSync(join(setup.state, "quiesced")), false);
});

test("the real remote activation decodes the no-migrations sentinel", () => {
  const setup = remoteSandbox();
  const result = runRemote(
    setup.activateBlock,
    [releaseSha, join(setup.incoming, `henukit-release-${releaseSha}`), setup.envFile, "operations-operator", "-"],
    setup.env,
  );

  assert.equal(result.status, 0, result.stderr);
});

test("the real remote activation reports watcher restart failure as exit 74", () => {
  const setup = remoteSandbox();
  setup.env.FAKE_START_FAIL = "1";
  const result = runRemote(
    setup.activateBlock,
    [releaseSha, join(setup.incoming, `henukit-release-${releaseSha}`), setup.envFile, "operations-operator", ""],
    setup.env,
  );

  assert.equal(result.status, 74, result.stderr);
  assert.equal(readFileSync(setup.serviceState, "utf8").trim(), "inactive");
});

test("the real remote activation rejects a concurrent transport with exit 76", () => {
  const setup = remoteSandbox();
  setup.env.FAKE_TRANSPORT_LOCK_HELD = "1";
  const result = runRemote(
    setup.activateBlock,
    [releaseSha, join(setup.incoming, `henukit-release-${releaseSha}`), setup.envFile, "operations-operator", ""],
    setup.env,
  );

  assert.equal(result.status, 76, result.stderr);
  assert.equal(readFileSync(setup.serviceState, "utf8").trim(), "active");
});

test("preflight rejects an henu-prod alias that routes through an SSH jump host", () => {
  const setup = fixture({ configuredAlias: true, proxyJump: "local-relay" });
  const result = spawnSync(script, args(setup), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /proxy|direct/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh .*henu-prod|rsync/);
});

test("preflight rejects an henu-prod alias that uses an SSH proxy command", () => {
  const setup = fixture({ configuredAlias: true, proxyCommand: "relay --stdio" });
  const result = spawnSync(script, args(setup), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /ProxyCommand|direct/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /ssh .*henu-prod|rsync/);
});

test("preflight verifies the bundle and production trust roots without transferring or activating", () => {
  const setup = fixture({ configuredAlias: true });
  const result = spawnSync(script, args(setup), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.equal(result.status, 0, result.stderr);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /verify .*--artifact-dir .* --sha a{40}/);
  assert.match(calls, /ssh .*henu-prod.*watch-henukit-actions/);
  assert.doesNotMatch(calls, /rsync|activate-henukit-release .*--execute/);
});

test("a generic Linux host is rejected before verification or production access", () => {
  const setup = fixture({ configuredAlias: true, kernelRelease: "6.8.0-generic" });
  const result = spawnSync(script, args(setup), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /must run on WSL2/i);
  assert.equal(readFileSync(setup.log, "utf8"), "");
});

test("a failed local signature check stops before production access", () => {
  const setup = fixture({ configuredAlias: true, verifierFails: true });
  const result = spawnSync(script, args(setup, "--execute"), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /^verify /);
  assert.doesNotMatch(calls, /ssh|rsync|activate-henukit-release/);
});

test("untrusted production helper permissions stop before transfer or activation", () => {
  const setup = fixture({ configuredAlias: true, remoteTrustFails: true });
  const result = spawnSync(script, args(setup, "--execute"), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /trust roots|preflight/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /rsync|activate-henukit-release .*--execute/);
});

test("execute quiesces an active watcher at a safe boundary and restores it when activation fails", () => {
  const setup = fixture({
    configuredAlias: true,
    activationFails: true,
    watcherActive: true,
  });
  const result = spawnSync(script, args(setup, "--execute"), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  const calls = readFileSync(setup.log, "utf8");
  assert.doesNotMatch(calls, /systemctl stop .*watcher_service/);
  assert.match(calls, /quiesce\.request/);
  assert.match(calls, /quiesced_file/);
  assert.match(calls, /trap restore_watcher EXIT/);
  assert.match(calls, /activate-henukit-release/);
});

test("execute safely resumes a previously verified production bundle without retransferring", () => {
  const setup = fixture({
    configuredAlias: true,
    remoteArtifactState: "verified-existing",
  });
  const result = spawnSync(script, args(setup, "--execute"), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.equal(result.status, 0, result.stderr);
  const calls = readFileSync(setup.log, "utf8");
  assert.doesNotMatch(calls, /rsync /);
  assert.match(calls, /verify-henukit-local-release\.sh/);
  assert.match(calls, /activate-henukit-release/);
  assert.match(calls, /operations-operator - :: set -eu/);
});

test("execute transfers the signed bundle directly from WSL to henu-prod and uses the guarded activation entry", () => {
  const setup = fixture({ configuredAlias: true });
  const result = spawnSync(script, args(setup, "--execute"), {
    encoding: "utf8",
    env: setup.env,
  });

  assert.equal(result.status, 0, result.stderr);
  const calls = readFileSync(setup.log, "utf8");
  assert.match(calls, /verify .*--artifact-dir .* --sha a{40} .*--allowed-signers/);
  assert.match(calls, /rsync .*henu-prod:\/opt\/henukit-incoming\/\.incoming-a{40}-[0-9]+-[0-9]+\//);
  assert.match(calls, /ssh .*henu-prod.*verify-henukit-local-release\.sh/);
  assert.match(calls, /ssh .*henu-prod.*activate-henukit-release/);
  assert.match(calls, /operations-operator - :: set -eu/);
  assert.doesNotMatch(calls, /scp|jerry-wsl|henukit-rel-/);
});
