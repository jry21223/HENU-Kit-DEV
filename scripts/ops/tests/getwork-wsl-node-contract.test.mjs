import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

const deployRoot = new URL("../../../services/getwork-mcp/deploy/", import.meta.url);

test("WSL runs the pinned crawler only on loopback under systemd", () => {
  const unit = readFileSync(new URL("systemd/henukit-getwork-mcp.service", deployRoot), "utf8");

  assert.match(unit, /^Requires=docker\.service$/m);
  assert.match(unit, /--pull never/);
  assert.match(unit, /--publish 127\.0\.0\.1:8100:8100/);
  assert.match(unit, /--env-file \/etc\/henukit-getwork\/mcp\.env/);
  assert.match(unit, /henukit-getwork-mcp:\$\{HENUKIT_GETWORK_RELEASE_SHA\}/);
  assert.match(unit, /--cap-drop ALL/);
  assert.match(unit, /--security-opt no-new-privileges/);
  assert.match(unit, /--read-only/);
  assert.match(unit, /--user 65532:65532/);
  assert.match(unit, /--network henukit-getwork-egress/);
  assert.doesNotMatch(unit, /--privileged|0\.0\.0\.0:8100|--network host/);
});

test("WSL tunnel identity can only establish the reviewed reverse forward", () => {
  const unit = readFileSync(new URL("systemd/henukit-getwork-tunnel.service", deployRoot), "utf8");

  assert.match(unit, /^User=henukit-getwork-tunnel$/m);
  assert.match(unit, /BatchMode=yes/);
  assert.match(unit, /StrictHostKeyChecking=yes/);
  assert.match(unit, /IdentitiesOnly=yes/);
  assert.match(unit, /ExitOnForwardFailure=yes/);
  assert.match(unit, /ServerAliveInterval=15/);
  assert.match(unit, /ServerAliveCountMax=4/);
  assert.match(unit, /KexAlgorithms=diffie-hellman-group14-sha256/);
  assert.match(unit, /LoadCredential=tunnel-key:.*id_ed25519/);
  assert.match(unit, /-i \$\{CREDENTIALS_DIRECTORY\}\/tunnel-key/);
  assert.match(unit, /-R 127\.0\.0\.1:18100:127\.0\.0\.1:8100/);
  assert.doesNotMatch(unit, /StrictHostKeyChecking=no|User=root|ProxyCommand|ProxyJump/);
});

test("deployment instructions preserve a remote-forward-only production account", () => {
  const runbook = readFileSync(new URL("README.md", deployRoot), "utf8");

  assert.match(
    runbook,
    /restrict,port-forwarding,permitopen="127\.0\.0\.1:18100",permitlisten="127\.0\.0\.1:18100",command="\/bin\/false"/,
  );
  assert.match(runbook, /AllowTcpForwarding remote/);
  assert.match(runbook, /PasswordAuthentication no/);
  assert.match(runbook, /idempotent/i);
  assert.match(runbook, /interruption/i);
  assert.match(runbook, /root private key.*never.*WSL/is);
  assert.match(runbook, /verify_node\.py/);
  assert.match(runbook, /rollback/i);
});

test("deployment includes idempotent install, rollback, reconnect, and production account gates", () => {
  for (const name of [
    "install_node.sh",
    "rollback_node.sh",
    "verify_reconnect.sh",
    "install_production_tunnel_account.sh",
    "henukit-getwork-egress",
  ]) {
    assert.equal(existsSync(new URL(name, deployRoot)), true, `${name} must exist`);
  }
  const install = readFileSync(new URL("install_node.sh", deployRoot), "utf8");
  const rollback = readFileSync(new URL("rollback_node.sh", deployRoot), "utf8");
  const reconnect = readFileSync(new URL("verify_reconnect.sh", deployRoot), "utf8");
  const production = readFileSync(
    new URL("install_production_tunnel_account.sh", deployRoot),
    "utf8",
  );

  assert.match(install, /getent passwd henukit-getwork-tunnel/);
  assert.match(install, /cp -a/);
  assert.match(install, /service-state/);
  assert.match(install, /sha256sum/);
  assert.match(install, /ssh-keygen -Y verify/);
  assert.match(install, /henukit-runtime-/);
  assert.match(install, /trusted_root_chain/);
  assert.doesNotMatch(install, / \+ /);
  assert.match(rollback, /cp -a/);
  assert.match(rollback, /unit_\$\{key\}_enabled/);
  assert.match(rollback, /network_present/);
  assert.match(rollback, /account_present/);
  assert.match(reconnect, /systemctl stop henukit-getwork-tunnel\.service/);
  assert.match(reconnect, /systemctl start henukit-getwork-tunnel\.service/);
  assert.match(reconnect, /mode.*stop/);
  assert.match(reconnect, /mode.*start/);
  assert.match(production, /AllowTcpForwarding remote/);
  assert.match(production, /PasswordAuthentication no/);
  assert.match(production, /KbdInteractiveAuthentication no/);
  assert.match(production, /AllowStreamLocalForwarding no/);
  assert.match(production, /AuthorizedKeysFile \/var\/lib\/henukit-getwork-tunnel\/\.ssh\/authorized_keys/);
  assert.match(production, /usermod --password NP/);
  assert.match(production, /sshd -t/);
  assert.match(production, /sshd -T -C/);
  const candidateInstall = production.indexOf(
    'install -o root -g "$account" -m 0640 "$authorized_tmp" "$authorized_candidate"',
  );
  const policyInstall = production.indexOf(
    'install -o root -g root -m 0644 "$dropin_tmp" "$dropin"',
  );
  const policyReload = production.indexOf("systemctl reload sshd", policyInstall);
  const preauthDrain = production.indexOf('sleep "$((prior_login_grace + 1))"');
  const authenticatedDrain = production.indexOf('pkill -KILL -u "$account"');
  const authenticatedProof = production.indexOf('pgrep -u "$account"', authenticatedDrain);
  const keyActivation = production.indexOf(
    'mv -f -- "$authorized_candidate" "$home/.ssh/authorized_keys"',
  );
  assert.ok(candidateInstall >= 0 && candidateInstall < policyInstall);
  assert.ok(policyInstall < policyReload && policyReload < preauthDrain);
  assert.ok(preauthDrain < authenticatedDrain && authenticatedDrain < authenticatedProof);
  assert.ok(authenticatedProof < keyActivation);
  assert.match(production, /prior LoginGraceTime must be between 1 and 300 seconds/);
});
