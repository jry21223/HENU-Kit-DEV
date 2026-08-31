# Remote Job Source MCP deployment

This directory installs only the browser-bearing, read-only Job Source MCP on
an always-on WSL2 node. Career Profiles, matching, durable search state,
results, and Opportunity Digest delivery remain on HENUKit production.

## Trust and network contract

- Never execute deployment code as root from a user-owned checkout. Bootstrap
  only after the runtime archive is bound to the exact current `origin/main`
  SHA by either the reviewed GitHub Actions attestation or the existing local
  release signer.
- Load the `henukit-getwork-mcp:<sha>` Linux/amd64 image only after verifying
  its checksum and provenance through one of those release trust paths.
- Bind the MCP only to WSL loopback at `127.0.0.1:8100`.
- Use a dedicated `henukit-getwork-tunnel` identity on both hosts. The
  production root private key must never be copied to WSL.
- Keep the existing deployment-owned MCP bearer token in
  `/etc/henukit-getwork/mcp.env`, a root-owned regular non-symlink file at mode
  `0600`. Do not print it in logs or shell history.
- Copy the already-approved production host-key record into
  `/etc/henukit-getwork/tunnel/known_hosts`; `ssh-keyscan` output alone is not a
  trust decision.

The production `authorized_keys` entry for the dedicated public key is
restricted to the reviewed reverse listener:

```text
restrict,port-forwarding,permitopen="127.0.0.1:18100",permitlisten="127.0.0.1:18100",command="/bin/false" ssh-ed25519 <dedicated-public-key> henukit-getwork-tunnel
```

The key line alone is not sufficient: OpenSSH's per-key `port-forwarding`
option enables both directions. Install the dedicated production account with
`install_production_tunnel_account.sh <root-owned-public-key> <approved-trust.env>`
from the same provenance-verified runtime. It requires the public key to match
the approved tunnel-key fingerprint. Its account-level `Match User` block
requires public-key authentication, disables password and keyboard-interactive
authentication, forces a no-shell account, sets `AllowTcpForwarding remote`,
`AllowStreamLocalForwarding no`, `PermitOpen none`,
`PermitListen 127.0.0.1:18100`, and `GatewayPorts no`.
The same block explicitly sets `PasswordAuthentication no` and
`KbdInteractiveAuthentication no`.
Together those gates permit only the reviewed `-R` listener and reject every
`-L` request. The installer replaces the account's `authorized_keys` with
the single dedicated key, validates `sshd -t` and `sshd -T -C`, preserves a
metadata-complete backup, and then reloads SSH.
Before promoting the new key, it waits beyond the prior bounded
`LoginGraceTime`, so no pre-reload unauthenticated sshd child can apply the old
forwarding policy to the new credential. It then terminates and proves the
absence of authenticated processes for this dedicated account before key
promotion; the supervised WSL tunnel reconnects under the new policy.
The account's password field is set to OpenSSH's documented non-password value
`NP`; a Linux `!` lock is deliberately not used because sshd rejects such an
account before public-key authentication.

## WSL installation

### GitHub Actions attestation (preferred)

The `attest-getwork-wsl-release` job runs only for `refs/heads/main` outside
pull requests. It downloads only the exact-SHA getWork image and runtime
artifacts, creates `henukit-getwork-actions-<sha>.manifest`, and uses GitHub
OIDC/Sigstore through `actions/attest` to attest that manifest. No long-lived
release private key is copied into GitHub Actions.

Use one successful release-workflow run ID to download the exact
`henukit-getwork-mcp-<sha>`, `henukit-runtime-<sha>`, and
`henukit-getwork-actions-provenance-<sha>` artifacts. Copy their six files,
plus `node.env`, `mcp.env`, the dedicated `id_ed25519`, and approved
`known_hosts`, and a connected-host-generated `trusted_root.jsonl`, into one
root-owned non-symlink staging directory with a fully root-trusted ancestor
chain. Install GitHub CLI from the OS package as the root-owned `/usr/bin/gh`; a
user-owned `gh` binary is not a trust root. The trusted-root file must be
generated on a connected, trusted host with the same OS GitHub CLI
(`gh attestation trusted-root`), transferred out of band, and checked for the
expected SHA-256 before it is installed at mode `0400`. Record that digest as
`HENUKIT_GETWORK_SIGSTORE_TRUSTED_ROOT_SHA256` in the root-owned `trust.env`
alongside the two SSH fingerprints. The installer and verifier both require the
digest to match the exact trusted-root bytes. It is the Sigstore trust root for
this offline WSL verification; it is not a release artifact.

