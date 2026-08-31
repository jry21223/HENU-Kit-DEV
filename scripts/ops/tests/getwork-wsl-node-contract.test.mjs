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

test("the Docker bridge name fits the Linux interface-name limit", () => {
  const egress = readFileSync(new URL("henukit-getwork-egress", deployRoot), "utf8");
  const verifier = readFileSync(new URL("verify_node.py", deployRoot), "utf8");
  const match = egress.match(/^bridge=([^\s]+)$/m);

  assert.ok(match, "egress helper must declare one fixed bridge name");
  assert.ok(
    Buffer.byteLength(match[1], "utf8") <= 15,
    `bridge name exceeds Linux IFNAMSIZ: ${match[1]}`,
  );
  assert.match(
    verifier,
    new RegExp(`com\\.docker\\.network\\.bridge\\.name.*${match[1]}`),
    "runtime verifier must require the same bridge name",
  );
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
  assert.match(runbook, /GitHub Actions attestation/);
  assert.match(runbook, /sudo timeout 60s env -u GH_TOKEN -u GITHUB_TOKEN \/usr\/bin\/gh attestation verify/);
  assert.match(runbook, /--repo jry21223\/HENU-Kit-DEV/);
  assert.match(
    runbook,
    /--signer-workflow jry21223\/HENU-Kit-DEV\/\.github\/workflows\/deploy-henukit\.yml/,
  );
  assert.match(runbook, /--source-ref refs\/heads\/main/);
  assert.match(runbook, /--source-digest "\$release_sha"/);
  assert.match(
    runbook,
    /--predicate-type https:\/\/github\.com\/jry21223\/HENU-Kit-DEV\/attestations\/getwork-actions-release-v1/,
  );
  assert.match(runbook, /--deny-self-hosted-runners/);
  assert.match(runbook, /--actions-attestation/);
  assert.match(runbook, /--actions-custom-trusted-root/);
  assert.match(runbook, /--current-main-sha-file/);
  assert.match(runbook, /--custom-trusted-root/);
  assert.match(
    runbook,
    /\/usr\/bin\/git[\s\S]*ls-remote --exit-code https:\/\/github\.com\/jry21223\/HENU-Kit-DEV\.git refs\/heads\/main/,
  );
  assert.match(runbook, /env -i PATH=\/usr\/bin:\/bin[\s\S]*GIT_CONFIG_NOSYSTEM=1[\s\S]*GIT_CONFIG_GLOBAL=\/dev\/null/);
  assert.match(runbook, /test "\$current_main_sha" = "\$release_sha"/);
  assert.match(
    runbook,
    /gh run view "\$run_id"[\s\S]*conclusion[\s\S]*headSha[\s\S]*status[\s\S]*workflowName/,
  );
  for (const artifact of [
    "henukit-getwork-mcp-",
    "henukit-runtime-",
    "henukit-getwork-actions-provenance-",
  ]) {
    assert.match(
      runbook,
      new RegExp(`gh run download "\\$run_id"[\\s\\S]*--name "${artifact}`),
    );
  }
  for (const input of ["node.env", "mcp.env", "id_ed25519", "known_hosts", "trusted_root.jsonl", "main-ref.env"]) {
    assert.match(
      runbook,
      new RegExp(`install -o root -g root -m 0400[\\s\\S]*"\\$stage/${input.replaceAll(".", "\\.")}"`),
    );
  }
  assert.match(
    runbook,
    /verify_node\.py[\s\S]*--provenance-mode github-actions[\s\S]*--actions-attestation-file[\s\S]*--actions-custom-trusted-root-file[\s\S]*--current-main-sha-file/,
  );
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

test("the WSL installer accepts only an exact-main GitHub Actions attestation or the existing local signer", () => {
  const install = readFileSync(new URL("install_node.sh", deployRoot), "utf8");

  assert.match(install, /--actions-attestation/);
  assert.match(install, /--actions-custom-trusted-root/);
  assert.match(install, /--current-main-sha-file/);
  assert.match(install, /actions_repository=jry21223\/HENU-Kit-DEV/);
  assert.match(
    install,
    /actions_signer_workflow=jry21223\/HENU-Kit-DEV\/\.github\/workflows\/deploy-henukit\.yml/,
  );
  assert.match(install, /actions_source_ref=refs\/heads\/main/);
  assert.match(install, /gh_bin=\/usr\/bin\/gh/);
  assert.match(install, /trusted_root_file "\$gh_bin"/);
  assert.match(
    install,
    /while :; do[\s\S]*stat -c %u "\$current"[\s\S]*"\$current" == \/[\s\S]*break/,
  );
  assert.match(
    install,
    /"\$gh_bin" attestation verify "\$stage_dir\/\$manifest"/,
  );
  assert.match(install, /--custom-trusted-root "\$actions_custom_trusted_root"/);
  assert.match(install, /--repo "\$actions_repository"/);
  assert.match(install, /--bundle "\$stage_dir\/\$actions_attestation"/);
  assert.match(install, /--signer-workflow "\$actions_signer_workflow"/);
  assert.match(install, /--source-ref "\$actions_source_ref"/);
  assert.match(install, /--source-digest "\$release_sha"/);
  assert.match(
    install,
    /actions_predicate_type=https:\/\/github\.com\/jry21223\/HENU-Kit-DEV\/attestations\/getwork-actions-release-v1/,
  );
  assert.match(install, /--predicate-type "\$actions_predicate_type"/);
  assert.match(install, /--deny-self-hosted-runners/);
  assert.match(install, /env -u GH_TOKEN -u GITHUB_TOKEN/);
  assert.match(install, /current_main_sha/);
  assert.match(install, /github\.com\/jry21223\/HENU-Kit-DEV\.git/);
  assert.match(
    install,
    /env -i PATH=\/usr\/bin:\/bin HOME=\/var\/empty XDG_CONFIG_HOME=\/var\/empty[\s\S]*GIT_CONFIG_NOSYSTEM=1[\s\S]*GIT_CONFIG_GLOBAL=\/dev\/null/,
  );
  assert.equal(
    (install.match(/assert_current_main/g) ?? []).length,
    3,
    "current main is checked before provenance work and again before activation",
  );
  assert.match(install, /format=henukit-getwork-actions-release-v1/);
  assert.match(install, /source_repository=\$\{actions_repository\}/);
  assert.match(install, /signer_workflow=\.github\/workflows\/deploy-henukit\.yml/);
  assert.match(install, /choose exactly one release provenance mode/);
  assert.match(install, /ssh-keygen -Y verify/);
});
