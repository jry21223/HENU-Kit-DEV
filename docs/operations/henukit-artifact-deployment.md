# HENU Kit fixed-artifact deployment

## Production deploy contract (binding)

HENU Kit production **must** follow this path and no other:

```
GitHub Actions build (fixed full SHA)
        ↓
Download image + runtime artifacts (+ SHA-256)
        ↓
Host: verify checksums → docker load → extract runtime
        ↓
deploy-henukit-artifact.sh (compose up, optional migration)
```

| Rule | Required behavior |
|------|-------------------|
| Compile location | **Only** GitHub Actions workflow `Build HENU Kit release artifacts` (`.github/workflows/deploy-henukit.yml`) |
| Host compile | **Forbidden** — no `docker compose build`, no `pnpm build` / `go build` on the production host for release units |
| Identity | Every image tag and runtime package is the **full 40-char git SHA** (`RELEASE_SHA`); never `latest` |
| Secrets | Live only in host `.env.henukit` (outside the artifact); artifacts never carry SMTP/DB/session secrets |
| Activate | `scripts/ops/deploy-henukit-artifact.sh` (also shipped inside the runtime tarball as `bin/deploy-henukit-artifact.sh`) |
| Legacy runtimes | Standalone Study / QuizCraft containers must not exist after activate (helper fail-closed) |

Supporting files:

| Piece | Path |
|-------|------|
| CI build + upload | `.github/workflows/deploy-henukit.yml` |
| Prebuilt compose (no `build:`) | `docker-compose.henukit.prebuilt.yml` |
| Download helper | `scripts/ops/download-henukit-artifacts.sh` |
| Activate helper | `scripts/ops/deploy-henukit-artifact.sh` |
| Contract tests | `scripts/ops/tests/deploy-henukit-workflow.test.mjs`, `deploy-henukit-artifact.test.mjs` |

**Out of contract for HENU Kit primary stack:** building on the server from a git worktree, ad-hoc `docker build`, or promoting untagged local images. Webhook auto-sync (`github-webhook-deploy.md`) is a separate code-sync/approve path; the **runtime** that serves users must still be a fixed-SHA artifact load, not an in-place compile.

---

## Release procedure

### 1. Build on GitHub

Merge (or push) to `main`, or open a PR / use `workflow_dispatch`. Wait for a **successful** run of **Build HENU Kit release artifacts** on the exact full SHA you intend to ship. Record that SHA.

Primary images (matrix):

- `henukit-console`
- `henukit-console-gateway`
- `henukit-platform-core`
- `henukit-platform-mail-worker`
- `henukit-platform-smtp-provider`
- `henukit-portal`
- `henukit-portal-api`
- `henukit-portal-gateway`

Plus runtime tarball `henukit-runtime-<sha>.tar.gz` (compose file, nginx example, platform-core migrations, deploy helper, `RELEASE_SHA`).

### 2. Download on an operator machine or the host

Prefer the helper (requires authenticated `gh`):

```bash
# from a checkout that contains scripts/ops (or copy the script alone)
./scripts/ops/download-henukit-artifacts.sh <full-sha> /var/tmp/henukit-artifacts/<full-sha>
# optional third arg: owner/repo if not the default gh repo context
```

The helper:

1. Finds a **completed success** workflow run for that `headSha`
2. `gh run download`s all artifacts
3. Flattens archives into the destination directory
4. Requires every image + runtime archive and matching `.sha256`
5. Runs `sha256sum -c` on each pair

Manual equivalent: GitHub UI → Actions → that run → download each artifact, then `sha256sum -c` locally.

### 3. Load images and extract runtime (on the host)

```bash
sha=<full-sha>
art=/var/tmp/henukit-artifacts/$sha
rel=/opt/henukit-releases/$sha

for img in console console-gateway platform-core platform-mail-worker \
           platform-smtp-provider portal portal-api portal-gateway; do
  sha256sum -c "$art/henukit-${img}-${sha}.docker.tar.gz.sha256"
  docker load < "$art/henukit-${img}-${sha}.docker.tar.gz"
done

sha256sum -c "$art/henukit-runtime-${sha}.tar.gz.sha256"
install -d "$rel"
tar -C "$rel" -xzf "$art/henukit-runtime-${sha}.tar.gz"
test "$(tr -d '[:space:]' < "$rel/RELEASE_SHA")" = "$sha"
```

Keep the existing `.env.henukit` **outside** the release directory. Make a read-only backup of the `platform` database before applying a migration.

### 4. Activate

If the release contains a not-yet-applied Platform Core migration, pass its exact numbered filename. The helper validates the file is inside the artifact, applies it with `ON_ERROR_STOP`, then starts the fixed-SHA Compose file with `--remove-orphans`:

```bash
/opt/henukit-releases/<sha>/bin/deploy-henukit-artifact.sh \
  /opt/henukit-releases/<sha> \
  /opt/henukit/.env.henukit \
  000013_password_registration.up.sql
```

Omit the third argument when the release has no database migration. The helper never runs `docker build`.

### 5. Verify the active release and public routes

The primary runtime keeps the `quizcraft` and `study` databases for Portal `/practice` and `/library`, but it must not keep the standalone `quizcraft-api`, `quizcraft-web`, `study-api`, or `study-worker` containers:

```bash
docker compose --env-file /opt/henukit/.env.henukit \
  -f /opt/henukit-releases/<sha>/docker-compose.henukit.release.yml ps
curl --fail --silent --show-error https://superhuazai.me/
curl --fail --silent --show-error https://superhuazai.me/practice
curl --fail --silent --show-error https://superhuazai.me/library
test "$(curl -s -o /dev/null -w '%{http_code}' https://superhuazai.me/quiz/)" = 404
test "$(curl -s -o /dev/null -w '%{http_code}' https://superhuazai.me/study-api/healthz)" = 404
```

Auth / mail smoke (after this contract is live): send a `@henu.edu.cn` login code, complete OAuth, confirm `GET /api/v1/session` is authenticated; confirm `platform-mail-worker` and `platform-smtp-provider` are up and SMTP env is filled.

Do not delete PostgreSQL databases, Docker volumes, or the retained QuizCraft/Study data as part of runtime retirement. `--remove-orphans` removes old service containers only; any legacy container that remains causes the helper to fail closed.
