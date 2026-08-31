import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  chmodSync,
  cpSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { spawnSync as run } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const rollback = join(repoRoot, "services", "getwork-mcp", "deploy", "rollback_node.sh");
const installer = join(repoRoot, "services", "getwork-mcp", "deploy", "install_node.sh");
const productionAccountInstaller = join(
  repoRoot,
  "services",
  "getwork-mcp",
  "deploy",
  "install_production_tunnel_account.sh",
);
const deployAssets = join(repoRoot, "services", "getwork-mcp", "deploy");

function executable(path, contents) {
  writeFileSync(path, "#!/usr/bin/env bash\nset -Eeuo pipefail\n" + contents);
  chmodSync(path, 0o755);
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function fixtureRun({ crawlerRemains = false, priorUnits = true } = {}) {
  const fixture = mkdtempSync(join(tmpdir(), "getwork-rollback-"));
  const bin = join(fixture, "bin");
  mkdirSync(bin);
  writeFileSync(join(fixture, "service-state"), [
    `unit_henukit_getwork_mcp_service_enabled=${priorUnits ? "1" : "0"}`,
    `unit_henukit_getwork_mcp_service_active=${priorUnits ? "1" : "0"}`,
    "unit_henukit_getwork_tunnel_service_enabled=0",
    "unit_henukit_getwork_tunnel_service_active=0",
    "network_present=1",
      "account_present=0",
      "group_present=0",
    "account_gid=",
    "account_home=",
    "account_shell=",
    "account_password=",
    "account_groups=",
    "account_home_metadata=",
    "",
  ].join("\n"));
  executable(join(bin, "systemctl"), [
    "printf 'systemctl %s\\n' \"$*\" >> /fixture/calls",
    "case \"$1\" in",
    "  is-active) printf 'inactive\\n' ;;",
    "  show) printf '0\\n' ;;",
    "esac",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "docker"), [
    "if [[ \"$1\" == inspect && \"$2\" == henukit-getwork-mcp && \"${CRAWLER_REMAINS:-0}\" == 1 ]]; then exit 0; fi",
    "if [[ \"$1\" == inspect ]]; then exit 1; fi",
    "exit 0",
    "",
  ].join("\n"));
  for (const command of ["getent", "pgrep"]) executable(join(bin, command), "exit 1\n");
  for (const command of ["userdel", "groupdel", "usermod", "chpasswd", "iptables"]) {
    executable(join(bin, command), "exit 0\n");
  }
  const setup = [
    "install -d -o root -g root -m 0755 /trusted",
    "install -o root -g root -m 0755 /source/rollback_node.sh /trusted/rollback_node.sh",
    "install -d -m 0700 /var/lib/henukit-getwork-backups/test/etc/henukit-getwork",
    "install -d -m 0700 /var/lib/henukit-getwork-backups/test/etc/systemd/system",
    "install -m 0400 /fixture/service-state /var/lib/henukit-getwork-backups/test/service-state",
    "printf old > /var/lib/henukit-getwork-backups/test/etc/henukit-getwork/marker",
    ...(priorUnits ? [
      "printf old-mcp > /var/lib/henukit-getwork-backups/test/etc/systemd/system/henukit-getwork-mcp.service",
      "printf old-tunnel > /var/lib/henukit-getwork-backups/test/etc/systemd/system/henukit-getwork-tunnel.service",
    ] : []),
    "install -d /etc/henukit-getwork /etc/systemd/system",
    "printf candidate > /etc/henukit-getwork/marker",
    "printf candidate-mcp > /etc/systemd/system/henukit-getwork-mcp.service",
    "printf candidate-tunnel > /etc/systemd/system/henukit-getwork-tunnel.service",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin /trusted/rollback_node.sh /var/lib/henukit-getwork-backups/test",
    "test \"$(cat /etc/henukit-getwork/marker)\" = old",
    priorUnits
      ? "test -f /etc/systemd/system/henukit-getwork-mcp.service"
      : "test ! -e /etc/systemd/system/henukit-getwork-mcp.service",
    "",
  ].join("\n");
  const result = run("docker", [
    "run", "--rm", "--env", "CRAWLER_REMAINS=" + (crawlerRemains ? "1" : "0"),
    "--volume", rollback + ":/source/rollback_node.sh:ro",
    "--volume", fixture + ":/fixture",
    "debian:bookworm-slim", "bash", "-ceu", setup,
  ], { encoding: "utf8" });
  return { result, calls: readFileSync(join(fixture, "calls"), "utf8") };
}

test("rollback restores recorded unit enablement and activity", () => {
  const { result, calls } = fixtureRun();
  assert.equal(result.status, 0, result.stderr);
  assert.match(calls, /systemctl enable henukit-getwork-mcp\.service/);
  assert.match(calls, /systemctl start henukit-getwork-mcp\.service/);
  assert.match(calls, /systemctl disable henukit-getwork-tunnel\.service/);
  assert.match(calls, /systemctl stop henukit-getwork-tunnel\.service/);
});

test("rollback fails before moving trust files when the crawler remains", () => {
  const { result } = fixtureRun({ crawlerRemains: true });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /crawler container remained/);
});

