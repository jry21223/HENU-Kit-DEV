# Security

- Store secrets in environment variables or mounted files, never in Git.
- JWT keys, WeChat Pay keys, LLM API keys, and real course files must not be committed.
- Production must reject mock payment and fixed verification code configuration.
- Run `cd services/api && go run ./cmd/preflight -env-file ../../.env.production` before a production deploy. The preflight rejects non-production `APP_ENV`, wildcard/non-HTTPS CORS, `AUTO_MIGRATE=true`, fixed verification codes, placeholder database/Redis/WeChat values, invalid JWT keys, mock WeChat Pay, invalid API v3 key length, non-HTTPS notify URLs, missing platform certs, and missing upload/key paths.
- AI-generated content must be reviewed before publication.
- Admin user, access-grant, course, organization, material, upload, archive, status-change, material-review, wiki entry/proposal-review, blog-review, forum post/reply review, forum best-answer selection, AI draft-review/publish, and report handling operations write server-side operation logs in the same database transaction as the protected mutation.
- Admin user management is server-side restricted to `admin` and `super_admin` roles. Admins cannot change their own role/status, and only `super_admin` users can edit or grant `super_admin`. Frozen users are blocked by backend `RequireNotFrozen` middleware on protected write endpoints.
- Course package management is server-side restricted to `admin` and `super_admin` roles. Creating/updating/archiving packages and binding/unbinding package materials must be authorized on the Go API and logged; frontend route hiding is not a security boundary.
- Public package detail must filter both `materials` and package `items` to published materials. It is not enough to hide unpublished material objects while still returning item resource ids for draft/pending/rejected/archived materials.
- Web package detail can show entitlement state from `/me/entitlements`, but that display does not grant access; paid downloads still require the Go API material download permission check.
- Frozen users cannot download `login_required`, `paid`, or `member_only` materials even when they have an active grant. Free materials remain publicly downloadable.
- Material manifest import accepts only files that already exist under `LOCAL_UPLOAD_DIR`; traversal paths and missing files fail the transaction, dry-run/import responses include an acceptance `report`, and real course files remain ignored by Git.
- Course package orders are server-priced records. Creating an order or generating a WeChat Native mock code URL does not mark payment success, create entitlement, or bypass paid download checks.
- WeChat Native order expiry is enforced server-side: stale pending/paying orders with `expires_at <= now` become `expired`, cannot continue creating code URLs, and are not reused by new package order creation.
- WeChat Native close-order handling is server-side only: users can close only their own pending/paying orders, admins can close any pending/paying order, and paid/closed orders cannot be closed or used to revoke entitlement.
- Development/test mock WeChat notify requires an HMAC header derived from `WECHAT_PAY_API_V3_KEY`, verifies order number and amount, and grants package entitlement idempotently only after a signed `SUCCESS` payload. This is a local test harness and is rejected in production.
- WeChat Pay live Native ordering is signed with the merchant private key and verifies the signed WeChat response against the configured platform certificate/public key before storing `code_url`; live notify processing verifies the callback signature, decrypts the encrypted resource, checks appid/mchid and amount, and grants package entitlement idempotently only after a verified `SUCCESS` transaction. Real merchant end-to-end payment verification is still required before production sales.
- `WECHAT_PAY_MODE=mock` is allowed only outside production; production mock configuration is rejected by the API payment boundary.
- The admin order browser is read-only. It can inspect order status and whether an entitlement already exists, but it cannot mark orders paid or issue grants.
- The admin order browser can display and filter `risk_flag` values for payment triage, but risk visibility is not an automated alerting or settlement system.
- The admin payment reconciliation report is read-only. It can flag local inconsistencies across orders, payment records, order-source grants, risk flags, and open incidents, but it cannot repair payment data, mark orders paid, resolve incidents, or grant entitlement.
- WeChat callback anomalies that fail trust checks are recorded as `payment_incidents` for manual triage. Unknown orders, amount mismatches, and transaction-id conflicts do not update orders or grant entitlement.
- The payment-incident admin page can only mark incidents `resolved` or `ignored`. It writes operation logs but deliberately cannot mark payment success, insert trusted payment records, or issue package/material grants.
- Payment incident webhooks are best-effort operator alerts only. They exclude raw WeChat notify bodies, can be signed with `PAYMENT_INCIDENT_WEBHOOK_SECRET`, and must never be treated by receivers as proof of payment.
- Manual access grants are server-side restricted to admin users, use `manual_admin` source, cannot create or mark payment orders, and can target only published paid/member-only materials or published course packages. Revoked grants are soft-deleted and no longer unlock paid downloads.
- User-scoped forum tracking/resubmission endpoints expose only the authenticated user's own posts/replies and may include that user's review reason, but they do not expose `reviewerId`, `reviewedAt`, or hidden submissions from other users. Resubmission is server-side restricted to draft/pending/needs_changes/rejected content, clears old reviewer metadata, and returns the content to pending review.
- User notification endpoints are user-scoped: authenticated users can list or mark read only their own notifications. Forum, material, wiki, blog, AI draft review, and report result notifications are written in the same database transaction as the protected mutation and operation log.
- The Go API sets baseline security headers on every response: `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, and a restrictive API CSP. Production responses also include HSTS.
- CORS must not use wildcard origins with credentials. The API rejects wildcard origins in all environments and refuses to start in production unless `CORS_ALLOWED_ORIGINS` contains exact HTTPS origins.

## Edge and Backup Controls

- Production Nginx must be validated with `nginx -t` after replacing domains and mounting TLS certificates.
- The Nginx template sets HTTPS redirects, HSTS, CSP, frame denial, content-type sniffing protection, referrer policy, permissions policy, cross-origin opener policy, secure API cookie flags, proxy timeouts, a 25 MB request body limit, and hidden-dotfile denial.
- After TLS is active, `CHECK_SECURITY_HEADERS=true scripts/ops/healthcheck.sh` should pass against both public Web/Admin origins.
- PostgreSQL backups are written with `umask 077` through a temporary file before final rename. When `sha256sum` is available, backups get a `.sha256` sidecar, and restore verifies that sidecar before `pg_restore`.
- Backup files must still be copied off-server and encrypted by the deployment operator; the repository script only creates and verifies local dumps.

## Operation Logs

The Go API writes `operation_logs` for the current hardening scope:

- organization create/update/archive: school, college, major
- user display name, role, and active/frozen status update
- access grant create/revoke
- course package create/update/archive and package item bind/unbind
- course create/update/archive
- material create/upload/update/status-update/archive
- material approve/reject review
- wiki entry approve/reject review
- wiki edit proposal approve/reject review
- blog post approve/reject review
- forum post approve/reject review
- forum reply approve/reject review
- forum best-answer selection and reward settlement
- AI draft approve/reject review
- report resolve/reject handling
- payment incident resolve/ignore handling

Log rows include the authenticated operator id, action, target type/id, IP, User-Agent, and minimal metadata. Invalid or rejected requests do not write operation logs. Vue Admin exposes a read-only operation-log browser with time filtering, CSV export, and a retention policy panel; logs cannot be edited or deleted from the admin UI. CSV export is admin-only, filter-aware, and capped by `OPERATION_LOG_EXPORT_LIMIT`. Automatic operation-log deletion is not enabled in the MVP.

## Dependency Checks

Run these before release-oriented pushes:

```bash
npm audit --audit-level=low
npm audit --prefix legacy/v1-next-prisma --audit-level=low
python -m pip_audit -r integrations/langbot-sales-agent/requirements.txt
cd services/api && go test ./...
cd services/worker && go test ./...
cd services/api && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
cd services/worker && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

The active Go services target Go 1.25 and should keep security-sensitive `golang.org/x/*` modules current. If GitHub reports Dependabot alerts while local npm/pip audits pass, check Go module advisories and the archived legacy manifest before dismissing the alert as stale.
