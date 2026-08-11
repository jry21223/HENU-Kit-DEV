# HENU Kit deploy webhook

`deploy-webhook` is the server-side GitHub push receiver for the HENU Kit
Monorepo. It replaces the unsafe operational pattern of updating a live working
tree with `git pull && restart`.

## Security boundary

- Listens on loopback only; HTTPS terminates at Nginx.
- Requires `Content-Type: application/json` and validates the raw request body
  with `X-Hub-Signature-256` HMAC-SHA256.
- Requires a valid `X-GitHub-Delivery`, accepts `ping`, and queues only `push`
  events for the configured repository and `refs/heads/main`.
- Stores only minimal repository/ref/SHA metadata. It never stores the webhook
  payload, secret, Deploy Key, database credentials, or GitHub token.
- Responds before deployment. A persistent file queue survives service and host
  restarts; successful Delivery IDs are deduplicated.
- The HTTP process never invokes deployment commands. A separate Systemd path
  unit starts the oneshot runner.
- The runner calls one fixed absolute executable with separately validated
  arguments. Request data is never evaluated as shell source.
- The deploy driver fetches a read-only checkout, verifies that the event SHA is
  exactly the current `origin/main`, creates an immutable release worktree, and
  executes root-owned `prepare`, `activate`, `verify`, and `rollback` hooks.
- First releases and high-risk path changes require an explicit root-owned SHA
  approval by default.

## Commands

```bash
henukit-deploy-webhook serve
henukit-deploy-webhook run
henukit-deploy-webhook retry <full-sha>
henukit-deploy-webhook materials-serve
henukit-deploy-webhook materials-run
```

The receiver exposes loopback endpoints:

- `POST /webhooks/github`
- `GET /healthz`
- `GET /readyz`
- `GET /statusz`

## One-time server installation

```bash
sudo services/deploy-webhook/deploy/install.sh
```

The installer builds the static Go binary, creates the unprivileged
`henukit-deploy` account, installs Systemd units, generates a 256-bit webhook
secret, creates a read-only Deploy Key, starts only the loopback receiver, and
prints the remaining manual steps. The queue watcher stays disabled until the
read-only clone, root-owned deployment hooks, HTTPS, and rollback path have been
reviewed. The installer intentionally does not register the Deploy Key, edit
DNS/Nginx, create the GitHub webhook, or enable a server-specific deployment
hook without operator review.

See [`../../docs/operations/github-webhook-deploy.md`](../../docs/operations/github-webhook-deploy.md)
for the complete bootstrap, GitHub setup, release, rollback, and handoff Runbook.

## Tests

```bash
cd services/deploy-webhook
go test -race ./...
go vet ./...
bash -n deploy/henukit-deploy deploy/install.sh
```

The test suite covers GitHub's published signature vector, invalid signatures,
repository/branch/SHA filtering, payload limits, persistent queue semantics,
successful Delivery dedupe, failed redelivery, restart recovery, argument-safe
command execution, stale push rejection, immutable worktrees, hook phases, and
manual high-risk approval, approved-SHA retry, and remote URL pinning.

## Legacy QuizCraft receiver

`products/quizcraft/scripts/github_deploy_webhook.py` is a product-specific
legacy deployment path. It is not the Monorepo release source of truth. Keep it
running only until the new receiver and QuizCraft hook have been installed and
verified on the server; remove it in a separate rollback-aware change.

## Materials candidate queue template

`materials-serve` and `materials-run` are a source-only B01 boundary for
[jry21223/HENU-Final-Review](https://github.com/jry21223/HENU-Final-Review).
They use a materials-only latest-arrival queue: one preparation may run while
only the most recently accepted delivery waits. The fixed consumer invokes the
unprivileged candidate-preparation command with its source, ref, and candidate
root bound by operator configuration.

This is not an installation or activation path. Do not use the legacy
`install.sh --enable-materials-sync` path, which installs the retired root sync
driver. B01 neither enables a host service nor changes an Nginx-served tree or
the Study catalog. See
[`../../docs/operations/henukit-materials-sync.md`](../../docs/operations/henukit-materials-sync.md).