Before executing any release code as root, use the OS GitHub CLI to verify the
GitHub Actions attestation and exact workflow identity, then bind both archive
digests to the attested manifest:

```bash
release_sha="paste-the-40-character-merged-main-sha"
run_id="paste-the-successful-actions-run-id"
stage="/root/henukit-getwork-stage"
verified_runtime="/root/henukit-getwork-runtime-${release_sha}"
node_env_source="/root/henukit-getwork-inputs/node.env"
mcp_env_source="/root/henukit-getwork-inputs/mcp.env"
tunnel_key_source="/root/henukit-getwork-inputs/id_ed25519"
known_hosts_source="/root/henukit-getwork-inputs/known_hosts"
trusted_root_source="/root/henukit-getwork-inputs/trusted_root.jsonl"
current_main_sha="$(cd / && env -i PATH=/usr/bin:/bin HOME=/var/empty XDG_CONFIG_HOME=/var/empty GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0 /usr/bin/timeout 60s /usr/bin/git -c credential.helper= -c protocol.allow=never -c protocol.https.allow=always ls-remote --exit-code https://github.com/jry21223/HENU-Kit-DEV.git refs/heads/main | awk 'NR == 1 { print $1 }')"
test "$current_main_sha" = "$release_sha"
run_state="$(timeout 60s /usr/bin/gh run view "$run_id" --repo jry21223/HENU-Kit-DEV --json conclusion,headSha,status,workflowName --jq '[.headSha,.status,.conclusion,.workflowName] | join(" ")')"
test "$run_state" = "$release_sha completed success Build HENU Kit release artifacts"
download_dir="$(mktemp -d)"
trap 'rm -rf -- "$download_dir"' EXIT
timeout 900s /usr/bin/gh run download "$run_id" --repo jry21223/HENU-Kit-DEV \
  --name "henukit-getwork-mcp-${release_sha}" --dir "$download_dir"
timeout 900s /usr/bin/gh run download "$run_id" --repo jry21223/HENU-Kit-DEV \
  --name "henukit-runtime-${release_sha}" --dir "$download_dir"
timeout 900s /usr/bin/gh run download "$run_id" --repo jry21223/HENU-Kit-DEV \
  --name "henukit-getwork-actions-provenance-${release_sha}" --dir "$download_dir"
test "$(find "$download_dir" -mindepth 1 -maxdepth 1 -type f | wc -l | tr -d '[:space:]')" = 6
test ! -e "$stage"
sudo install -d -o root -g root -m 0700 "$stage"
sudo find "$download_dir" -mindepth 1 -maxdepth 1 -type f \
  -exec install -o root -g root -m 0400 {} "$stage/" \;
sudo install -o root -g root -m 0400 "$node_env_source" "$stage/node.env"
sudo install -o root -g root -m 0400 "$mcp_env_source" "$stage/mcp.env"
sudo install -o root -g root -m 0400 "$tunnel_key_source" "$stage/id_ed25519"
sudo install -o root -g root -m 0400 "$known_hosts_source" "$stage/known_hosts"
sudo install -o root -g root -m 0400 "$trusted_root_source" "$stage/trusted_root.jsonl"
manifest="$stage/henukit-getwork-actions-${release_sha}.manifest"
attestation="$stage/henukit-getwork-actions-${release_sha}.attestation.json"
sudo timeout 60s env -u GH_TOKEN -u GITHUB_TOKEN /usr/bin/gh attestation verify \
  "$manifest" \
  --repo jry21223/HENU-Kit-DEV \
  --bundle "$attestation" \
  --signer-workflow jry21223/HENU-Kit-DEV/.github/workflows/deploy-henukit.yml \
  --source-ref refs/heads/main \
  --source-digest "$release_sha" \
  --predicate-type https://github.com/jry21223/HENU-Kit-DEV/attestations/getwork-actions-release-v1 \
  --deny-self-hosted-runners \
  --custom-trusted-root "$stage/trusted_root.jsonl" \
  --format json >/dev/null
image="henukit-getwork-mcp-${release_sha}.docker.tar.gz"
runtime="henukit-runtime-${release_sha}.tar.gz"
image_digest="$(sudo sha256sum "$stage/$image" | awk '{print $1}')"
runtime_digest="$(sudo sha256sum "$stage/$runtime" | awk '{print $1}')"
sudo grep -Fqx "artifact_sha256=${image_digest}  ${image}" "$manifest"
sudo grep -Fqx "artifact_sha256=${runtime_digest}  ${runtime}" "$manifest"
sudo install -d -o root -g root -m 0700 "$verified_runtime"
sudo tar --no-same-owner -xzf "$stage/$runtime" -C "$verified_runtime"
```

