# QuizCraft production cutover

This runbook is the HC-22 release boundary. A GitHub Actions run or a started deployment is not success; success requires the build-bound SHA from the authenticated cutover-evidence endpoint, database reconciliation, live HTTP checks, and a verified rollback artifact.

## Gates and order

1. Deploy the legacy event-capture schema while FastAPI still owns all traffic. Keep `QUIZCRAFT_READ_ONLY=0`.
2. Create a physically separate empty target database, apply all Go migrations, run `reconcile -mode full`, then catch up until `ready=true`.
3. Save custom-format dumps of both databases plus SHA-256 sidecars. Restore each dump into a separate verification database and run the reconciliation/readiness queries there. Save the current systemd unit, Nginx config, Go binary SHA, legacy commit, and static-bundle tarball in the same release directory.
4. Start the Go service with `QUIZCRAFT_WRITES_ENABLED=0`. Route only `/healthz`, `/readyz`, and `/api/v1/**` to it. `/api/**` remains FastAPI.
5. Build the browser with `VITE_QUIZCRAFT_GO_READ_PERCENT=5` and `VITE_QUIZCRAFT_GO_WRITES=0`. Verify the stable cohort receives Go bank reads while every practice/feedback mutation still uses FastAPI. Observe 5xx, latency, bank count, and content hashes before increasing the read percentage.
6. For write cutover, announce a stop-write window. Set legacy `QUIZCRAFT_READ_ONLY=1`, restart it, prove an unsafe request returns `503`, run final catch-up, and take the final verified dumps. Do not reopen writes if the cursor, exceptions, hashes, or shadow gate blocks.
7. Enable Go writes server-side first, run the direct synthetic session/answer smoke, then deploy the browser with `VITE_QUIZCRAFT_GO_WRITES=1`. Run desktop and 390px practice, feedback, favorites, ranking, and workshop checks against the public origin.
8. Keep FastAPI running with `QUIZCRAFT_READ_ONLY=1` for at least one full release cycle. Record `observation_started_at`, `retain_until`, the legacy release SHA, and its snapshot hash. Do not archive the service or branch before `retain_until` and a final zero-write audit.

## Executable operator sequence

Run on the production host with DSNs loaded from root-readable environment files; never place them in argv or logs. Replace the example database/role names only after reading them from the DSN without printing its password.

```bash
set -euo pipefail
release_sha='<40-char merged commit>'
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]]
install -d -m 700 "/var/backups/quizcraft/$release_sha"
# Define quizcraft-legacy and quizcraft-go in root-owned mode-0600
# /root/.pg_service.conf; keep passwords in a mode-0600 PGPASSFILE.
pg_dump --dbname='service=quizcraft-legacy' --format=custom --file="/var/backups/quizcraft/$release_sha/legacy.dump"
pg_dump --dbname='service=quizcraft-go' --format=custom --file="/var/backups/quizcraft/$release_sha/go.dump"
sha256sum /var/backups/quizcraft/$release_sha/*.dump > "/var/backups/quizcraft/$release_sha/SHA256SUMS"
pg_restore --list "/var/backups/quizcraft/$release_sha/legacy.dump" >/dev/null
pg_restore --list "/var/backups/quizcraft/$release_sha/go.dump" >/dev/null
quizcraft_restore_db="quizcraft_restore_$release_sha"
legacy_restore_db="qc_legacy_restore_$release_sha"
[[ "$quizcraft_restore_db" =~ ^quizcraft_restore_[0-9a-f]{40}$ ]]
[[ "$legacy_restore_db" =~ ^qc_legacy_restore_[0-9a-f]{40}$ ]]
(( ${#quizcraft_restore_db} <= 63 && ${#legacy_restore_db} <= 63 ))
sudo -u postgres createdb --owner="$QUIZCRAFT_DB_ROLE" "$quizcraft_restore_db"
sudo -u postgres createdb --owner="$LEGACY_DB_ROLE" "$legacy_restore_db"
sudo -u postgres pg_restore --dbname="$quizcraft_restore_db" --no-owner "/var/backups/quizcraft/$release_sha/go.dump"
sudo -u postgres pg_restore --dbname="$legacy_restore_db" --no-owner "/var/backups/quizcraft/$release_sha/legacy.dump"
sudo -u postgres psql --dbname="$quizcraft_restore_db" -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM quizcraft_banks"
sudo -u postgres psql --dbname="$legacy_restore_db" -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM question_banks"
sudo -u postgres dropdb "$quizcraft_restore_db"
sudo -u postgres dropdb "$legacy_restore_db"
```