test("first-install rollback removes dangling enablement when no prior units exist", () => {
  const { result, calls } = fixtureRun({ priorUnits: false });
  assert.equal(result.status, 0, result.stderr);
  assert.match(calls, /systemctl disable henukit-getwork-mcp\.service/);
  assert.match(calls, /systemctl disable henukit-getwork-tunnel\.service/);
});

test("installer rejects an untrusted staging ancestry before loading Docker", () => {
  const fixture = mkdtempSync(join(tmpdir(), "getwork-install-fail-"));
  const bin = join(fixture, "bin");
  mkdirSync(bin);
  for (const command of ["docker", "git", "iptables", "jq", "ssh-keygen", "systemctl", "tar", "timeout"]) {
    executable(join(bin, command), "printf invoked >> /fixture/invoked\nexit 91\n");
  }
  const setup = [
    "install -d -o root -g root -m 0755 /trusted",
    "install -o root -g root -m 0755 /source/install_node.sh /trusted/install_node.sh",
    "install -d /tmp/untrusted-stage",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin /trusted/install_node.sh --sha aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --stage-dir /tmp/untrusted-stage --allowed-signers /tmp/missing-signers --trust-file /tmp/missing-trust",
  ].join("\n");
  const result = run("docker", [
    "run", "--rm",
    "--volume", installer + ":/source/install_node.sh:ro",
    "--volume", fixture + ":/fixture",
    "debian:bookworm-slim", "bash", "-ceu", setup,
  ], { encoding: "utf8" });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /stage directory ancestry is not root-trusted/);
});

