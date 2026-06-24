# Deployment

This project is still in internal-test hardening. The production files in this repository are examples, not a one-command guarantee of a safe public launch. They are meant to make deployment review repeatable.

## Production Shape

- Nginx terminates HTTPS and proxies two public origins:
  - `review.example.com` -> Next.js Web and `/api/` to Go API
  - `admin.review.example.com` -> Vue Admin and `/api/` to Go API
- Go API and Worker run as private containers.
- PostgreSQL, Redis, and local uploads are private Docker volumes.
- JWT keys, WeChat Pay merchant keys, WeChat platform certs, and TLS certs are mounted from ignored `secrets/` and `certs/` directories.

## Files

- `docker-compose.prod.example.yml`: production-like Compose stack without real secrets.
- `.env.production.example`: copy to `.env.production` and replace every placeholder.
- `infra/nginx/final-review.conf.example`: two-domain HTTPS reverse-proxy template.
- `services/api/cmd/preflight`: production configuration preflight for API, JWT, CORS, WeChat Pay, and mounted secret/file paths.
- `scripts/ops/backup-postgres.sh`: creates a custom-format PostgreSQL dump.
- `scripts/ops/restore-postgres.sh`: restores a dump, guarded by `CONFIRM_RESTORE=yes`.
- `scripts/ops/healthcheck.sh`: checks API readiness plus Web and Admin endpoints; optionally checks Worker readiness when `WORKER_READY_URL` is set.

## Required Secret Layout

Do not commit these files.

```txt
secrets/
  jwt_private.pem
  jwt_public.pem
  wechat/
    apiclient_key.pem
certs/
  tls/
    review.example.com/
      fullchain.pem
      privkey.pem
    admin.review.example.com/
      fullchain.pem
      privkey.pem
  wechat/
    wechatpay_platform.pem
```

The matching `.env.production` values should point to the mounted paths:

```env
JWT_PRIVATE_KEY_PATH=/run/secrets/final-review/jwt_private.pem
JWT_PUBLIC_KEY_PATH=/run/secrets/final-review/jwt_public.pem
WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH=/run/secrets/final-review/wechat/apiclient_key.pem
WECHAT_PAY_PLATFORM_CERTS_DIR=/run/certs/final-review/wechat
```

## Build-Time Frontend Variables

`NEXT_PUBLIC_API_BASE_URL` and `VITE_API_BASE_URL` are build-time values. The production Compose example passes them as Docker build args. If the public domain changes, rebuild Web and Admin; restarting containers is not enough.

## First Deploy

```bash
cp .env.production.example .env.production
# edit .env.production and replace every example domain/password/key value

docker compose --env-file .env.production -f docker-compose.prod.example.yml config --quiet
cd services/api && go run ./cmd/preflight -env-file ../../.env.production
docker compose --env-file .env.production -f docker-compose.prod.example.yml build
docker compose --env-file .env.production -f docker-compose.prod.example.yml up -d
```

To validate the example without creating `.env.production`, set `APP_ENV_FILE=.env.production.example` for the config command.

Seed and imports should be deliberate production actions, not automatic container startup. The current API image contains the server binary only. For production material imports, run the importer from a trusted build/admin workstation with the same database URL and upload mount until a dedicated admin job image is added.

## Health Checks

Container-level:

```bash
docker compose --env-file .env.production -f docker-compose.prod.example.yml ps
docker compose --env-file .env.production -f docker-compose.prod.example.yml logs --tail=100 api worker nginx
```

Public endpoints:

```bash
API_HEALTH_URL=https://review.example.com/readyz \
WEB_URL=https://review.example.com/health \
ADMIN_URL=https://admin.review.example.com \
scripts/ops/healthcheck.sh
```

After TLS is active, require edge security headers as part of the same check:

```bash
CHECK_SECURITY_HEADERS=true \
API_HEALTH_URL=https://review.example.com/readyz \
WEB_URL=https://review.example.com/health \
ADMIN_URL=https://admin.review.example.com \
scripts/ops/healthcheck.sh
```

