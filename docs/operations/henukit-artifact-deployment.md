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

3. Extract the runtime tarball into a new release directory. Keep the existing `.env.henukit` outside that directory and make a read-only backup of the `platform` database before applying a migration.
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
   test "$(curl -s -o /dev/null -w '%{http_code}' https://superhuazai.me/quiz/)" = 404
   test "$(curl -s -o /dev/null -w '%{http_code}' https://superhuazai.me/study-api/healthz)" = 404
   ```

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
3. verifies the runtime `RELEASE_SHA`;
4. creates a custom-format `platform` database backup, records its checksum,
   size and PostgreSQL version, and restores it into an isolated temporary
   database with key-table count checks;
5. loads all eight fixed-SHA Docker images;
6. calls the existing `deploy-henukit-artifact.sh`;
7. verifies all eight running image tags and the public health routes, rolling
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
EOF
```

Run one foreground check first, then enable continuous polling:

```bash
sudo GH_TOKEN_FILE=/etc/henukit/github-actions-read.token \
  HENUKIT_ENV_FILE=/opt/henukit/.env.henukit \
  /usr/local/sbin/watch-henukit-actions --once
sudo cat /var/lib/henukit-actions-watch/prepared/<full-main-sha>
sudo cat /opt/henukit-backups/platform-<timestamp>-<sha>-<pid>.dump.meta
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

Only then authorize production activation:

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
power loss. A release already active on all eight image tags is an idempotent
health-checked no-op. A failed check exits the process, and Systemd retries it
after 30 seconds. Activation or public verification failure invokes the
previous fixed-SHA release helper and verifies the rollback before exiting.
The activation record is deliberately not a production-acceptance claim: the
real school-mail registration, OAuth session, metrics, and observation checks in
the release procedure above must still be recorded for that SHA.

Automatic schema selection is intentionally disabled. When a reviewed release
requires one Platform Core migration, set its exact artifact filename with
`HENUKIT_PLATFORM_MIGRATION` for that deployment and remove it afterward.
