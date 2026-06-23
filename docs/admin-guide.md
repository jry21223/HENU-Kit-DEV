# Admin Guide

The Vue admin console is intentionally narrow during the V2 MVP. It only exposes flows that are backed by server-side role checks in the Go API.

## Current Pages

- `/dashboard`: module status summary.
- `/users`: user listing, role update, and active/frozen status management.
- `/access-grants`: manual material/package access grants for internal testing and after-sales delivery.
- `/packages`: admin-only course package CRUD and package-material binding page.
- `/courses`: organization-backed course creation, all-status listing, editing, and archiving.
- `/materials`: local material upload, all-status material listing, metadata editing, and material status operations.
- `/downloads`: successful material download audit logs.
- `/material-reviews`: reviewer/admin material approve/reject review queue with review notes.
- `/wiki-reviews`: reviewer/admin wiki entry approve/reject review queue with review notes.
- `/wiki-proposal-reviews`: reviewer/admin wiki edit proposal approve/reject queue with stale-version protection.
- `/blog-reviews`: reviewer/admin blog post approve/reject review queue with review notes.
- `/forum-reviews`: reviewer/admin forum post approve/reject review queue with review notes.
- `/forum-reply-reviews`: reviewer/admin forum reply approve/reject review queue with review notes.
- `/ai/drafts`: AI task visibility and AI draft approve/reject review operations with review notes.
- `/analytics`: read-only material download and course demand analytics.
- `/operation-logs`: read-only high-risk admin operation logs.

## User Management

`/users` calls:

- `GET /api/v1/admin/users?email=&role=&status=&limit=`
- `PATCH /api/v1/admin/users/:id`

The page is available only to `admin` and `super_admin` roles. It provides a compact user list with email, display name, role, active/frozen status, profile binding hints, points balance, verification state, and creation time.

Important boundaries:

- Reviewer-only users cannot access the user management page or API.
- The UI edits only display name, role, and active/frozen status; it does not edit email, points, membership, or password credentials.
- Valid roles are `user`, `creator`, `reviewer`, `operator`, `admin`, and `super_admin`.
- Valid statuses are `active` and `frozen`.
- Admin users cannot change their own role or status through this endpoint.
- Only `super_admin` users can edit an existing `super_admin` account or grant the `super_admin` role.
- Freezing a user is enforced by server-side `RequireNotFrozen` checks on protected write endpoints; it is not a frontend-only toggle.
- User updates write `operation_logs` with previous/current role and status metadata.

## Access Grants

`/access-grants` calls:

- `GET /api/v1/admin/access-grants?userId=&materialId=&packageId=&source=&active=&limit=`
- `POST /api/v1/admin/access-grants`
- `DELETE /api/v1/admin/access-grants/:id`

The page is available only to `admin` and `super_admin` roles. It supports manual delivery during internal testing or after-sales handling by granting a user access to a paid/member-only material or a published course package.

Important boundaries:

- Manual grants use source `manual_admin`; they do not create payment orders, mark orders as paid, or simulate a payment callback.
- Material grants can target only `published` materials with `access_level=paid` or `member_only`.
- Package grants can target only `published` course packages.
- Duplicate active grants for the same user/resource return the existing grant instead of creating another record.
- Revoking a grant soft-deletes it. The revoked grant is removed from `/me/entitlements` and no longer unlocks paid downloads.
- The page does not expose raw file storage keys and does not send PDF files directly.
- Grant create/revoke operations write `operation_logs`.

## Course Package Management

`/packages` is the admin-side course package maintenance page. It calls:

- `GET /api/v1/admin/packages?schoolId=&majorId=&courseId=&grade=&status=`
- `POST /api/v1/admin/packages`
- `PATCH /api/v1/admin/packages/:id`
- `DELETE /api/v1/admin/packages/:id`
- `GET /api/v1/admin/packages/:id/items`
- `POST /api/v1/admin/packages/:id/items`
- `DELETE /api/v1/admin/packages/:id/items/:itemId`

The page is available only to `admin` and `super_admin` roles. It is separate from `/access-grants`: package management defines the product and its included materials, while access grants deliver permission to a specific user.

Important boundaries:

- Package create/edit operations require server-side admin authorization and should reject reviewer-only users.
- Valid package statuses are `draft`, `published`, and `archived`.
- `priceFen` is the source admin field for price and is always integer cents; the UI may display yuan, but the API contract should not use floating-point currency.
- Publishing a package makes only the package shell visible. Public package detail must still filter included `items` and `materials` to published materials only.
- Package item binding initially supports `resourceType=material`; binding a material to a package does not change the material status or storage key.
- Draft, pending, rejected, and archived materials may be staged in admin, but public package APIs must not reveal those item ids or material metadata.
- Duplicate active material bindings should be idempotent or explicitly rejected without creating duplicate package items.
- Removing a package item only unbinds the relation. It must not delete the underlying material, revoke existing user grants, or rewrite download logs.
- Package create/update/archive and package item bind/unbind operations should write `operation_logs` rows server-side.

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
- `rejected`: reviewed and rejected, hidden from public pages
- `archived`: hidden from public pages