The restore database variable in the preceding block must be assigned and validated as `quizcraft_restore_<40 hex>` before `createdb`, `pg_restore`, or `dropdb`; never derive a destructive target from an unset variable. Also archive the live unit/Nginx files and static tree:

```bash
cp --preserve=mode,timestamps /etc/systemd/system/quizcraft-cn.service "/var/backups/quizcraft/$release_sha/"
cp --preserve=mode,timestamps /etc/nginx/sites-enabled/superhuazai.me "/var/backups/quizcraft/$release_sha/"
tar --create --file="/var/backups/quizcraft/$release_sha/static.tar" --directory=/var/www quizcraft-cn
sha256sum "/var/backups/quizcraft/$release_sha/static.tar" >> "/var/backups/quizcraft/$release_sha/SHA256SUMS"
```

Build-bound Go artifacts must inject the merged commit rather than read a SHA from service environment:

```bash
repo_root=/opt/HENU-Kit-DEV
[[ -d "$repo_root/products/quizcraft/go-service" ]]
cd "$repo_root/products/quizcraft/go-service"
go_release_dir="/opt/quizcraft-go/releases/$release_sha"
read_release_id="${release_sha}-read5"
read_static_dir="/var/www/quizcraft-releases/$read_release_id"
[[ ! -e "$go_release_dir" && ! -e "$read_static_dir" ]]
install -d -m 755 "$go_release_dir" "$read_static_dir"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-X main.buildReleaseSHA=$release_sha" \
  -o "$go_release_dir/quizcraft-server" ./cmd/server
VITE_QUIZCRAFT_GO_SHADOW=0 VITE_QUIZCRAFT_GO_READ_PERCENT=5 VITE_QUIZCRAFT_GO_WRITES=0 \
  pnpm --dir "$repo_root/products/quizcraft/web-app" run build:ops
cp -a "$repo_root/products/quizcraft/web-app/dist/." "$read_static_dir/"
# One-time baseline before the first switch:
install -d -m 755 /var/www/quizcraft-releases/legacy-baseline
cp -a /var/www/quizcraft-cn/. /var/www/quizcraft-releases/legacy-baseline/
ln -s /var/www/quizcraft-releases/legacy-baseline /var/www/quizcraft-current
nginx -t
CONFIRM_CUTOVER_SWITCH=yes \
  "$repo_root/products/quizcraft/go-service/scripts/switch-cutover-release.sh" \
  activate "$release_sha" "$read_release_id"
```

Nginx must use `root /var/www/quizcraft-current;` and include the three Go locations from `deploy/nginx-cutover.conf.example`. Before read-only mode, install the event-capture legacy code and verify its health. At the write window, first set the legacy database role itself read-only, then set the application flag:

```bash
sudo -u postgres psql --dbname=postgres -v ON_ERROR_STOP=1 -v legacy_role="$LEGACY_DB_ROLE" -c 'ALTER ROLE :"legacy_role" SET default_transaction_read_only=on'
# Set QUIZCRAFT_READ_ONLY=1 in /etc/quizcraft-cn.env without printing the file.
systemctl restart quizcraft-cn.service
systemctl is-active --quiet quizcraft-cn.service
curl --fail --silent http://127.0.0.1:10086/api/healthz
```

Run final `reconcile -mode catch-up`, record its exact `run_id` and `source_head`, and evaluate a new shadow window whose end is not older than that migration run. Only then build an immutable all-Go browser artifact in a different release directory, set `QUIZCRAFT_WRITES_ENABLED=1`, restart Go, execute `verify-cutover.sh`, and atomically activate the write bundle:

```bash
write_release_id="${release_sha}-writes"
write_static_dir="/var/www/quizcraft-releases/$write_release_id"
[[ ! -e "$write_static_dir" ]]
install -d -m 755 "$write_static_dir"
VITE_QUIZCRAFT_GO_SHADOW=0 VITE_QUIZCRAFT_GO_READ_PERCENT=100 VITE_QUIZCRAFT_GO_WRITES=1 \
  pnpm --dir "$repo_root/products/quizcraft/web-app" run build:ops
cp -a "$repo_root/products/quizcraft/web-app/dist/." "$write_static_dir/"
# Export every variable from the Live evidence command below, including a
# dedicated cutover operator session. The switch owns the 0->1 write-gate edit,
# restarts Go, runs verify-cutover.sh, and exposes the write bundle only if all pass.
CONFIRM_CUTOVER_SWITCH=yes \
  "$repo_root/products/quizcraft/go-service/scripts/switch-cutover-release.sh" \
  activate-writes "$release_sha" "$write_release_id"
```

Before enabling the Go write flag, the switch atomically persists `activating-writes:<pending-static-target>`. All Nginx checks and reloads happen before the final static swap. If the process is interrupted, the next invocation compares that target with the live symlink: an unexposed transaction is restored to write flag `0`, while an already exposed write bundle is finalized as `writes` and can never fall back to read5 or legacy.

## Rollback boundary

Before public writes reopen, rollback is: restore the previous static bundle and Nginx config, set legacy read-only to `0`, restart FastAPI, and keep Go writes disabled. Verify the public legacy practice flow before ending the stop-write window.

```bash
# Keep QUIZCRAFT_WRITES_ENABLED=0. Restore QUIZCRAFT_READ_ONLY=0 in the legacy env.
sudo -u postgres psql --dbname=postgres -v ON_ERROR_STOP=1 -v legacy_role="$LEGACY_DB_ROLE" -c 'ALTER ROLE :"legacy_role" SET default_transaction_read_only=off'
CONFIRM_CUTOVER_SWITCH=yes \
NGINX_ROLLBACK_CONFIG="/var/backups/quizcraft/$release_sha/superhuazai.me" \
"$repo_root/products/quizcraft/go-service/scripts/switch-cutover-release.sh" rollback
systemctl restart quizcraft-cn.service
systemctl is-active --quiet quizcraft-cn.service
nginx -t && systemctl reload nginx
```

After public Go writes reopen, do not point traffic at the stale legacy database. `switch-cutover-release.sh rollback` intentionally refuses while the active phase is `writes`. Pause traffic in an explicit maintenance window, retain the compatible expand migrations, and activate a separately verified write-capable Go/static release against the Go database. A data restore is allowed only from the verified final dump with an explicitly accepted recovery-point loss.

## Live evidence command

```bash
GO_BASE_URL=https://quiz.example.com \
LEGACY_BASE_URL=http://127.0.0.1:10086 \
EXPECTED_RELEASE_SHA='<full deployed SHA>' \
EXPECTED_WRITES_ENABLED=true \
EXPECTED_LEGACY_READ_ONLY=true \
EXPECTED_MIGRATION_RUN_ID='<run UUID>' \
EXPECTED_SOURCE_HEAD='<final source head>' \
EXPECTED_SHADOW_GATE_REPORT_ID='<gate UUID>' \
LEGACY_SERVER_PATH=/opt/quizcraft-cn/server.py \
EXPECTED_LEGACY_SHA256='<sha256 of reviewed server.py>' \
"$repo_root/products/quizcraft/go-service/scripts/verify-cutover.sh"
```

`CUTOVER_EVIDENCE_SECRET` and `QUIZCRAFT_OPERATOR_SESSION` are loaded from a separate root-only cutover environment before this command; do not type either value inline or persist the operator session in the service environment. The operator session must belong to a dedicated disposable cutover account with Workshop read permission. The script places both credentials in mode-0600 temporary curl configs so neither is exposed in process argv. The live verification creates one idempotent, clearly labelled feedback record, adds and removes one favorite, reads ranking and Workshop catalog data, and executes a synthetic practice session and answer.

Store the redacted output, reconciliation run ID, shadow report ID, database dump hashes, static artifact hash, Nginx test result, service status, public response codes, and observation dates in the HC-22 issue or release evidence file. Never store DSNs, cookies, HMAC secrets, or admin tokens.