function productionAccountFailureFixture({
  existingAccount = false,
  accountHome = "/var/lib/fixture-original-home",
  originalAuthorizedKey = false,
  failure = "validation",
} = {}) {
  const fixture = mkdtempSync(join(tmpdir(), "getwork-production-account-fail-"));
  const bin = join(fixture, "bin");
  mkdirSync(bin);
  executable(join(bin, "ssh-keygen"), [
    "if [[ \" $* \" == *\" -lf \"* ]]; then",
    "  printf '256 SHA256:approved fixture (ED25519)\\n'",
    "fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "sshd"), failure === "validation" ? [
    "if [[ \"${1:-}\" == -T ]]; then printf 'logingracetime 1\\n'; exit 0; fi",
    "if [[ \"${1:-}\" == -t ]]; then exit 78; fi",
    "exit 0",
    "",
  ].join("\n") : [
    "if [[ \"${1:-}\" == -T ]]; then",
    "  printf '%s\\n' \\",
    "    'authenticationmethods publickey' \\",
    "    'authorizedkeysfile /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys' \\",
    "    'passwordauthentication no' \\",
    "    'kbdinteractiveauthentication no' \\",
    "    'permittty no' \\",
    "    'x11forwarding no' \\",
    "    'allowagentforwarding no' \\",
    "    'allowtcpforwarding remote' \\",
    "    'allowstreamlocalforwarding no' \\",
    "    'gatewayports no' \\",
    "    'permitopen none' \\",
    "    'permitlisten 127.0.0.1:18100' \\",
    "    'forcecommand /bin/false' \\",
    "    'logingracetime 1'",
    "fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "systemctl"), [
    "printf '%s\\n' \"$*\" >> /fixture/systemctl-calls",
    ...(failure === "reload" ? ["if [[ \"${1:-}\" == reload ]]; then exit 79; fi"] : []),
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "pgrep"), "exit 1\n");
  executable(join(bin, "pkill"), "exit 0\n");
  const setup = [
    "install -d -o root -g root -m 0755 /etc/ssh/sshd_config.d /trusted /trusted-inputs",
    "install -o root -g root -m 0755 /source/install_production_tunnel_account.sh /trusted/install_production_tunnel_account.sh",
    ...(existingAccount ? [
      "groupadd --system fixture-primary",
      `useradd --system --gid fixture-primary --home-dir ${accountHome} --create-home --shell /usr/sbin/nologin henukit-getwork-tunnel`,
      "! getent group henukit-getwork-tunnel >/dev/null",
      ...(originalAuthorizedKey ? [
        `install -d -o root -g fixture-primary -m 0750 ${accountHome}/.ssh`,
        `printf 'ssh-ed25519 AAAAOLD old\\n' > ${accountHome}/.ssh/authorized_keys`,
        `chown root:fixture-primary ${accountHome}/.ssh/authorized_keys`,
        `chmod 0640 ${accountHome}/.ssh/authorized_keys`,
      ] : []),
    ] : []),
    "printf 'ssh-ed25519 AAAAFIXTURE fixture\\n' > /trusted-inputs/id_ed25519.pub",
    "printf 'HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT=SHA256:approved\\n' > /trusted-inputs/trust.env",
    "chmod 0600 /trusted-inputs/id_ed25519.pub /trusted-inputs/trust.env",
    "set +e",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin /trusted/install_production_tunnel_account.sh /trusted-inputs/id_ed25519.pub /trusted-inputs/trust.env",
    "status=$?",
    "set -e",
    "test \"$status\" -ne 0",
    ...(failure === "reload" ? ["grep -Fqx 'reload sshd' /fixture/systemctl-calls"] : []),
    existingAccount
      ? `test "$(getent passwd henukit-getwork-tunnel | cut -d: -f4,6,7)" = "$(getent group fixture-primary | cut -d: -f3):${accountHome}:/usr/sbin/nologin"`
      : "! getent passwd henukit-getwork-tunnel >/dev/null",
    "! getent group henukit-getwork-tunnel >/dev/null",
    ...(accountHome === "/var/lib/henukit-getwork-tunnel" ? [
      "test \"$(cat /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys)\" = 'ssh-ed25519 AAAAOLD old'",
      "test -z \"$(find /var/lib/henukit-getwork-tunnel/.ssh -maxdepth 1 -name '.authorized_keys.*' -print -quit)\"",
    ] : ["test ! -e /var/lib/henukit-getwork-tunnel"]),
    "test ! -e /etc/ssh/sshd_config.d/60-henukit-getwork-tunnel.conf",
    "",
  ].join("\n");
  const result = run("docker", [
    "run", "--rm",
    "--volume", productionAccountInstaller + ":/source/install_production_tunnel_account.sh:ro",
    "--volume", fixture + ":/fixture",
    "debian:bookworm-slim", "bash", "-ceu", setup,
  ], { encoding: "utf8" });
  return result;
}

test("failed production account install removes a newly created account, group, and home", () => {
  const result = productionAccountFailureFixture();
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stderr, /sshd configuration validation failed/);
});

test("failed production account install removes a newly created group for an existing account", () => {
  const result = productionAccountFailureFixture({ existingAccount: true });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stderr, /sshd configuration validation failed/);
});

