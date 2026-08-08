# HENU Kit fixed-artifact deployment

HENU Kit production is deployed from fixed Docker images and a runtime tarball
built either by GitHub Actions or by the controlled WSL Linux/amd64 builder
described below. The host does not compile the repository and does not read
secrets from the release artifact.

## Release procedure

1. Select one successful `Build HENU Kit release artifacts` run for the exact full SHA. Download every image archive, its `.sha256` file, and `henukit-runtime-<sha>.tar.gz` with its checksum.
2. Verify each checksum before loading anything:

   ```bash
   sha256sum -c henukit-<image>-<sha>.docker.tar.gz.sha256
   sha256sum -c henukit-runtime-<sha>.tar.gz.sha256
   docker load < henukit-<image>-<sha>.docker.tar.gz
   ```

3. Extract the runtime tarball into a new release directory. Keep the existing `.env.henukit` outside that directory and make read-only, checksum-verified backups of both the `platform` and `account_portfolio` databases before applying a migration or enabling Account Portfolio traffic. The first Account Portfolio release may safely create an otherwise empty `account_portfolio` database before taking that baseline backup; rollback never drops that database or its volume. Before activation, place a unique `ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY=$(openssl rand -base64 32)` in that root-owned environment file. It is decoded to exactly 32 bytes only inside Account Portfolio, must not equal any Portal or Console credential, and must never be copied to logs, browser responses, or the release artifact.
4. If the release contains a not-yet-applied Platform Core migration, pass its exact numbered filename to the release helper. The helper validates the file is inside the artifact, applies it with `ON_ERROR_STOP`, then starts the fixed-SHA Compose file with `--remove-orphans`:

   ```bash
   /opt/henukit-releases/<sha>/bin/deploy-henukit-artifact.sh \
     /opt/henukit-releases/<sha> \
     /opt/henukit/.env.henukit \
     000013_password_registration.up.sql
   ```

   Omit the third argument when the release has no database migration. The helper never runs `docker build`.

5. Verify the active release and public routes. The primary runtime keeps the `quizcraft` and `study` databases for Portal `/practice` and `/library`, but it must not keep the standalone `quizcraft-api`, `quizcraft-web`, `study-api`, or `study-worker` containers:

   ```bash
   docker compose --env-file /opt/henukit/.env.henukit \
     -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml ps
   curl --fail --silent --show-error https://superhuazai.me/
   curl --fail --silent --show-error https://superhuazai.me/practice
   curl --fail --silent --show-error https://superhuazai.me/library
   test "$(curl --location --max-redirs 3 -s -o /dev/null -w '%{http_code}' https://superhuazai.me/quiz/)" = 404
   test "$(curl --location --max-redirs 3 -s -o /dev/null -w '%{http_code}' https://superhuazai.me/study-api/healthz)" = 404
   ```

   Retirement probes follow at most three canonical redirects and require the
   final response to be `404`; an intermediate redirect is not itself a pass.

   Auth and mail acceptance requires a real `@henu.edu.cn` registration flow on the deployed SHA: request a fresh registration code (`purpose=register`), confirm that it arrives in the actual mailbox, complete registration, and confirm that the OAuth redirect creates an authenticated Portal session (`GET /api/v1/session` returns `200`). Do not reuse a previously failed code or record the code value in logs.

Do not delete PostgreSQL databases, Docker volumes, or the retained QuizCraft/Study data as part of this runtime retirement. `--remove-orphans` removes old service containers only; any container that remains causes the helper to fail closed.

## Controlled WSL Linux/amd64 artifact path (when Actions is unavailable)

This path is a replacement for artifact *construction*, not a bypass of the
release gates. Its only permissible source is the exact, current
`origin/main` commit. An unmerged pull request (including a reviewed candidate)
must first be merged under the repository's normal human governance; never
build or deploy its branch directly.

