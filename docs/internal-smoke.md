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

## Browser Delivery Smoke

After Web, Admin, and API are all reachable, run the browser smoke from the repository root:

```bash
$env:E2E_DELIVERY_SMOKE="1"
$env:E2E_WEB_BASE_URL="http://127.0.0.1:3000"
$env:E2E_ADMIN_BASE_URL="http://127.0.0.1:5173"
$env:E2E_API_BASE_URL="http://127.0.0.1:8080/api/v1"
$env:E2E_STUDENT_EMAIL="smoke-browser@stu.henu.edu.cn"
$env:E2E_STUDENT_CODE="123456"
$env:E2E_ADMIN_EMAIL="admin@example.com"
$env:E2E_ADMIN_CODE="123456"
npm --workspace @final-review/web run test:e2e:delivery
```

The browser smoke uses separate student/admin browser contexts. It opens the real Web package/material pages, logs in as the student through the Web login page, confirms paid download is denied before entitlement, logs in as admin through the Vue Admin login page, creates an admin-only package grant through the API, then confirms the same student can download the paid material and sees the package as unlocked.

Use a fresh student account for each run. If the student already owns the package, the pre-grant denial check should fail and the test is not proving the paid boundary.

## Quiz Wrong-Question Browser Smoke

After Web and API are reachable, run the quiz smoke from the repository root:

```bash
$env:E2E_QUIZ_SMOKE="1"
$env:E2E_WEB_BASE_URL="http://127.0.0.1:3000"
$env:E2E_API_BASE_URL="http://127.0.0.1:8080/api/v1"
$env:E2E_STUDENT_EMAIL="smoke-quiz@stu.henu.edu.cn"
$env:E2E_STUDENT_CODE="123456"
npm --workspace @final-review/web run test:e2e:quiz
```

This smoke logs in as a student through the real Web login page, opens a real course quiz page, submits an intentionally wrong choice answer, checks the authenticated Go API wrong-question count, and confirms `/me/wrong-questions` renders the question. Use a fresh student account when possible because the test increments wrong-question counters. Set `E2E_QUIZ_COURSE_ID`, `E2E_QUIZ_QUESTION_ID`, or `E2E_QUIZ_WRONG_ANSWER` when the target environment does not use seed data.

## Admin Blog-Review Browser Smoke

After Web, Admin, and API are reachable, run the admin review smoke from the repository root:

```bash
$env:E2E_REVIEW_SMOKE="1"
$env:E2E_WEB_BASE_URL="http://127.0.0.1:3000"
$env:E2E_ADMIN_BASE_URL="http://127.0.0.1:5173"
$env:E2E_API_BASE_URL="http://127.0.0.1:8080/api/v1"
$env:E2E_REVIEW_AUTHOR_EMAIL="smoke-review-author@stu.henu.edu.cn"
$env:E2E_REVIEW_AUTHOR_CODE="123456"
$env:E2E_ADMIN_EMAIL="admin@example.com"
$env:E2E_ADMIN_CODE="123456"
npm --workspace @final-review/web run test:e2e:review
```

This smoke creates a unique pending blog post through the Go API, proves the public detail endpoint returns 404 before review, opens Vue Admin `/blog-reviews`, approves the post through the Admin UI, and verifies the public Web blog detail page renders the approved post. It is opt-in because it mutates blog rows, notifications, and operation logs.

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
- Optional browser smoke: validates the Web/Admin UI path around the same server-side paid boundary
- Optional quiz browser smoke: validates the authenticated quiz submission to wrong-question-book path
- Optional admin review browser smoke: validates pending Blog content remains hidden until approval and becomes public after Admin UI review

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
E2E_DELIVERY_SMOKE=0
E2E_WEB_BASE_URL=http://127.0.0.1:3000
E2E_ADMIN_BASE_URL=http://127.0.0.1:5173
E2E_API_BASE_URL=http://127.0.0.1:8080/api/v1
E2E_STUDENT_EMAIL=smoke-browser@stu.henu.edu.cn
E2E_STUDENT_CODE=123456
E2E_ADMIN_EMAIL=admin@example.com
E2E_ADMIN_CODE=123456
E2E_PACKAGE_ID=
E2E_QUIZ_SMOKE=0
E2E_QUIZ_COURSE_ID=
E2E_QUIZ_QUESTION_ID=
E2E_QUIZ_WRONG_ANSWER=
E2E_REVIEW_SMOKE=0
E2E_REVIEW_AUTHOR_EMAIL=smoke-review-author@stu.henu.edu.cn
E2E_REVIEW_AUTHOR_CODE=123456
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

   UTF-8 manifests with a BOM are accepted, so JSON files saved by Windows tooling should not fail solely because of byte-order marks.

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

8. Run browser delivery smoke with Web/Admin/API base URLs and fresh test accounts:

   ```bash
   npm --workspace @final-review/web run test:e2e:delivery
   ```

9. Run quiz wrong-question smoke with Web/API base URLs and a fresh test account:

   ```bash
   npm --workspace @final-review/web run test:e2e:quiz
   ```

10. Run admin blog-review smoke with Web/Admin/API base URLs and fresh author/admin test accounts:

   ```bash
   npm --workspace @final-review/web run test:e2e:review
   ```

11. For paid-sales testing, use a real WeChat merchant sandbox/internal payment only after the smoke proves unpaid access is denied. Payment success must be confirmed by the backend WeChat notify path, not by frontend polling or manual access-grant smoke.

## Failure Handling

- `api readiness` fails: check Postgres/Redis health and API logs.
- `public packages` fails: seed/import package data before testing.
- `package detail hides storage keys` fails: treat as a data-leak regression.
- `paid material presence` fails: the selected package is not suitable for paid delivery smoke.
- `paid download denied before entitlement` fails with HTTP 200: use a fresh smoke email or investigate paid access leakage.
- `create order` reports already owned: use a fresh smoke email.
- `manual package grant` fails with 401/403: confirm the admin account exists, is active, and has `admin` or `super_admin` role.
- `paid download after grant` fails: inspect `/access-grants`, `/packages/:id`, and package item bindings; the selected package must be published and contain the paid material returned by package detail.
- Browser smoke opens but skips: set `E2E_DELIVERY_SMOKE=1`. It is opt-in because it creates or reuses an access grant.
- Quiz smoke opens but skips: set `E2E_QUIZ_SMOKE=1`. It is opt-in because it writes wrong-question records.
- Review smoke opens but skips: set `E2E_REVIEW_SMOKE=1`. It is opt-in because it creates and approves a Blog post.
- Browser smoke cannot log in: development can use `DEV_FIXED_VERIFICATION_CODE`; staging/production needs a real test inbox or manually supplied current code.
