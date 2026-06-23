# Security

- Store secrets in environment variables or mounted files, never in Git.
- JWT keys, WeChat Pay keys, LLM API keys, and real course files must not be committed.
- Production must reject mock payment and fixed verification code configuration.
- AI-generated content must be reviewed before publication.
- Admin user, access-grant, course, organization, material, upload, archive, status-change, material-review, wiki entry/proposal-review, blog-review, forum post/reply review, forum best-answer selection, AI draft-review, and report handling operations write server-side operation logs in the same database transaction as the protected mutation.
- Admin user management is server-side restricted to `admin` and `super_admin` roles. Admins cannot change their own role/status, and only `super_admin` users can edit or grant `super_admin`. Frozen users are blocked by backend `RequireNotFrozen` middleware on protected write endpoints.
- Course package management is server-side restricted to `admin` and `super_admin` roles. Creating/updating/archiving packages and binding/unbinding package materials must be authorized on the Go API and logged; frontend route hiding is not a security boundary.
- Public package detail must filter both `materials` and package `items` to published materials. It is not enough to hide unpublished material objects while still returning item resource ids for draft/pending/rejected/archived materials.
- Web package detail can show entitlement state from `/me/entitlements`, but that display does not grant access; paid downloads still require the Go API material download permission check.
- Manual access grants are server-side restricted to admin users, use `manual_admin` source, cannot create or mark payment orders, and can target only published paid/member-only materials or published course packages. Revoked grants are soft-deleted and no longer unlock paid downloads.
- User-scoped forum tracking/resubmission endpoints expose only the authenticated user's own posts/replies and may include that user's review reason, but they do not expose `reviewerId`, `reviewedAt`, or hidden submissions from other users. Resubmission is server-side restricted to draft/pending/needs_changes/rejected content, clears old reviewer metadata, and returns the content to pending review.
- User notification endpoints are user-scoped: authenticated users can list or mark read only their own notifications. Forum, material, wiki, blog, AI draft review, and report result notifications are written in the same database transaction as the protected mutation and operation log.
- CORS must not use wildcard origins with credentials.

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