Use a clean WSL2 checkout on Linux `x86_64` with an `linux/amd64` Docker daemon.
The WSL host has two separate roles: a dedicated builder account holds the
private signing key but has no production SSH, database, or application secrets;
a deployment operator account owns the `henu-prod` SSH alias but cannot read the
signing key. From the builder's clean detached checkout, fetch and select the
target before building:

```bash
git fetch --prune origin main
git switch --detach origin/main
release_sha="$(git rev-parse HEAD)"
scripts/ops/build-henukit-release-local.sh \
  --sha "$release_sha" \
  --output-dir /srv/henukit-artifacts \
  --signing-key /etc/henukit-release-builder/ed25519 \
  --handoff-group henukit-release-deployers
```

The builder fails closed unless the worktree is clean, `HEAD` equals the given
SHA, the remote `origin/main` head still equals that SHA before and after the
build, Docker reports `linux/amd64`, every image matches the trusted inventory,
the signing key is a private non-group/world-writable file, and the output
directory is non-symlinked and non-group/world-writable. The builder must be a
member of the dedicated handoff group; after verification it sets the output
root to `0750`, completed bundle directories to `0550`, and bundle files to
`0440`, all group-owned by `henukit-release-deployers`. The deployment identity
can therefore read but not modify the bundle, while the signing key remains
owner-only outside that tree. It emits one
flat `henukit-release-<sha>` directory containing all thirteen image archives,
the runtime archive, checksums, `RELEASE_SHA`, and a signed manifest.

### Direct WSL2-to-production transport

The controlled path is clean `origin/main` → dedicated WSL2 builder → signed
flat bundle → separate WSL2 deployment operator → `henu-prod`. Synchronize the
complete, clean source worktree into a new WSL directory; never reuse a mixed
developer checkout or make production a source mirror. Both WSL identities must
fetch `origin/main`, detach the same exact SHA, and pass clean-tree checks. The
builder hands the immutable bundle to the deployment operator through a
read-only local artifact location; the private signing key is not handed over.

The stable transport entry is
`scripts/ops/deploy-henukit-release-from-wsl.sh`. It runs only on WSL2
Linux/x86_64, resolves the fixed `henu-prod` SSH alias with strict host-key
checking (`BatchMode=yes`, `StrictHostKeyChecking=yes`), locally verifies the
signed bundle, rejects configured `ProxyJump`/`ProxyCommand` relays, explicitly
disables both proxy mechanisms for transfer, and rechecks the exact current
`origin/main` SHA. First run its read-only preflight from the deployment
operator's clean checkout:

```bash
release_sha="$(git rev-parse HEAD)"
scripts/ops/deploy-henukit-release-from-wsl.sh \
  --sha "$release_sha" \
  --artifact-dir "/srv/henukit-artifacts/henukit-release-$release_sha" \
  --allowed-signers /etc/henukit-release-deployer/release-signers \
  --remote-env-file /opt/henukit/.env.henukit \
  --account-operator-role operations-operator \
  --preflight
```

Before approving execution, compare `ssh -G henu-prod` with the approved
production connection record without copying its host or identity details into
release logs. A result that leaves `hostname henu-prod` unchanged is not a
configured alias. Retain the approved `known_hosts` fingerprint and the
out-of-band trust roots described below.

After the target SHA, migration list, backup reviewer, rollback owner, and
maintenance window are approved, replace `--preflight` with `--execute` (and
add `--platform-migrations <reviewed-comma-separated-list>` when required).
The script transfers the bundle directly from WSL2 to production with `rsync`;
the local workstation is not a transfer hop. Production verifies the signature
again with its root-owned verifier and allowed-signers file, atomically moves
the bundle into its final incoming directory, and calls the existing
`activate-henukit-release` entry. That entry retains the single-use exact-SHA
approval, backup/restore validation, smoke checks, and rollback behavior.
Immediately before each root helper execution, the transport rechecks that the
helper, inventory, allowed-signers file, and every ancestor are root-owned,
non-symlinked, and not group/world-writable. During activation it quiesces an
active `henukit-actions-watch.service` through a root-owned request. The watcher
binds the request to its current systemd `MainPID`, acknowledges it, and exits
only before or after a complete poll/deploy cycle. A restarted process cannot
acknowledge an older instance's request. A separate root-owned transport
`flock` serializes direct deployments, while a per-execution random nonce binds
each request and acknowledgement to that handoff. The transport never sends
`systemctl stop` into an active release. It waits for both the acknowledgement
and released service, and restores the service from an `EXIT` trap on both
success and failure. If a prior
service restart fails, the transport itself returns failure rather than claiming
the release sequence completed. If a prior
attempt already placed the same final bundle, the next run verifies it again
with production trust roots and resumes activation without retransferring or
deleting it; an invalid residual bundle fails closed for administrator review.