Important boundaries:

- Upload and manual create default to `draft` when no status is provided.
- The admin UI can edit course binding, title, type, description, preview content, access level, and status.
- The admin UI can move materials to `pending`, `published`, `rejected`, `draft`, or `archived`.
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

## Blog Review

`/blog-reviews` calls:

- `GET /api/v1/admin/blog/posts?status=`
- `POST /api/v1/admin/blog/posts/:id/approve`
- `POST /api/v1/admin/blog/posts/:id/reject`

The page is available to `reviewer`, `admin`, and `super_admin` roles. It lists student-submitted blog posts by review status and does not provide edit/delete controls.

Important boundaries:

- Logged-in users submit blog posts through `POST /api/v1/blog/posts`; posts always enter `pending`.
- Public blog list/detail endpoints only expose `published` posts.
- Approving a post sets it to `published`, records reviewer metadata, and makes it visible through public blog endpoints.
- Rejecting a post requires `reviewReason`, records reviewer metadata, and keeps it hidden from public blog endpoints.
- Only `draft`, `pending`, and `needs_changes` posts can be reviewed; repeating review on published/rejected posts returns `409 post_not_reviewable`.
- Blog approve/reject operations write `operation_logs` rows server-side; rejected repeat-review attempts do not write extra log rows.

## Wiki Review

`/wiki-reviews` calls:

- `GET /api/v1/admin/wiki/entries?status=`
- `POST /api/v1/admin/wiki/entries/:id/approve`
- `POST /api/v1/admin/wiki/entries/:id/reject`

The page is available to `reviewer`, `admin`, and `super_admin` roles. It lists creator-submitted wiki entries by review status and does not provide edit/delete controls.

Important boundaries:

- Creator/admin users submit wiki entries through `POST /api/v1/wiki/entries`; entries always enter `pending`.
- Public wiki list/detail endpoints only expose `published` entries with `visibility=public`.
- Public wiki responses use a public DTO and do not expose review metadata.
- Approving an entry sets it to `published`, records reviewer metadata, and makes it visible through public wiki endpoints.
- Rejecting an entry requires `reviewReason`, records reviewer metadata, and keeps it hidden from public wiki endpoints.
- Only `draft`, `pending`, and `needs_changes` entries can be reviewed; repeating review on published/rejected entries returns `409 entry_not_reviewable`.
- The initial submission writes a `wiki_edit_histories` version-1 row.
- Wiki approve/reject operations write `operation_logs` rows server-side; rejected repeat-review attempts do not write extra log rows.

## Wiki Proposal Review

`/wiki-proposal-reviews` calls:

- `GET /api/v1/admin/wiki/proposals?status=`
- `POST /api/v1/admin/wiki/proposals/:id/approve`
- `POST /api/v1/admin/wiki/proposals/:id/reject`

The page is available to `reviewer`, `admin`, and `super_admin` roles. It lists proposed edits to already-published wiki entries and does not expose direct live-content editing controls.

Important boundaries:

- Creator/admin users submit proposals through `POST /api/v1/wiki/entries/:id/proposals`.
- Proposals can target only published public wiki entries, and public wiki content stays unchanged while a proposal is pending.
- Each proposal stores the live entry `baseVersion` at submission time.
- The review queue returns base-version history content, current live entry content/version/status, and an `isStale` flag so reviewers can compare base, current, and proposed content before acting.
- Approving a proposal requires the live entry version to still match `baseVersion`; approval updates the live entry, increments its version, writes a new `wiki_edit_histories` row, and records reviewer metadata in one transaction.
- If the live entry changed after proposal creation, approval returns `409 proposal_stale`, leaves the proposal pending, and does not update public content.
- Rejecting a proposal requires `reviewReason`, records reviewer metadata, and does not mutate the live entry.
- Only `draft`, `pending`, and `needs_changes` proposals can be reviewed; repeating review on published/rejected proposals returns `409 proposal_not_reviewable`.
- Wiki proposal approve/reject operations write `operation_logs` rows with target type `wiki_edit_proposal`; stale or rejected repeat-review attempts do not write extra log rows.

## Forum Review

`/forum-reviews` calls:

- `GET /api/v1/admin/forum/posts?status=`
- `POST /api/v1/admin/forum/posts/:id/approve`
- `POST /api/v1/admin/forum/posts/:id/reject`

The page is available to `reviewer`, `admin`, and `super_admin` roles. It lists student-submitted forum posts by review status and does not provide edit/delete controls.

