# HENU Kit fixed-artifact deployment

HENU Kit production is deployed from the Docker images and runtime tarball built by GitHub Actions. The host does not compile the repository and does not read secrets from the release artifact.

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
5. loads all nine fixed-SHA Docker images;
6. calls the existing `deploy-henukit-artifact.sh`, then invokes Platform
   Core's owner-defined command to grant all eight Account Console permissions,
   bump the role revision, and append an immutable grant audit;
7. verifies all nine running image tags, Account Portfolio health, and the public health routes, rolling
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

Install the watcher and its unit from a verified runtime artifact:

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
sudo install -o root -g root -m 0644 \
  /opt/henukit-releases/<sha>/infra/systemd/henukit-actions-watch.service \
  /etc/systemd/system/henukit-actions-watch.service
```

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
as a legacy baseline.

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
  /usr/local/sbin/activate-henukit-release <full-main-sha> --execute
```

This single entry performs the unapproved preparation pass first. It refuses
while #166 is open, restore-tests both database backups, verifies the mock-free
manifest, securely copies the existing MetaView HENU tenant identity into the
Account environment, transfers the exact three EasyPay patches to `root@metaview.top`,
tests and atomically activates the gateway with health rollback, creates the
single-use SHA approval, refreshes both backups, applies Platform Core
`000014` through `000019`, deploys all nine fixed-SHA images, grants the eight
Account Console permissions through Platform Core, and probes the public
Account summary and EasyPay callback routes in addition to deterministic health
checks. Account Portfolio migrations
`000006` and `000007` remain service-owned startup migrations.
Migration `000019` declares the empty `operations-operator` role but grants it
to no user; the audited release command grants the role's Account permissions,
while assigning a human operator remains a separate explicit authorization.

If activation fails, the watcher invokes the previous fixed-SHA helper with the
pre-release environment snapshot before the outer command restores that file,
so running containers and disk state agree. A crash after the new containers
become healthy but before the grant is recorded converges on the next run: the
active SHA path re-invokes Platform Core's idempotent audited grant before it
writes `last-activated-sha`. Migrations `000014` through `000019` are safe to
reapply after a later release step fails.

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
power loss. A release already active on all nine image tags is an idempotent
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