### Out-of-band local trust-root bootstrap

Before the first local-artifact activation, a production administrator must
install the corresponding public key in the root-owned, non-group/world-writable
allowed-signers file at `/etc/henukit/release-signers`. The long-lived watcher,
activation entry, verifier, image inventory, and allowed-signers file are local
trust roots. Never copy any of those control-plane files from a candidate
bundle, including its runtime archive.

This is an exceptional, out-of-band root-admin action performed **before** a
local candidate is transferred. Obtain a separate clean checkout of the
reviewed current `origin/main` SHA and an independently reviewed SHA-256 record
for exactly these four source basenames: `watch-henukit-actions.sh`,
`activate-henukit-release.sh`, `henukit-release-images.sh`, and
`verify-henukit-local-release.sh`. The hash record is created and approved from
the reviewed commit on a separate trusted admin station; do not generate it
from the checkout being installed or from the candidate artifact.

Copy that approved record to `/etc/henukit/release-trust-root-<sha>.sha256`
with owner `root:root` and mode `0400`, then use a root-owned staging directory
to close the source-copy race before installing the four long-lived helpers:

```bash
tooling=/srv/henukit-release-tooling
release_sha=<reviewed-current-main-sha>
test "$(git -C "$tooling" rev-parse HEAD)" = "$release_sha"
test -z "$(git -C "$tooling" status --porcelain --untracked-files=all)"
test "$(git -C "$tooling" ls-remote --exit-code origin refs/heads/main | awk 'NR == 1 { print $1 }')" = "$release_sha"

stage="/opt/henukit-trust-root/$release_sha"
sudo install -d -o root -g root -m 0700 "$stage"
sudo install -o root -g root -m 0555 "$tooling/scripts/ops/watch-henukit-actions.sh" "$stage/watch-henukit-actions.sh"
sudo install -o root -g root -m 0555 "$tooling/scripts/ops/activate-henukit-release.sh" "$stage/activate-henukit-release.sh"
sudo install -o root -g root -m 0555 "$tooling/scripts/ops/henukit-release-images.sh" "$stage/henukit-release-images.sh"
sudo install -o root -g root -m 0555 "$tooling/scripts/ops/verify-henukit-local-release.sh" "$stage/verify-henukit-local-release.sh"
sudo sh -c "cd '$stage' && sha256sum -c /etc/henukit/release-trust-root-$release_sha.sha256"
sudo install -o root -g root -m 0555 "$stage/watch-henukit-actions.sh" /usr/local/sbin/watch-henukit-actions
sudo install -o root -g root -m 0555 "$stage/activate-henukit-release.sh" /usr/local/sbin/activate-henukit-release
sudo install -o root -g root -m 0555 "$stage/henukit-release-images.sh" /usr/local/sbin/henukit-release-images.sh
sudo install -o root -g root -m 0555 "$stage/verify-henukit-local-release.sh" /usr/local/sbin/verify-henukit-local-release.sh
```

The local watcher rejects non-root-owned trust files or writable parent
directories. This bootstrap reads reviewed scripts only; it does not build,
upload, extract, or deploy a candidate. Transfer the completed flat directory
only after this trust-root record is retained, to a root-owned incoming path
such as `/opt/henukit-incoming/henukit-release-<sha>` without extracting or
modifying it. Record the transfer digest and make the target SHA, maintenance
window, backup reviewer, rollback owner, and signing-key authorization explicit
before the approval command.

