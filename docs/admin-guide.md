# Admin Guide

The Vue admin console is intentionally narrow during the V2 MVP. It only exposes flows that are backed by server-side role checks in the Go API.

## Current Pages

- `/dashboard`: module status summary.
- `/courses`: organization-backed course creation, all-status listing, editing, and archiving.
- `/materials`: local material upload, all-status material listing, metadata editing, and material status operations.
- `/downloads`: successful material download audit logs.
- `/ai/drafts`: AI task visibility and AI draft approve/reject review operations with review notes.
- `/analytics`: read-only material download and course demand analytics.
- `/operation-logs`: read-only high-risk admin operation logs.

## Course Operations

`/courses` calls:

- `GET /api/v1/admin/courses`
- `POST /api/v1/admin/courses`
- `PATCH /api/v1/admin/courses/:id`
- `DELETE /api/v1/admin/courses/:id`

The admin course list returns `draft`, `published`, and `archived` courses. Public course APIs continue to expose only `published` courses.

Important boundaries:

- Course create and edit operations require an authenticated `admin` or `super_admin` role.
- Valid course statuses are `draft`, `published`, and `archived`.
- Deleting a course archives it instead of hard-deleting data.
- The edit dialog updates course organization, grade, slug, description, exam scope, and status through the Go API.
- Course create/update/archive operations write `operation_logs` rows server-side.

## Material Operations

`/materials` calls:

- `GET /api/v1/admin/materials`
- `POST /api/v1/admin/materials/upload`
- `PATCH /api/v1/admin/materials/:id`
- `PATCH /api/v1/admin/materials/:id/status`
- `DELETE /api/v1/admin/materials/:id`

All material admin endpoints require an authenticated `admin` or `super_admin` role.

Status flow used by the MVP:

- `draft`: stored but hidden from public pages
- `pending`: submitted for review, still hidden from public pages
- `published`: visible to public material list/detail and eligible for server-side download permission checks
- `archived`: hidden from public pages

Important boundaries:

- Upload and manual create default to `draft` when no status is provided.
- The admin UI can edit course binding, title, type, description, preview content, access level, and status.
- The admin UI can move materials to `pending`, `published`, `draft`, or `archived`.
- Public APIs only expose `published` materials.
- Invalid statuses, material types, and access levels are rejected by the Go API.
- The UI does not display `storage_key`; downloads still go through `GET /api/v1/materials/:id/download`.
- The edit dialog does not expose or mutate `storage_key`; the Go API also rejects metadata PATCH attempts that include `storageKey`, `fileName`, or `fileSize`.
- Replacing the actual file remains an upload flow.
- Material create/upload/update/status-update/archive operations write `operation_logs` rows server-side.

## Download Audit

`/downloads` calls `GET /api/v1/admin/downloads` and requires an authenticated `admin` or `super_admin` role.

The table shows:

- material title and material id
- access level
- user id, or anonymous when the download did not require login
- IP
- User-Agent
- download time

Filters:

- `materialId`
- `userId`

Important boundaries:

- The admin UI only reads audit logs; it does not grant access or mutate download records.
- Failed authorization, unsafe storage keys, and missing files are not recorded as successful downloads.
- PDF downloads are dynamically watermarked by the Go API using a temporary copy; the stored source PDF is not overwritten.
- Non-PDF downloads are served unchanged and explicitly return `X-Watermark-Applied: false`.
- PDF watermark failures return an API error instead of falling back to an unwatermarked PDF.
- `storage_key` is not returned in material JSON and is not displayed in the admin UI.

## Material Review

`/material-reviews` calls:

- `GET /api/v1/admin/material-reviews?status=`
- `POST /api/v1/admin/materials/:id/approve`
- `POST /api/v1/admin/materials/:id/reject`

The page is available to `reviewer`, `admin`, and `super_admin` roles. It lists reviewable material records without exposing storage keys or upload/edit/delete controls.

Important boundaries:

- Reviewer users can approve or reject pending materials, but they still cannot access material CRUD, course CRUD, download audit, analytics, or operation logs.
- Approving a material sets it to `published`, records reviewer metadata, and makes it visible through public material endpoints.
- Rejecting a material requires `reviewReason`, records reviewer metadata, and keeps it hidden from public material endpoints.
- Only `pending` materials can be reviewed; repeating review on published/rejected/archived materials returns `409 material_not_reviewable`.
- Material approve/reject operations write `operation_logs` rows server-side; rejected repeat-review attempts do not write extra log rows.

## AI Draft Review

`/ai/drafts` calls:

- `GET /api/v1/admin/ai/tasks`
- `GET /api/v1/admin/ai/drafts`
- `POST /api/v1/admin/ai/drafts/:id/approve`
- `POST /api/v1/admin/ai/drafts/:id/reject`

The page shows recent AI tasks and reviewable AI drafts created by the worker.

Important boundaries:

- AI review endpoints require an authenticated `reviewer`, `admin`, or `super_admin` role.
- `reviewer` users can access `/ai/drafts` and `/material-reviews`, but they cannot access course, material CRUD, download, analytics, operation logs, or other admin-only pages.
- Approving a draft can include an optional review note; rejecting a draft requires a review reason.
- Approving or rejecting a draft only changes the draft review status and review metadata.
- Only `draft`, `pending`, and `needs_changes` drafts can be reviewed; repeating review on `approved` or `rejected` drafts returns `409 draft_not_reviewable`.
- The MVP does not automatically publish AI drafts as materials, questions, wiki entries, or papers.
- The UI displays task input/result and draft content for review, but it does not call any LLM directly.
- Real LLM/RAG and publish-to-resource flows remain later work.
- AI draft approve/reject operations write `operation_logs` rows server-side; rejected repeat-review attempts do not write extra log rows.

## Operation Logs

`/operation-logs` calls the following endpoints and requires an authenticated `admin` or `super_admin` role:

- `GET /api/v1/admin/operation-logs`
- `GET /api/v1/admin/operation-logs/export`
- `GET /api/v1/admin/operation-logs/retention`

The Go API currently writes operation logs for organization, course, material, upload, archive, material status, material review, and AI draft review mutations. Each log records the authenticated operator, action, target type/id, IP, User-Agent, and minimal metadata.

Filters:

- `operatorId`
- `action`
- `targetType`
- `targetId`
- `createdFrom`
- `createdTo`
- `limit`

Important boundaries:

- The page is read-only.
- Logs cannot be edited or deleted from Vue Admin.
- CSV export uses the same filters as the table and is capped by `OPERATION_LOG_EXPORT_LIMIT`.
- The retention panel shows `OPERATION_LOG_RETENTION_DAYS`; automatic deletion is not enabled in the MVP.
- Reviewer-only users cannot access operation logs.
- Invalid or rejected mutation requests do not write operation log rows.

## Analytics

`/analytics` calls `GET /api/v1/admin/analytics/overview` and requires an authenticated `admin` or `super_admin` role.

The page shows:

- total users, courses, materials, published materials, course packages, and successful downloads
- 14-day successful material download trend
- top downloaded materials
- access-level download breakdown
- course demand rows sorted by download count and material supply

Important boundaries:

- Analytics are derived from server-side successful download logs and current course/material/package records.
- Denied downloads are not counted as demand because they are not successful delivery events.
- The page is read-only and does not grant access, mutate material status, or expose `storage_key`.
- This MVP analytics view does not yet include page visits, search intent, payment conversion, or course request voting.

## Planned Areas

- Users and roles
- Richer content review
- Points and memberships
- Orders
- Reports
- System config
- Automatic operation-log retention cleanup after a production-safe archival flow exists
