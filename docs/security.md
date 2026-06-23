# Security

- Store secrets in environment variables or mounted files, never in Git.
- JWT keys, WeChat Pay keys, LLM API keys, and real course files must not be committed.
- Production must reject mock payment and fixed verification code configuration.
- AI-generated content must be reviewed before publication.
- Admin course, organization, material, upload, archive, status-change, material-review, wiki-review, blog-review, and AI draft-review operations write server-side operation logs in the same database transaction as the protected mutation.
- CORS must not use wildcard origins with credentials.

## Operation Logs

The Go API writes `operation_logs` for the current hardening scope:

- organization create/update/archive: school, college, major
- course create/update/archive
- material create/upload/update/status-update/archive
- material approve/reject review
- wiki entry approve/reject review
- blog post approve/reject review
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