The existing activation entry then prepares and restores backups before it
creates its single-use approval. It rechecks the current GitHub `main` head,
verifies the signed bundle with the trusted `allowed-signers` file, applies the
same Account/EPay gates, and uses the existing smoke checks and rollback:

```bash
sudo GH_TOKEN_FILE=/etc/henukit/github-actions-read.token \
  HENUKIT_ENV_FILE=/opt/henukit/.env.henukit \
  HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=operations-operator \
  HENUKIT_PLATFORM_MIGRATIONS=<reviewed-exact-list> \
  /usr/local/sbin/activate-henukit-release <full-main-sha> \
    --local-artifacts /opt/henukit-incoming/henukit-release-<full-main-sha> \
    --execute
```

Do not run this command until the target SHA is confirmed as the current
`origin/main` equivalent, the signature trust root is installed, the prepared
backup/restore evidence is reviewed, and an operator is available to own the
rollback. The production host still needs its read-only GitHub token to verify
that exact branch head; local artifacts never relax the main-only check.

## Server-side GitHub Actions watcher

The production host can poll GitHub Actions and download successful `main`
artifacts directly. The watcher never checks out the repository and never runs
`docker build`, package managers, or language compilers.

It deploys only the newest completed, successful `push` run of
`deploy-henukit.yml` on `main`. Before activation it:

1. downloads the exact full-SHA artifact set with `gh`;
2. rejects missing, duplicate, unexpected, or checksum-invalid files;
3. verifies the runtime `RELEASE_SHA` and its exact-SHA Account production
   boundary manifest. The manifest follows Account's local import graph,
   rejects user-reachable Portal/Gateway fixtures and fake-success sources,
   proves runtime wiring is EasyPay-or-disabled, and proves Portal requires the
   real Gateway with Portal API defaulting live;
4. creates custom-format `platform` and `account_portfolio` database backups,
   records their checksums, sizes and PostgreSQL version, and restores each
   into isolated temporary databases with key-table and Account durable-fact
   count checks. On the first Account Portfolio release it records and
   restores an explicit empty-database baseline before schema creation;
5. loads all thirteen fixed-SHA Docker images;
6. calls the existing `deploy-henukit-artifact.sh`, then invokes Platform
   Core's owner-defined command to grant all eight Account Console permissions,
   bump the role revision, and append an immutable grant audit;
7. verifies all thirteen running image tags, Account Portfolio health, and the public health routes, rolling
   back to the previously active fixed-SHA release if activation or verification
   fails.

### One-time bootstrap

Create a fine-grained GitHub token scoped only to
`jry21223/HENU-Kit-DEV`, with repository permissions `Actions: Read` and
`Contents: Read`. Do not paste the token into chat or place it in shell history.
On the server, enter it interactively:

```bash
sudo install -d -o root -g root -m 0700 /etc/henukit
sudo bash -c 'umask 077; read -r -s token; printf "%s\n" "$token" > /etc/henukit/github-actions-read.token'
sudo chown root:root /etc/henukit/github-actions-read.token
sudo chmod 0600 /etc/henukit/github-actions-read.token
```

When a verified GitHub Actions runtime artifact exists, install the watcher and
its unit from that artifact:

```bash
sudo install -d -o root -g root -m 0700 \
  /opt/henukit-staging /opt/henukit-releases /opt/henukit-backups \
  /var/lib/henukit-actions-watch
sudo install -o root -g root -m 0555 \
  /opt/henukit-releases/<sha>/bin/watch-henukit-actions.sh \
  /usr/local/sbin/watch-henukit-actions
sudo install -o root -g root -m 0555 \
  /opt/henukit-releases/<sha>/bin/activate-henukit-release.sh \
  /usr/local/sbin/activate-henukit-release
sudo install -o root -g root -m 0555 \
  /opt/henukit-releases/<sha>/bin/henukit-release-images.sh \
  /usr/local/sbin/henukit-release-images.sh
sudo install -o root -g root -m 0555 \
  /opt/henukit-releases/<sha>/bin/verify-henukit-local-release.sh \
  /usr/local/sbin/verify-henukit-local-release.sh
sudo install -o root -g root -m 0644 \
  /opt/henukit-releases/<sha>/infra/systemd/henukit-actions-watch.service \
  /etc/systemd/system/henukit-actions-watch.service
```