`/healthz` is liveness and can remain HTTP 200 while a dependency is down. `/readyz` is readiness and must be used before routing traffic. Worker readiness is checked by Docker healthcheck inside the private Compose network; set `WORKER_READY_URL` only if you deliberately expose or port-forward the worker probe endpoint.

## Backups

Create a PostgreSQL dump:

```bash
ENV_FILE=.env.production COMPOSE_FILE=docker-compose.prod.example.yml scripts/ops/backup-postgres.sh
```

The backup script writes the dump with `umask 077`, uses a temporary file before the final rename, and writes a `.sha256` sidecar when `sha256sum` is available.

Restore requires an explicit confirmation flag:

```bash
CONFIRM_RESTORE=yes ENV_FILE=.env.production COMPOSE_FILE=docker-compose.prod.example.yml \
  scripts/ops/restore-postgres.sh backups/postgres/final_review_YYYYMMDDTHHMMSSZ.dump
```

Backups must be copied off the server and encrypted by the deployment operator. The repository ignores dump files.

If a `.sha256` sidecar is present beside the dump, the restore script verifies it before calling `pg_restore`.

## Nginx Config Check

After replacing domains and mounting TLS certificates, validate the edge config before routing traffic:

```bash
docker compose --env-file .env.production -f docker-compose.prod.example.yml run --rm nginx nginx -t
```

The template currently sets HTTPS redirect, HSTS, CSP, frame denial, content-type sniffing protection, referrer policy, permissions policy, `Secure`/`HttpOnly`/`SameSite=Lax` cookie flags, API proxy timeouts, a 25 MB request body limit, and hidden-dotfile denial.

## Release Gate

Before opening paid sales, verify all items below:

- `docker compose --env-file .env.production -f docker-compose.prod.example.yml config --quiet`
- `cd services/api && go run ./cmd/preflight -env-file ../../.env.production` passes against the real mounted production paths
- API `/readyz` returns HTTP 200, and `docker compose ps` shows API and Worker as healthy
- `go test ./...` in `services/api`
- `go test ./...` in `services/worker`
- `npm run build --workspace @final-review/web`
- `npm run build --workspace @final-review/admin`
- production `.env.production` has `APP_ENV=production`, `WECHAT_PAY_MODE=live`, `AUTO_MIGRATE=false`, and an empty `DEV_FIXED_VERIFICATION_CODE`
- `CORS_ALLOWED_ORIGINS` lists exact HTTPS origins and does not use `*`; the API rejects wildcard origins and refuses production startup without exact HTTPS origins
- `docker compose --env-file .env.production -f docker-compose.prod.example.yml run --rm nginx nginx -t`
- API smoke in `docs/internal-smoke.md` passes with a fresh student test email
- WeChat Pay Native live order and notify have been tested with the real merchant dashboard
- material import dry-run report has been reviewed against mounted real files
- manual-grant smoke in `docs/internal-smoke.md` passes with fresh student/admin test accounts after importing real mounted files
- browser delivery smoke `npm --workspace @final-review/web run test:e2e:delivery` passes against Web/Admin/API with fresh student/admin test accounts
- mobile public-page smoke `npm --workspace @final-review/web run test:e2e:mobile` passes against the deployed Web origin at a 390px viewport
- paid material download is denied before entitlement and allowed after a verified paid order
- `scripts/ops/backup-postgres.sh` produces a restorable dump in a staging environment
- Nginx TLS certs, HSTS, CSP, secure cookie flags, and security headers are active; `CHECK_SECURITY_HEADERS=true scripts/ops/healthcheck.sh` passes against the public Web/Admin origins

## Current Limits

- No automated TLS issuance is included yet.
- No managed database backup policy is included yet.
- No Prometheus/Grafana or uptime monitor is included yet.
- No live WeChat merchant settlement reconciliation is included yet.
- The API image currently ships the server binary only; material import remains an operator command from a trusted workstation or future job image.
