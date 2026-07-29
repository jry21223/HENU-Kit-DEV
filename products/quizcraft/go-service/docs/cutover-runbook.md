# QuizCraft maintenance-window full cutover

This is the HC-22 production boundary. QuizCraft is in a no-expected-user maintenance period after finals, but that fact is not proof of zero writes. The migration therefore uses an explicit technical write freeze and one atomic 100% read/write switch. Percentage cohorts, live shadow traffic, and gray observation are not release gates. Offline comparison reports remain useful diagnostic evidence.

## Hard prerequisites

Do not enter the write freeze until all of these are true:

1. Platform Core is independently deployed at `https://account.superhuazai.me`, with its own systemd services, PostgreSQL database/user, Redis access, loopback SMTP Provider, TLS vhost, and root-owned secrets.
2. A real `henu.edu.cn` mailbox has completed request, delivery, manual entry of the current emailed code, verification, 15-day Core Session, OAuth callback, and logout/revocation checks. The owner enters the code through the verifier's hidden interactive terminal prompt; environment variables, command arguments, test claims, and manual SQL are forbidden.
3. QuizCraft OAuth/HMAC credentials were generated on the host and provisioned with `services/platform-core/scripts/provision-quizcraft-client.sh`.
4. The first human operator logged in normally; root then used `grant-initial-operator` to grant only platform operations and QuizCraft Workshop permissions. The audit row exists.
5. The immutable Go release and the `VITE_QUIZCRAFT_GO_WRITES=1` browser bundle are staged but not public. The immutable web-app directory retains its locked Playwright dependency and installed Chromium for the release gate. Install the explicit maintenance page at `/var/www/quizcraft-maintenance/maintenance.html` before enabling the marker; it must not depend on either release symlink.
6. Legacy and Go backups have each been restored into temporary databases and queried successfully. Final backups are root-only, SHA-256 recorded, and retained for 30 days.

Any authentication, restore, reconciliation, hash, or migration-exception failure aborts. There is no human override.

## Technical write freeze

1. Publish the maintenance page. Every public mutation must return `503`; do not rely on expected low traffic.
2. Set legacy `QUIZCRAFT_READ_ONLY=1`, restart FastAPI, and verify an unsafe request is rejected.
3. Set the legacy PostgreSQL role to `default_transaction_read_only=on` and prove direct writes fail.
4. Keep `QUIZCRAFT_WRITES_ENABLED=0` in `/etc/quizcraft-go.env` and prove a Go practice-session mutation returns `503 writes_disabled`.
5. Run the final incremental catch-up.
6. Require cursor=head, zero unresolved exceptions, and 100% reconciliation of banks, questions, types, answers, chapters, feedback, legacy ranking snapshots, and content hashes.
7. Take the final dual-database dumps, record SHA-256 values, restore both into temporary databases, and query their key tables.
8. Run desktop and 390px synthetic checks for practice, feedback, favorites, rankings, and Workshop. Also verify health, readiness, account login, mail delivery, OAuth, and logs.

Record release/reconciliation and backup metadata in a root-owned, non-group/world-writable JSON file following `deploy/cutover-gate-evidence.example.json`. Boolean claims are not acceptance evidence. `verify-cutover.sh` binds metadata to the exact release SHA, migration run and source head; recomputes both backup SHA-256 values; uses its version-controlled restore verifier to create/query/drop fresh databases from those exact dumps; runs its version-controlled desktop/390px browser, interactive real-mail Platform Core login/OAuth/logout, and journal log-audit verifiers; and probes the live maintenance page plus public mutation paths. It reads the dedicated Portal-read credentials only from a root-owned, mode `0600` `QUIZCRAFT_GO_ENV_FILE`, writes a fresh short-lived signed curl configuration for each protected catalog or ranking probe, and never puts that credential in a curl command argument. The ranking probe is part of the pre-write gate, and all protected probes use the normal Portal-read contract. The verifier requests a fresh email, requires the mailbox owner to enter its current code without echo through `/dev/tty`, and submits that code to the real verification API before checking Session and OAuth behavior. It intentionally does not require mailbox credentials or a traffic shadow report.

## Atomic activation

Load `CUTOVER_EVIDENCE_SECRET`, the dedicated operator Session, source database URLs, isolated restore-admin URL, real test mailbox address, Console origin, log start time, migration run ID, source head, release SHA, and legacy source hash from root-owned environment files. `GO_BASE_URL` and `LEGACY_BASE_URL` must be loopback origins that bypass public maintenance; `PUBLIC_BASE_URL` is the public origin. Run the gate from an interactive terminal so the mailbox owner can enter the current emailed code at the hidden prompt. Do not place the code or other secrets in environment variables, shell history, or arguments.

```bash
VITE_QUIZCRAFT_GO_WRITES=1 pnpm --filter quiz-app build:ops

sudo --preserve-env=CONFIRM_CUTOVER_SWITCH,GO_BASE_URL,LEGACY_BASE_URL,PUBLIC_BASE_URL,CUTOVER_E2E_BASE_URL,CUTOVER_WEB_APP_DIR,PLATFORM_ACCOUNT_ORIGIN,CONSOLE_ORIGIN,EXPECTED_RELEASE_SHA,EXPECTED_LEGACY_READ_ONLY,CUTOVER_EVIDENCE_SECRET,CUTOVER_GATE_EVIDENCE_FILE,EXPECTED_MIGRATION_RUN_ID,EXPECTED_SOURCE_HEAD,LEGACY_SERVER_PATH,EXPECTED_LEGACY_SHA256,LEGACY_DATABASE_URL,CUTOVER_RESTORE_ADMIN_URL,CUTOVER_TEST_EMAIL,CUTOVER_LOG_SINCE,PLATFORM_CLIENT_SECRET,QUIZCRAFT_OPERATOR_SESSION,QUIZCRAFT_GO_ENV_FILE \
  /opt/quizcraft-go/releases/current/scripts/switch-cutover-release.sh \
  activate-writes "$EXPECTED_RELEASE_SHA" "$EXPECTED_RELEASE_SHA-writes"
```

The switch script first enables the Nginx maintenance marker, activates the immutable Go binary with writes still disabled, waits for health, and executes the complete preflight with `EXPECTED_WRITES_ENABLED=false`. Only after that succeeds may it set `QUIZCRAFT_WRITES_ENABLED=1`, restart Go, run post-activation synthetic mutations, atomically expose the all-Go browser bundle, and remove maintenance.

The first durable Go business mutation is the **Go write promise point**. Before it, restore the final snapshots and legacy routing if activation fails. Once the script enables Go writes it conservatively treats the promise as crossed: every later error keeps the maintenance marker and write-capable Go state for forward repair. After correcting the failed gate, rerun that exact release with `resume-writes <sha> <sha-writes>`; it re-verifies and completes only the write-capable release. Direct rollback to the stale legacy database is forbidden unless every Go write is reverse-synced and reconciled first.

## Seven-day cold reserve

After successful activation:

- Stop FastAPI and remove it from Nginx; do not send live observation traffic to it.
- Keep its database role read-only, and retain its release SHA, final source hash, and snapshot for seven days.
- Keep issue #44 open during this reserve.
- At day seven, require a human approval after a zero-write audit, successful Go-backup restore, and public key-flow checks. Then remove the cold service artifacts and close #44, followed by #45 and parent #22.
- Keep the final root-only backups and SHA-256 records for 30 days even after the cold service is removed.

Workflow start or green CI is never production success. Record exact live SHA, migration run/cursor, backup hashes and restore evidence, synthetic-flow results, Go write promise timestamp, cold-reserve deadline, and final human approval in the issue.