This Actions-artifact bootstrap is not valid for the first local-artifact
release. Use the out-of-band local trust-root bootstrap above instead.

Point the watcher at the existing production environment file. This file stores
only watcher configuration; application secrets stay in the referenced file:

```bash
sudo sh -c 'umask 077; cat > /etc/henukit/actions-watch.env' <<'EOF'
HENUKIT_ENV_FILE=/opt/henukit/.env.henukit
HENUKIT_PUBLIC_BASE_URL=https://superhuazai.me
HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=operations-operator
# Override only when Docker Compose uses a non-default container name.
HENUKIT_ACCOUNT_PORTFOLIO_CONTAINER=henukit-account-portfolio-1
EOF
```

The referenced application environment must explicitly contain
`PORTAL_API_MODE=live` and `NEXT_PUBLIC_PORTAL_ALLOW_MOCK=0`. The one-command
entry reads the already-installed HENU tenant PID/key from MetaView over the
existing root SSH channel without logging either value, atomically installs the
Account-side tenant configuration and `ENABLED=1`, and restores the complete
environment file on any failed release. Exact base, callback, and return URLs
are also installed. Invoking the watcher directly requires all six values
already be valid. Any invalid value is rejected before artifact download or
backup. The
operator role has no default: confirm the intended production role and set it
explicitly. Migrations declare permissions but deliberately do not grant them.

### First 8-to-9 image Account Portfolio cutover

The old eight-image watcher cannot accept the ninth Account Portfolio artifact.
Before preparing the first Account Portfolio release, install the updated
watcher from a verified candidate runtime as shown above, but do not create its
approval file yet. The updated watcher accepts an eight-image rollback baseline
only when that baseline's already-extracted
`docker-compose.henukit.release.yml` explicitly lacks `account-portfolio`; it
still requires all nine images and a healthy Account Portfolio container for
the candidate. This prevents a partially broken new release from being treated
as a legacy baseline. Current releases carry thirteen images: the nine above plus
`notice`, `notice-worker`, `food`, and `library` (see
`notice-food-production-onboarding.md`).

The initial `--once` run creates and restore-tests an empty
`account_portfolio` database if the old PostgreSQL volume does not contain one.
That database is retained across a rollback to the eight-image release; never
drop it as part of a rollback. Record both backup files and the metadata before
creating the exact-SHA approval.

Run one foreground check first, then enable continuous polling:

```bash
sudo GH_TOKEN_FILE=/etc/henukit/github-actions-read.token \
  HENUKIT_ENV_FILE=/opt/henukit/.env.henukit \
  /usr/local/sbin/watch-henukit-actions --once
sudo cat /var/lib/henukit-actions-watch/prepared/<full-main-sha>
sudo cat /opt/henukit-backups/platform-<timestamp>-<sha>-<pid>.dump.meta
sudo awk -F= '$1 == "account_portfolio_backup" { print $2 }' \
  /opt/henukit-backups/platform-<timestamp>-<sha>-<pid>.dump.meta
```

Before creating the approval file, record all production gates for that same
full SHA:

- Standards and Spec reviews have zero findings and all required PR/main CI is
  green;
- the same immutable artifacts have passed Staging Readiness, contract, smoke,
  browser/E2E, and rollback verification;
- the prepared backup metadata and isolated-restore counts have been inspected;
- the release scope, migration boundary, monitoring window, and rollback owner
  are explicitly approved.

### One-command Account payment release

After issue #166 is closed and the fixed-SHA artifact has passed the gates
above, the runtime's release entry is the sole production approval command:

```bash
sudo GH_TOKEN_FILE=/etc/henukit/github-actions-read.token \
  HENUKIT_ENV_FILE=/opt/henukit/.env.henukit \
  HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=operations-operator \
  HENUKIT_PLATFORM_MIGRATIONS=000017_account_portfolio_order_access.up.sql,000018_account_operator_role_grant_audit.up.sql \
  /usr/local/sbin/activate-henukit-release <full-main-sha> --execute
```

This single entry performs the unapproved preparation pass first. It refuses
while #166 is open, restore-tests both database backups, verifies the mock-free
manifest, securely copies the existing MetaView HENU tenant identity into the
Account environment, transfers the exact three EasyPay patches to `root@metaview.top`,
tests and atomically activates the gateway with health rollback, creates the
single-use SHA approval, refreshes both backups, applies Platform Core
`000017` and `000018`, deploys all thirteen fixed-SHA images, grants the eight
Account Console permissions through Platform Core, and probes the public
Account summary and EasyPay callback routes in addition to deterministic health
checks. Account Portfolio migrations
`000006` and `000007` remain service-owned startup migrations.

If activation fails, the watcher invokes the previous fixed-SHA helper with the
pre-release environment snapshot before the outer command restores that file,
so running containers and disk state agree. A crash after the new containers
become healthy but before the grant is recorded converges on the next run: the
active SHA path re-invokes Platform Core's idempotent audited grant before it
writes `last-activated-sha`. Migration `000018` is itself safe to reapply after
a later release step fails.

The defaults assume SSH key access to `root@metaview.top` and gateway directory
`/root/epay-gateway`; override only with
`HENUKIT_EPAY_GATEWAY_SSH_TARGET` and `HENUKIT_EPAY_GATEWAY_DIR`. SSH host-key
verification remains enabled. A successful gateway patch is backward-compatible
with MetaView and is intentionally retained if the later HENU activation rolls
back; reruns are idempotent through the exact patch-hash marker.

The manual approval procedure below remains available for recovery and
diagnosis, but is not the normal Account payment release. Only then authorize
production activation manually:

```bash
printf '%s\n' '<full-main-sha>' | \
  sudo tee /var/lib/henukit-actions-watch/approvals/<full-main-sha> >/dev/null
sudo chmod 0600 /var/lib/henukit-actions-watch/approvals/<full-main-sha>
sudo systemctl daemon-reload
sudo systemctl enable --now henukit-actions-watch.service
sudo systemctl status henukit-actions-watch.service
sudo journalctl -u henukit-actions-watch.service -f
```

The first check prepares artifacts and recovery evidence but does not activate
anything until the root-owned approval file contains that exact 40-character
SHA. This binds the production decision to the reviewed release instead of
turning every green build into an unattended deployment. When approval is
observed, the watcher creates and restore-tests a fresh second backup immediately
before activation, so writes made during the approval window are included.
The approval is moved into a root-only consumed audit directory before any image
is loaded. Any load, activation, verification, or rollback event therefore
requires a new explicit approval before the failed SHA can be attempted again.

The process polls every 60 seconds by default and uses a kernel `flock` to
prevent overlapping deployments; the lock is released even after a crash or
power loss. A release already active on all thirteen image tags is an idempotent
health-checked no-op. During the one-time 8-to-9 transition, the explicitly
legacy eight-image baseline remains a valid rollback target. A failed check exits the process, and Systemd retries it
after 30 seconds. Activation or public verification failure invokes the
previous fixed-SHA release helper and verifies the rollback before exiting.
The activation record is deliberately not a production-acceptance claim: the
real school-mail registration, OAuth session, metrics, and observation checks in
the release procedure above must still be recorded for that SHA.

Automatic schema selection is intentionally disabled. Set the reviewed,
comma-separated Platform Core migration filenames with
`HENUKIT_PLATFORM_MIGRATIONS` for that deployment and remove the setting
afterward. The singular `HENUKIT_PLATFORM_MIGRATION` remains a compatibility
alias for older one-migration releases.