Run only the installer from that attestation-verified extraction. The installer
repeats the exact repository, workflow, main ref, source SHA, attestation, and
archive digest verification before loading Docker:

```bash
sudo "$verified_runtime/getwork-node-deploy/install_node.sh" \
  --sha "$release_sha" \
  --stage-dir "$stage" \
  --actions-attestation "$attestation" \
  --actions-custom-trusted-root "$stage/trusted_root.jsonl" \
  --trust-file /etc/henukit-getwork-bootstrap/trust.env
```

### Locally signed fallback

1. Copy the signed release manifest/signature, getWork image archive/checksum,
   runtime archive/checksum, `node.env`, `mcp.env`, dedicated `id_ed25519`,
   and approved `known_hosts` into a root-owned staging tree whose complete
   ancestor chain is not group/world writable. Separately install the approved
   release signer set and `trust.env` containing the independently reviewed
   dedicated-key and production-host-key SHA256 fingerprints.
2. Before executing any release code as root, use the OS `ssh-keygen -Y verify`
   command with the root-owned signer set, confirm the signed manifest binds
   both archive digests to the exact main SHA, then extract the runtime archive
   into a fresh root-only directory. Run only the installer from that verified
   extraction:

   ```bash
   release_sha="paste-the-40-character-merged-main-sha"
   stage="/root/henukit-getwork-stage"
   verified_runtime="/root/henukit-getwork-runtime-${release_sha}"
   manifest="$stage/henukit-release-${release_sha}.manifest"
   current_main_sha="$(cd / && env -i PATH=/usr/bin:/bin HOME=/var/empty XDG_CONFIG_HOME=/var/empty GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0 /usr/bin/timeout 60s /usr/bin/git -c credential.helper= -c protocol.allow=never -c protocol.https.allow=always ls-remote --exit-code https://github.com/jry21223/HENU-Kit-DEV.git refs/heads/main | awk 'NR == 1 { print $1 }')"
   test "$current_main_sha" = "$release_sha"
   sudo ssh-keygen -Y verify \
     -f /etc/henukit-getwork-bootstrap/release-signers \
     -I henukit-release -n henukit-release \
     -s "${manifest}.sig" < "$manifest"
   image="henukit-getwork-mcp-${release_sha}.docker.tar.gz"
   runtime="henukit-runtime-${release_sha}.tar.gz"
   image_digest="$(sudo sha256sum "$stage/$image" | awk '{print $1}')"
   runtime_digest="$(sudo sha256sum "$stage/$runtime" | awk '{print $1}')"
   sudo grep -Fqx "artifact_sha256=${image_digest}  ${image}" \
     "$manifest"
   sudo grep -Fqx "artifact_sha256=${runtime_digest}  ${runtime}" \
     "$manifest"
   sudo install -d -o root -g root -m 0700 "$verified_runtime"
   sudo tar --no-same-owner -xzf "$stage/$runtime" -C "$verified_runtime"
   ```

   Do not replace this bootstrap with `sudo` execution of any checkout script.

   ```bash
   release_sha="paste-the-40-character-merged-main-sha"
   stage="/root/henukit-getwork-stage"
   verified_runtime="/root/henukit-getwork-runtime-${release_sha}"
   current_main_sha="$(cd / && env -i PATH=/usr/bin:/bin HOME=/var/empty XDG_CONFIG_HOME=/var/empty GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0 /usr/bin/timeout 60s /usr/bin/git -c credential.helper= -c protocol.allow=never -c protocol.https.allow=always ls-remote --exit-code https://github.com/jry21223/HENU-Kit-DEV.git refs/heads/main | awk 'NR == 1 { print $1 }')"
   test "$current_main_sha" = "$release_sha"
   sudo "$verified_runtime/getwork-node-deploy/install_node.sh" \
     --sha "$release_sha" \
     --stage-dir "$stage" \
     --allowed-signers /etc/henukit-getwork-bootstrap/release-signers \
     --trust-file /etc/henukit-getwork-bootstrap/trust.env
   ```

   The installer repeats provenance/digest verification, copies every input
   into a fresh root-only snapshot, extracts and installs code only from the
   verified runtime, normalizes the no-login account, and verifies the image ID
   encoded inside the Docker archive before starting it. The flow is idempotent
   and re-running it is supported.
   The private key stays root-owned at mode `0600`; systemd exposes a read-only
   transient credential to the no-login SSH process.
   The crawler is read-only, uses a dedicated bridge, and host firewall rules
   reject RFC1918, link-local, CGNAT, loopback, multicast, and reserved egress.