Important boundaries:

- Logged-in, non-frozen users submit forum posts through `POST /api/v1/forum/posts`; posts always enter `pending`.
- Public forum post list/detail endpoints only expose `published` posts with `visibility=public` under a published board.
- Public forum post responses use a public DTO and do not expose review metadata.
- Approving a post sets it to `published`, records reviewer metadata, and makes it visible through public forum endpoints.
- Rejecting a post requires `reviewReason`, records reviewer metadata, and keeps it hidden from public forum endpoints.
- Only `draft`, `pending`, and `needs_changes` posts can be reviewed; repeating review on published/rejected posts returns `409 forum_post_not_reviewable`.
- The MVP supports `normal`, `question`, and `reward` post types.
- Reward posts freeze author points when submitted, show reward points/status in the review queue, keep points escrowed after approval, refund points automatically if rejected, and settle to the selected best-answer author through the public authenticated API.
- Forum approve/reject operations write `operation_logs` rows server-side; rejected repeat-review attempts do not write extra log rows.

## Forum Reply Review

`/forum-reply-reviews` calls:

- `GET /api/v1/admin/forum/replies?status=`
- `POST /api/v1/admin/forum/replies/:id/approve`
- `POST /api/v1/admin/forum/replies/:id/reject`

The page is available to `reviewer`, `admin`, and `super_admin` roles. It lists student-submitted forum replies by review status and does not provide edit/delete controls.

Important boundaries:

- Logged-in, non-frozen users submit replies through `POST /api/v1/forum/posts/:id/replies`.
- Replies can only be submitted to published public posts under published forum boards.
- Public forum post detail only exposes published replies.
- Approving a reply sets it to `published`, records reviewer metadata, increments the parent post `comment_count` once, and makes it visible through public post detail.
- Rejecting a reply requires `reviewReason`, records reviewer metadata, and keeps it hidden from public post detail.
- Only `draft`, `pending`, and `needs_changes` replies can be reviewed; repeating review on published/rejected replies returns `409 forum_reply_not_reviewable`.
- Post authors, `admin`, and `super_admin` users can select one published reply as best answer through `POST /api/v1/forum/replies/:id/mark-best`; reward posts settle escrowed points exactly once through a `forum_reward_settlement` points log.
- The current admin reply review page does not expose a best-answer button yet; reply editing and richer user-facing controls remain later work.
- Forum reply approve/reject and best-answer selection operations write `operation_logs` rows server-side; rejected repeat-review attempts do not write extra log rows.

## AI Draft Review

`/ai/drafts` calls:

- `GET /api/v1/admin/ai/tasks`
- `GET /api/v1/admin/ai/drafts`
- `POST /api/v1/admin/ai/drafts/:id/approve`
- `POST /api/v1/admin/ai/drafts/:id/reject`

The page shows recent AI tasks and reviewable AI drafts created by the worker.

Important boundaries:

- AI review endpoints require an authenticated `reviewer`, `admin`, or `super_admin` role.
- `reviewer` users can access `/ai/drafts`, `/material-reviews`, `/wiki-reviews`, `/wiki-proposal-reviews`, `/blog-reviews`, `/forum-reviews`, and `/forum-reply-reviews`, but they cannot access course, material CRUD, download, analytics, operation logs, or other admin-only pages.
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

The Go API currently writes operation logs for organization, course, material, upload, archive, material status, material review, wiki entry/proposal review, blog review, forum post/reply review, forum best-answer selection, and AI draft review mutations. Each log records the authenticated operator, action, target type/id, IP, User-Agent, and minimal metadata.

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
- report totals and target/status distribution

Important boundaries:

- Analytics are derived from server-side successful download logs and current course/material/package records.
- Denied downloads are not counted as demand because they are not successful delivery events.
- The page is read-only and does not grant access, mutate material status, or expose `storage_key`.
- This MVP analytics view does not yet include page visits, search intent, payment conversion, or course request voting.

## Reports

`/reports` calls `GET /api/v1/admin/reports` and requires an authenticated `reviewer`, `admin`, or `super_admin` role.

The page supports:

- filtering reports by status and target type
- listing reporter id, target id/type, report reason, and processing record
- resolving pending reports through `POST /api/v1/admin/reports/:id/resolve`
- rejecting pending reports through `POST /api/v1/admin/reports/:id/reject`

Important boundaries:

- Rejection requires a review reason.
- Already handled reports cannot be overwritten from the UI because the server returns `409 report_not_reviewable`.
- Report handling writes operation logs and sends reporter notifications server-side.
- The page does not expose hidden target content or make moderation decisions without the reviewer action.

## Planned Areas

- Richer content review
- Points and memberships
- Orders
- System config
- Automatic operation-log retention cleanup after a production-safe archival flow exists