test("policy reload failure preserves the old key until atomic replacement", () => {
  const result = productionAccountFailureFixture({
    existingAccount: true,
    accountHome: "/var/lib/henukit-getwork-tunnel",
    originalAuthorizedKey: true,
    failure: "reload",
  });
  assert.equal(result.status, 0, result.stderr);
});

test("hard interruption before key promotion preserves a nonstandard-home account key", () => {
  const fixture = mkdtempSync(join(tmpdir(), "getwork-production-account-kill-"));
  const bin = join(fixture, "bin");
  mkdirSync(bin);
  executable(join(bin, "ssh-keygen"), [
    "if [[ \" $* \" == *\" -lf \"* ]]; then",
    "  printf '256 SHA256:approved fixture (ED25519)\\n'",
    "fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "sshd"), [
    "if [[ \"${1:-}\" == -T ]]; then",
    "  printf '%s\\n' \\",
    "    'authenticationmethods publickey' \\",
    "    'authorizedkeysfile /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys' \\",
    "    'passwordauthentication no' \\",
    "    'kbdinteractiveauthentication no' \\",
    "    'permittty no' \\",
    "    'x11forwarding no' \\",
    "    'allowagentforwarding no' \\",
    "    'allowtcpforwarding remote' \\",
    "    'allowstreamlocalforwarding no' \\",
    "    'gatewayports no' \\",
    "    'permitopen none' \\",
    "    'permitlisten 127.0.0.1:18100' \\",
    "    'forcecommand /bin/false' \\",
    "    'logingracetime 1'",
    "fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "systemctl"), [
    "if [[ \"${1:-}\" == reload ]]; then kill -KILL \"$PPID\"; fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "pgrep"), "exit 1\n");
  executable(join(bin, "pkill"), "exit 0\n");
  const setup = [
    "install -d -o root -g root -m 0755 /etc/ssh/sshd_config.d /trusted /trusted-inputs",
    "install -o root -g root -m 0755 /source/install_production_tunnel_account.sh /trusted/install_production_tunnel_account.sh",
    "groupadd --system fixture-primary",
    "useradd --system --gid fixture-primary --home-dir /var/lib/fixture-original-home --create-home --shell /usr/sbin/nologin henukit-getwork-tunnel",
    "install -d -o root -g fixture-primary -m 0750 /var/lib/fixture-original-home/.ssh",
    "printf 'ssh-ed25519 AAAAOLD old\\n' > /var/lib/fixture-original-home/.ssh/authorized_keys",
    "chown root:fixture-primary /var/lib/fixture-original-home/.ssh/authorized_keys",
    "chmod 0640 /var/lib/fixture-original-home/.ssh/authorized_keys",
    "install -d -o root -g root -m 0750 /var/lib/henukit-getwork-tunnel",
    "install -d -o root -g fixture-primary -m 0750 /var/lib/henukit-getwork-tunnel/.ssh",
    "printf 'ssh-ed25519 AAAASTALE stale\\n' > /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys",
    "chown root:fixture-primary /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys",
    "chmod 0640 /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys",
    "printf 'ssh-ed25519 AAAAFIXTURE fixture\\n' > /trusted-inputs/id_ed25519.pub",
    "printf 'HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT=SHA256:approved\\n' > /trusted-inputs/trust.env",
    "chmod 0600 /trusted-inputs/id_ed25519.pub /trusted-inputs/trust.env",
    "set +e",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin /trusted/install_production_tunnel_account.sh /trusted-inputs/id_ed25519.pub /trusted-inputs/trust.env",
    "status=$?",
    "set -e",
    "test \"$status\" -eq 137",
    "test \"$(getent passwd henukit-getwork-tunnel | cut -d: -f6)\" = /var/lib/fixture-original-home",
    "test \"$(cat /var/lib/fixture-original-home/.ssh/authorized_keys)\" = 'ssh-ed25519 AAAAOLD old'",
    "test \"$(cat /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys)\" = 'ssh-ed25519 AAAAOLD old'",
    "! grep -Fq AAAAFIXTURE /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys",
    "! grep -Fq AAAASTALE /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys",
    "test -f /etc/ssh/sshd_config.d/60-henukit-getwork-tunnel.conf",
    "",
  ].join("\n");
  const result = run("docker", [
    "run", "--rm",
    "--volume", productionAccountInstaller + ":/source/install_production_tunnel_account.sh:ro",
    "--volume", fixture + ":/fixture",
    "debian:bookworm-slim", "bash", "-ceu", setup,
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, `${result.stderr}\n${result.stdout}`);
});

test("successful key promotion drains authenticated account processes first", () => {
  const fixture = mkdtempSync(join(tmpdir(), "getwork-production-account-success-"));
  const bin = join(fixture, "bin");
  mkdirSync(bin);
  executable(join(bin, "ssh-keygen"), [
    "if [[ \" $* \" == *\" -lf \"* ]]; then",
    "  printf '256 SHA256:approved fixture (ED25519)\\n'",
    "fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "sshd"), [
    "if [[ \"${1:-}\" == -T ]]; then",
    "  printf '%s\\n' \\",
    "    'authenticationmethods publickey' \\",
    "    'authorizedkeysfile /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys' \\",
    "    'passwordauthentication no' \\",
    "    'kbdinteractiveauthentication no' \\",
    "    'permittty no' \\",
    "    'x11forwarding no' \\",
    "    'allowagentforwarding no' \\",
    "    'allowtcpforwarding remote' \\",
    "    'allowstreamlocalforwarding no' \\",
    "    'gatewayports no' \\",
    "    'permitopen none' \\",
    "    'permitlisten 127.0.0.1:18100' \\",
    "    'forcecommand /bin/false' \\",
    "    'logingracetime 1'",
    "fi",
    "exit 0",
    "",
  ].join("\n"));
  executable(join(bin, "systemctl"), "printf 'systemctl %s\\n' \"$*\" >> /fixture/calls\n");
  executable(join(bin, "sleep"), "printf 'sleep %s\\n' \"$*\" >> /fixture/calls\n");
  executable(join(bin, "pkill"), "printf 'pkill %s\\n' \"$*\" >> /fixture/calls\ntouch /fixture/drained\n");
  executable(join(bin, "pgrep"), "test ! -e /fixture/drained\n");
  const setup = [
    "install -d -o root -g root -m 0755 /etc/ssh/sshd_config.d /trusted /trusted-inputs",
    "install -o root -g root -m 0755 /source/install_production_tunnel_account.sh /trusted/install_production_tunnel_account.sh",
    "printf 'ssh-ed25519 AAAAFIXTURE fixture\\n' > /trusted-inputs/id_ed25519.pub",
    "printf 'HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT=SHA256:approved\\n' > /trusted-inputs/trust.env",
    "chmod 0600 /trusted-inputs/id_ed25519.pub /trusted-inputs/trust.env",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin /trusted/install_production_tunnel_account.sh /trusted-inputs/id_ed25519.pub /trusted-inputs/trust.env",
    "grep -Fqx 'sleep 2' /fixture/calls",
    "grep -Fqx 'pkill -KILL -u henukit-getwork-tunnel' /fixture/calls",
    "grep -Fq 'permitopen=\"127.0.0.1:18100\"' /var/lib/henukit-getwork-tunnel/.ssh/authorized_keys",
    "grep -Fqx '    AllowStreamLocalForwarding no' /etc/ssh/sshd_config.d/60-henukit-getwork-tunnel.conf",
    "",
  ].join("\n");
  const result = run("docker", [
    "run", "--rm",
    "--volume", productionAccountInstaller + ":/source/install_production_tunnel_account.sh:ro",
    "--volume", fixture + ":/fixture",
    "debian:bookworm-slim", "bash", "-ceu", setup,
  ], { encoding: "utf8" });
  assert.equal(result.status, 0, `${result.stderr}\n${result.stdout}`);
  assert.match(result.stdout, /installed remote-forward-only account/);
});

test("local-signed and Actions node installers both succeed idempotently", () => {
  const fixture = mkdtempSync(join(tmpdir(), "getwork-install-twice-"));
  const stage = join(fixture, "stage-source");
  const runtime = join(fixture, "runtime-source");
  const bin = join(fixture, "bin");
  const state = join(fixture, "state");
  mkdirSync(stage);
  mkdirSync(runtime);
  mkdirSync(bin);
  mkdirSync(state);

  const releaseSha = "a".repeat(40);
  const imageConfig = "d".repeat(64) + ".json";
  const imageName = `henukit-getwork-mcp-${releaseSha}.docker.tar.gz`;
  const runtimeName = `henukit-runtime-${releaseSha}.tar.gz`;
  const manifestName = `henukit-release-${releaseSha}.manifest`;
  const actionsManifestName = `henukit-getwork-actions-${releaseSha}.manifest`;
  const actionsAttestationName = `henukit-getwork-actions-${releaseSha}.attestation.json`;
  const runtimeDeploy = join(runtime, "getwork-node-deploy");
  cpSync(deployAssets, runtimeDeploy, { recursive: true });
  writeFileSync(join(runtime, "RELEASE_SHA"), releaseSha + "\n");
  writeFileSync(join(fixture, "manifest.json"), JSON.stringify([{ Config: imageConfig }]));

  for (const [archive, cwd, entry] of [
    [join(stage, imageName), fixture, "manifest.json"],
    [join(stage, runtimeName), runtime, "."],
  ]) {
    const tarResult = run("tar", ["-czf", archive, "-C", cwd, entry], {
      encoding: "utf8",
      env: { ...process.env, COPYFILE_DISABLE: "1" },
    });
    assert.equal(tarResult.status, 0, tarResult.stderr);
  }
  const imageDigest = sha256(join(stage, imageName));
  const runtimeDigest = sha256(join(stage, runtimeName));
  writeFileSync(join(stage, imageName + ".sha256"), `${imageDigest}  ${imageName}\n`);
  writeFileSync(join(stage, runtimeName + ".sha256"), `${runtimeDigest}  ${runtimeName}\n`);
  writeFileSync(join(stage, manifestName), [
    "format=henukit-local-release-v1",
    `release_sha=${releaseSha}`,
    "source_ref=refs/heads/main",
    "builder_platform=linux/amd64",
    "signer=henukit-release",
    "signature_namespace=henukit-release",
    `artifact_sha256=${imageDigest}  ${imageName}`,
    `artifact_sha256=${runtimeDigest}  ${runtimeName}`,
    "",
  ].join("\n"));
  writeFileSync(join(stage, manifestName + ".sig"), "fixture-signature\n");
  writeFileSync(join(stage, actionsManifestName), [
    "format=henukit-getwork-actions-release-v1",
    `release_sha=${releaseSha}`,
    "source_repository=jry21223/HENU-Kit-DEV",
    "source_ref=refs/heads/main",
    "signer_workflow=.github/workflows/deploy-henukit.yml",
    "builder_platform=linux/amd64",
    `artifact_sha256=${imageDigest}  ${imageName}`,
    `artifact_sha256=${runtimeDigest}  ${runtimeName}`,
    "",
  ].join("\n"));
  writeFileSync(join(stage, actionsAttestationName), "fixture-attestation\n");
  writeFileSync(join(stage, "trusted_root.jsonl"), "fixture-trusted-root\n");
  writeFileSync(join(stage, "node.env"), [
    "HENUKIT_GETWORK_MEMORY_LIMIT=4g",
    "HENUKIT_GETWORK_TUNNEL_TARGET=henukit-getwork-tunnel@production.example",
    "HENUKIT_GETWORK_TUNNEL_PORT=22222",
    "",
  ].join("\n"));
  writeFileSync(
    join(stage, "mcp.env"),
    "GETWORK_MCP_ACCESS_TOKEN=0123456789abcdef0123456789abcdef\n",
  );
  writeFileSync(join(stage, "id_ed25519"), "fixture-private-key\n");
  writeFileSync(join(stage, "known_hosts"), "[production.example]:22222 fixture-host-key\n");
  for (const name of [
    imageName,
    imageName + ".sha256",
    runtimeName,
    runtimeName + ".sha256",
    manifestName,
    manifestName + ".sig",
    actionsManifestName,
    actionsAttestationName,
    "trusted_root.jsonl",
    "node.env",
    "mcp.env",
    "id_ed25519",
    "known_hosts",
  ]) chmodSync(join(stage, name), 0o600);

  executable(join(bin, "ssh-keygen"), [
    "case \"${1:-}\" in",
    "  -Y) cat >/dev/null; exit 0 ;;",
    "  -y) printf 'ssh-ed25519 AAAAFIXTURE fixture\\n'; exit 0 ;;",
    "  -F) exit 0 ;;",
    "  -lf)",
    "    if [[ \"${2:-}\" == *known_hosts ]]; then",
    "      printf '256 SHA256:host fixture (ED25519)\\n'",
    "    else",
    "      printf '256 SHA256:tunnel fixture (ED25519)\\n'",
    "    fi",
    "    exit 0",
    "    ;;",
    "esac",
    "exit 1",
    "",
  ].join("\n"));
  executable(join(bin, "jq"), [
    "if [[ \"$*\" == *\"invalid verification count\"* ]]; then printf 'ok\\n'; exit 0; fi",
    "cat >/dev/null",
    `printf '${imageConfig}\\n'`,
    "",
  ].join("\n"));
  const git = join(fixture, "git");
  executable(git, [
    `printf '${releaseSha}\\trefs/heads/main\\n'`,
    "",
  ].join("\n"));
  const gh = join(fixture, "gh");
  executable(gh, "printf '%s\\n' \"$*\" >> /fixture/gh-calls\nprintf '[{}]\\n'\n");
  executable(join(bin, "docker"), [
    "case \"${1:-} ${2:-}\" in",
    "  'load --input') exit 0 ;;",
    "  'image inspect')",
    "    if [[ \"$*\" == *Architecture* ]]; then printf 'linux/amd64\\n'; else printf 'sha256:${IMAGE_ID}\\n'; fi",
    "    exit 0",
    "    ;;",
    "  'network inspect') exit 1 ;;",
    "esac",
    "exit 0",
    "",
  ].join("\n").replace("${IMAGE_ID}", "d".repeat(64)));
  executable(join(bin, "iptables"), "exit 0\n");
  executable(join(bin, "systemctl"), [
    "command=${1:-}; shift || true",
    "case \"$command\" in",
    "  is-enabled) test -e \"/fixture/state/enabled-${1:-}\" ;;",
    "  is-active)",
    "    if test -e \"/fixture/state/active-${1:-}\"; then printf 'active\\n'; exit 0; fi",
    "    printf 'inactive\\n'; exit 3",
    "    ;;",
    "  enable) for unit in \"$@\"; do touch \"/fixture/state/enabled-$unit\"; done ;;",
    "  restart) touch \"/fixture/state/active-${1:-}\" ;;",
    "  daemon-reload) ;;",
    "  *) exit 0 ;;",
    "esac",
    "",
  ].join("\n"));

  writeFileSync(join(fixture, "allowed-signers"), "henukit-release fixture\n");
  writeFileSync(join(fixture, "trust.env"), [
    "HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT=SHA256:tunnel",
    "HENUKIT_GETWORK_HOST_KEY_FINGERPRINT=SHA256:host",
    "",
  ].join("\n"));
  writeFileSync(join(fixture, "actions-trust.env"), [
    "HENUKIT_GETWORK_TUNNEL_KEY_FINGERPRINT=SHA256:tunnel",
    "HENUKIT_GETWORK_HOST_KEY_FINGERPRINT=SHA256:host",
    `HENUKIT_GETWORK_SIGSTORE_TRUSTED_ROOT_SHA256=${sha256(join(stage, "trusted_root.jsonl"))}`,
    "",
  ].join("\n"));
  chmodSync(join(fixture, "allowed-signers"), 0o600);
  chmodSync(join(fixture, "trust.env"), 0o600);
  chmodSync(join(fixture, "actions-trust.env"), 0o600);

  const setup = [
    "trap 'echo \"fixture setup failed at line $LINENO: $BASH_COMMAND\" >&2' ERR",
    "install -d -o root -g root -m 0755 /trusted /trusted-stage /trusted-inputs /usr/local/libexec",
    "install -o root -g root -m 0755 /fixture/git /usr/bin/git",
    "install -o root -g root -m 0755 /fixture/gh /usr/bin/gh",
    "install -o root -g root -m 0755 /source/install_node.sh /trusted/install_node.sh",
    "cp -a /fixture/stage-source/. /trusted-stage/",
    "chown -R root:root /trusted-stage",
    "install -o root -g root -m 0600 /fixture/allowed-signers /trusted-inputs/allowed-signers",
    "install -o root -g root -m 0600 /fixture/trust.env /trusted-inputs/trust.env",
    "install -o root -g root -m 0600 /fixture/actions-trust.env /trusted-inputs/actions-trust.env",
    `command=(/trusted/install_node.sh --sha ${releaseSha} --stage-dir /trusted-stage --allowed-signers /trusted-inputs/allowed-signers --trust-file /trusted-inputs/trust.env)`,
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin \"${command[@]}\" | tee /fixture/first.out",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin \"${command[@]}\" | tee /fixture/second.out",
    `actions_command=(/trusted/install_node.sh --sha ${releaseSha} --stage-dir /trusted-stage --actions-attestation /trusted-stage/${actionsAttestationName} --actions-custom-trusted-root /trusted-stage/trusted_root.jsonl --trust-file /trusted-inputs/actions-trust.env)`,
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin \"${actions_command[@]}\" | tee /fixture/actions-first.out",
    "PATH=/fixture/bin:/usr/sbin:/usr/bin:/sbin:/bin \"${actions_command[@]}\" | tee /fixture/actions-second.out",
    "test -s /fixture/first.out",
    "test -s /fixture/second.out",
    "test -s /fixture/actions-first.out",
    "test -s /fixture/actions-second.out",
    "grep -Fq -- '--custom-trusted-root' /fixture/gh-calls",
    "grep -Fq -- 'trusted_root.jsonl' /fixture/gh-calls",
    "test \"$(find /var/lib/henukit-getwork-backups -mindepth 1 -maxdepth 1 -type d | wc -l)\" -eq 4",
    "test \"$(getent passwd henukit-getwork-tunnel | cut -d: -f6)\" = /var/lib/henukit-getwork-tunnel",
    "test \"$(getent shadow henukit-getwork-tunnel | cut -d: -f2)\" = NP",
    "",
  ].join("\n");
  const result = run("docker", [
    "run", "--rm",
    "--volume", installer + ":/source/install_node.sh:ro",
    "--volume", fixture + ":/fixture",
    "debian:bookworm-slim", "bash", "-ceu", setup,
  ], { encoding: "utf8", timeout: 120_000 });
  assert.equal(result.status, 0, `${result.stderr}\n${result.stdout}`);
  assert.match(readFileSync(join(fixture, "first.out"), "utf8"), /installed henukit-getwork-mcp/);
  assert.match(readFileSync(join(fixture, "second.out"), "utf8"), /installed henukit-getwork-mcp/);
  assert.match(readFileSync(join(fixture, "actions-first.out"), "utf8"), /verified github-actions provenance/);
  assert.match(readFileSync(join(fixture, "actions-second.out"), "utf8"), /verified github-actions provenance/);
});