3. Run the installed fail-closed verifier:

   ```bash
   sudo python3 /usr/local/libexec/henukit-getwork-deploy/verify_node.py \
     --sha "$release_sha" \
     --token-file /etc/henukit-getwork/mcp.env \
     --artifact-file "/var/lib/henukit-getwork-artifacts/henukit-getwork-mcp-${release_sha}.docker.tar.gz"
   ```

   For a GitHub Actions installation, run the complete verifier command with
   the Actions provenance arguments:

   ```bash
   sudo python3 /usr/local/libexec/henukit-getwork-deploy/verify_node.py \
     --sha "$release_sha" \
     --token-file /etc/henukit-getwork/mcp.env \
     --artifact-file "/var/lib/henukit-getwork-artifacts/henukit-getwork-mcp-${release_sha}.docker.tar.gz" \
         --provenance-mode github-actions \
         --actions-attestation-file "/var/lib/henukit-getwork-artifacts/henukit-getwork-actions-${release_sha}.attestation.json" \
         --actions-custom-trusted-root-file /etc/henukit-getwork/trusted_root.jsonl \
         --manifest-file "/var/lib/henukit-getwork-artifacts/henukit-getwork-actions-${release_sha}.manifest"
   ```

The verifier opens secrets without following symlinks, checks every trusted
parent, exact signed-manifest or Actions-attestation provenance, approved
tunnel/host fingerprints,
archive-to-image identity, normalized account state, installed unit/helper
content, the live read-only container and firewall/network rules, both active
units, strict bounded MCP envelopes, pinned upstream health, the exact read-only
tool surface, all 18 pinned sources, and a real `crawl_jobs` call without
printing the bearer token.

## Production activation

Before changing production, record the active image SHA, healthy rollback
release, Docker host-gateway address, free disk, existing listeners, service
states, and root-owned environment metadata. Back up the complete environment
file with checksum, owner, group, mode, and symlink evidence.

The fixed-SHA runtime starts the small `getwork-mcp-relay` process from the
Career image in host-network mode. It binds only the configured private Docker
gateway address, forwards only `/mcp` and `/healthz` to production loopback
`127.0.0.1:18100`, and returns a stable 503 while the SSH tunnel is unavailable.
Career resolves the stable `getwork-mcp-relay` name to the host gateway. The
production watcher installs and verifies INPUT/OUTPUT rules that allow only the
HENUKit Compose subnet plus root's bounded verifier; other host users and local
containers are rejected. Career continues to send the existing bearer
credential. The browser-bearing `getwork-mcp` profile must
remain absent from the production runtime.

Do not activate until the implementation is merged, required review and CI are
green, the exact current `origin/main` artifacts are verified, the production
release has a healthy rollback point, and the formal free-disk gate passes.

## Reconnect and acceptance

After activation, run `verify_reconnect.sh stop` with the verifier arguments.
It verifies the node and leaves the transport stopped. Before continuing, the
production operator must record a bounded relay probe returning 503 and prove
that `henukit-getwork-mcp-1` is absent:

```bash
bridge="$(docker network inspect bridge --format '{{(index .IPAM.Config 0).Gateway}}')"
test "$(curl --max-time 5 --silent --output /dev/null --write-out '%{http_code}' \
  "http://${bridge}:18101/healthz")" = 503
test -z "$(docker ps -a --filter 'name=^/henukit-getwork-mcp-1$' --format '{{.Names}}')"
```

Only after that interruption evidence exists, run `verify_reconnect.sh start`. It starts
the tunnel, kills the supervised SSH process once, proves systemd replaces it,
and re-runs the complete verifier. Production must then pass the watcher's
bounded authenticated tool/source probe before one actor-scoped Career scan.
Record the complete source count,
per-source states, terminal status, and at least one normalized official Job
Opportunity when upstream data is available. A container health response alone
is not acceptance.

## Rollback

If any mandatory check fails, stop the WSL tunnel, invoke
`rollback_node.sh <exact-backup-directory>` for WSL files, then restore the production
environment backup and previous fixed-SHA release through the existing atomic
release procedure, and verify the previous image set and public Career state.
The formal WSL rollback restores recorded unit enablement/activity, prior
account metadata, and prior network presence; it removes the dedicated account
or network only when the provenance-verified installation attempt created it.
Keep the prior release, configuration backup, dedicated public-key record, and
WSL image available until acceptance completes. Do not delete releases,
images, keys, accounts, databases, or volumes outside that recorded rollback.
