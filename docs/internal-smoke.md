# Internal Smoke Runbook

Use this runbook before an internal test, after deploy changes, and after importing real course materials. It is intentionally conservative: the default API smoke does not mark orders paid, does not grant entitlement, and does not bypass paid download checks.

## API Smoke CLI

Run from the API service directory:

```bash
cd services/api
go run ./cmd/smoke \
  -base-url http://localhost:8080/api/v1 \
  -email smoke@stu.henu.edu.cn \
  -code 123456 \
  -create-order
```

Development mode can omit `-code` when the API returns `devCode`. Production and internal-test environments should pass the real email code manually.

The command prints JSON and exits non-zero if a required check fails.

To verify the internal manual-delivery path after a real material import, use a fresh student account and an admin account:

```bash
cd services/api
go run ./cmd/smoke \
  -base-url http://localhost:8080/api/v1 \
  -email smoke@stu.henu.edu.cn \
  -code 123456 \
  -admin-email admin@example.com \
  -admin-code 123456 \
  -grant-package-access
```

This path first proves the selected paid material is denied before entitlement, then calls the admin-only access-grant API, then verifies the same student can download the paid material. It is for internal manual delivery and after-sales validation only; it is not a substitute for WeChat notify confirmation in the real payment flow.

## What It Checks

- API readiness: `GET /readyz`
- Public catalog: `GET /schools` and `GET /packages`
- Course package detail: `GET /packages/:id`
- Public package detail does not expose `storageKey`
- Selected package contains at least one `paid` or `member_only` material
- Student email login works
- `GET /auth/me` works with the returned bearer token
- Paid material download is denied before entitlement
- Optional `-create-order`: creates/reuses a local pending package order and reads its status
- Optional `-grant-package-access`: logs in as admin, grants the selected published package to the smoke user, and verifies paid download succeeds after the server-side grant

## Important Flags

- `-base-url`: API base URL including `/api/v1`
- `-email`: student test account email
- `-code`: verification code; required outside development unless the user manually reads the code
- `-package-id`: package id to test; defaults to the first published package
- `-skip-login`: only checks public endpoints and package safety
- `-create-order`: mutates data by creating or reusing a pending order; it does not mark payment success
- `-admin-email`: admin test account email used only with `-grant-package-access`
- `-admin-code`: admin verification code used only with `-grant-package-access`
- `-grant-package-access`: mutates data by creating/reusing a manual admin package grant; it does not create a payment order or mark an order paid
- `-expect-paid-denied=false`: use only if the smoke account already has entitlement and you deliberately cannot test unpaid denial

## Environment Variables

The CLI also reads:

```env
SMOKE_API_BASE_URL=http://localhost:8080/api/v1
SMOKE_EMAIL=smoke@stu.henu.edu.cn
SMOKE_CODE=123456
SMOKE_ADMIN_EMAIL=admin@example.com
SMOKE_ADMIN_CODE=123456
SMOKE_PACKAGE_ID=
SMOKE_CREATE_ORDER=false
SMOKE_GRANT_PACKAGE_ACCESS=false
SMOKE_SKIP_LOGIN=false
SMOKE_EXPECT_PAID_DENIED=true
SMOKE_TIMEOUT_SECONDS=15
```

## Internal-Test Sequence

1. Deploy and confirm Compose health:

   ```bash
   docker compose --env-file .env.production -f docker-compose.prod.example.yml ps
   ```

2. Run readiness checks:

   ```bash
   API_HEALTH_URL=https://review.example.com/readyz \
   WEB_URL=https://review.example.com/health \
   ADMIN_URL=https://admin.review.example.com \
   scripts/ops/healthcheck.sh
   ```

3. Run material import dry-run against mounted real files and review the `report` block:

   ```bash
   cd services/api
   go run ./cmd/import-materials -dry-run ../../data/material-manifest.example.json
   ```

4. Run real material import only after the report matches expected package/material counts.

5. Run API smoke with a fresh student test email:

   ```bash
   cd services/api
   go run ./cmd/smoke -base-url https://review.example.com/api/v1 -email smoke@stu.henu.edu.cn -create-order
   ```

6. For manual-delivery internal testing, run the access-grant smoke with a fresh student test email and an admin email:

   ```bash
   cd services/api
   go run ./cmd/smoke \
     -base-url https://review.example.com/api/v1 \
     -email smoke-manual@stu.henu.edu.cn \
     -admin-email admin@example.com \
     -grant-package-access
   ```

   Use the real email codes in non-development environments. This should pass only after paid download is denied before the grant and allowed after the admin package grant.

7. In Vue Admin, inspect:

   - `/orders`
   - `/payment-reconciliation`
   - `/payment-incidents`
   - `/access-grants`
   - `/downloads`

8. For paid-sales testing, use a real WeChat merchant sandbox/internal payment only after the smoke proves unpaid access is denied. Payment success must be confirmed by the backend WeChat notify path, not by frontend polling or manual access-grant smoke.

## Failure Handling

- `api readiness` fails: check Postgres/Redis health and API logs.
- `public packages` fails: seed/import package data before testing.
- `package detail hides storage keys` fails: treat as a data-leak regression.
- `paid material presence` fails: the selected package is not suitable for paid delivery smoke.
- `paid download denied before entitlement` fails with HTTP 200: use a fresh smoke email or investigate paid access leakage.
- `create order` reports already owned: use a fresh smoke email.
- `manual package grant` fails with 401/403: confirm the admin account exists, is active, and has `admin` or `super_admin` role.
- `paid download after grant` fails: inspect `/access-grants`, `/packages/:id`, and package item bindings; the selected package must be published and contain the paid material returned by package detail.
