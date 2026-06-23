# Security

- Store secrets in environment variables or mounted files, never in Git.
- JWT keys, WeChat Pay keys, LLM API keys, and real course files must not be committed.
- Production must reject mock payment and fixed verification code configuration.
- AI-generated content must be reviewed before publication.
- Admin course, organization, material, upload, archive, status-change, material-review, wiki entry/proposal-review, blog-review, forum post/reply review, forum best-answer selection, and AI draft-review operations write server-side operation logs in the same database transaction as the protected mutation.
- User-scoped forum tracking/resubmission endpoints expose only the authenticated user's own posts/replies and may include that user's review reason, but they do not expose `reviewerId`, `reviewedAt`, or hidden submissions from other users. Resubmission is server-side restricted to draft/pending/needs_changes/rejected content, clears old reviewer metadata, and returns the content to pending review.
- User notification endpoints are user-scoped: authenticated users can list or mark read only their own notifications. Forum, material, wiki, blog, and AI draft review notifications are written in the same database transaction as the review mutation and operation log.
- CORS must not use wildcard origins with credentials.

## Operation Logs

The Go API writes `operation_logs` for the current hardening scope:

- organization create/update/archive: school, college, major
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
