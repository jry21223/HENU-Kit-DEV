# V2 API

Base path: `/api/v1`

Currently implemented endpoints:

- `GET /healthz`
- `GET /api/v1/healthz`
- `GET /api/v1/version`
- `POST /api/v1/auth/send-code`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `PATCH /api/v1/auth/me`
- `GET /api/v1/schools`
- `GET /api/v1/colleges?schoolId=`
- `GET /api/v1/majors?schoolId=&collegeId=`
- `GET /api/v1/courses?schoolId=&majorId=&grade=`
- `GET /api/v1/courses/:id`
- `GET /api/v1/courses/:id/materials`
- `GET /api/v1/courses/:id/packages`
- `GET /api/v1/courses/:id/questions`
- `GET /api/v1/materials?courseId=`
- `GET /api/v1/materials/:id`
- `GET /api/v1/materials/:id/download`
- `GET /api/v1/packages?courseId=&schoolId=&majorId=&grade=`
- `GET /api/v1/packages/:id`
- `GET /api/v1/questions/:id`
- `POST /api/v1/questions/:id/submit`
- `GET /api/v1/blog/posts?limit=`
- `GET /api/v1/blog/posts/:id`
- `POST /api/v1/blog/posts`
- `GET /api/v1/forum/boards`
- `GET /api/v1/forum/posts?boardId=&limit=`
- `GET /api/v1/forum/posts/:id`
- `POST /api/v1/forum/posts`
- `POST /api/v1/forum/posts/:id/replies`
- `GET /api/v1/wiki/entries?courseId=&limit=`
- `GET /api/v1/wiki/entries/:id`
- `POST /api/v1/wiki/entries`
- `POST /api/v1/wiki/entries/:id/proposals`
- `POST /api/v1/quiz/attempts`
- `GET /api/v1/me/quiz-attempts`
- `GET /api/v1/me/wrong-questions`
- `DELETE /api/v1/me/wrong-questions/:id`
- `GET /api/v1/me/weakness-report`
- `GET /api/v1/me/downloads`
- `GET /api/v1/me/entitlements`
- `POST /api/v1/ai/tasks`
- `GET /api/v1/ai/tasks/:id`
- `POST /api/v1/admin/schools`
- `PATCH /api/v1/admin/schools/:id`
- `DELETE /api/v1/admin/schools/:id`
- `POST /api/v1/admin/colleges`
- `PATCH /api/v1/admin/colleges/:id`
- `DELETE /api/v1/admin/colleges/:id`
- `POST /api/v1/admin/majors`
- `PATCH /api/v1/admin/majors/:id`
- `DELETE /api/v1/admin/majors/:id`
- `GET /api/v1/admin/courses?schoolId=&majorId=&grade=&status=`
- `POST /api/v1/admin/courses`
- `PATCH /api/v1/admin/courses/:id`
- `DELETE /api/v1/admin/courses/:id`
- `GET /api/v1/admin/materials?courseId=&status=`
- `POST /api/v1/admin/materials`
- `PATCH /api/v1/admin/materials/:id/status`
- `PATCH /api/v1/admin/materials/:id`
- `DELETE /api/v1/admin/materials/:id`
- `POST /api/v1/admin/materials/upload`
- `GET /api/v1/admin/material-reviews?status=pending|published|rejected&courseId=`
- `POST /api/v1/admin/materials/:id/approve`
- `POST /api/v1/admin/materials/:id/reject`
- `GET /api/v1/admin/downloads?materialId=&userId=`
- `GET /api/v1/admin/ai/tasks`
- `GET /api/v1/admin/ai/drafts`
- `POST /api/v1/admin/ai/drafts/:id/approve`
- `POST /api/v1/admin/ai/drafts/:id/reject`
- `GET /api/v1/admin/blog/posts?status=draft|pending|needs_changes|published|rejected&authorId=&limit=`
- `POST /api/v1/admin/blog/posts/:id/approve`
- `POST /api/v1/admin/blog/posts/:id/reject`
- `GET /api/v1/admin/forum/posts?status=draft|pending|needs_changes|published|rejected&authorId=&boardId=&limit=`
- `POST /api/v1/admin/forum/posts/:id/approve`
- `POST /api/v1/admin/forum/posts/:id/reject`
- `GET /api/v1/admin/forum/replies?status=draft|pending|needs_changes|published|rejected&authorId=&postId=&limit=`
- `POST /api/v1/admin/forum/replies/:id/approve`
- `POST /api/v1/admin/forum/replies/:id/reject`
- `GET /api/v1/admin/wiki/entries?status=draft|pending|needs_changes|published|rejected&authorId=&courseId=&limit=`
- `POST /api/v1/admin/wiki/entries/:id/approve`
- `POST /api/v1/admin/wiki/entries/:id/reject`
- `GET /api/v1/admin/wiki/proposals?status=draft|pending|needs_changes|published|rejected&entryId=&editorId=&limit=`
- `POST /api/v1/admin/wiki/proposals/:id/approve`
- `POST /api/v1/admin/wiki/proposals/:id/reject`
- `GET /api/v1/admin/analytics/overview`
- `GET /api/v1/admin/operation-logs?operatorId=&action=&targetType=&targetId=&createdFrom=&createdTo=&limit=`
- `GET /api/v1/admin/operation-logs/export?operatorId=&action=&targetType=&targetId=&createdFrom=&createdTo=&limit=`
- `GET /api/v1/admin/operation-logs/retention`

Response envelope:

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

Error envelope:

```json
{
  "code": 40001,
  "message": "unauthorized",
  "details": {}
}
```

Later stages add points, membership, richer wiki conflict resolution, forum reward posts, notification, report, and expanded admin APIs.

Implemented authentication behavior:

- Verification codes are hashed before storage.
- Development and test can use `DEV_FIXED_VERIFICATION_CODE=123456`.
- Production does not return codes in API responses.
- JWT uses RS256. Production must provide a private key through environment or a mounted file.
- `access_token` and `refresh_token` are issued as httpOnly cookies; the access token is also returned for API clients.
- logged-in users can update their own display name, school, major, and grade through `PATCH /auth/me`
- profile binding validates that the selected school and major exist and that the major belongs to the selected school

Implemented material behavior:

- material detail and list responses do not expose `storage_key`
- `free` materials can be downloaded without login
- `login_required` materials require an authenticated, email-verified user
- `paid` materials require an authenticated, email-verified user and either a valid material grant or a valid package grant containing that material
- successful downloads create `material_download_logs` records with material, optional user, access level, IP, user agent, and download time
- denied downloads, unsafe storage keys, and missing files are not recorded as successful downloads
- PDF downloads generate a temporary watermarked copy and return `X-Watermark-Applied: true`; the original stored file is not modified
- non-PDF downloads return the original file with `X-Watermark-Applied: false`
- if PDF watermark generation fails, the API returns `watermark_failed` instead of silently serving an unwatermarked PDF
- logged-in users can list only their own successful downloads through `/me/downloads`
- logged-in users can list only their own active material/package entitlements through `/me/entitlements`
- admin users can list successful download audit logs, including IP and User-Agent metadata
- unsafe or missing storage keys return `file_not_found` without revealing local paths

Implemented package behavior:

- package list/detail endpoints only return `published` course packages
- package detail returns package items plus published materials included in the package
- a `material_access_grants.package_id` grant unlocks paid package materials on the server side
- expired package grants do not unlock paid materials
- `/me/entitlements` returns direct material grants, published package grants, included materials, and summary counts for the current user only
- expired grants and grants for unpublished packages are excluded from `/me/entitlements`

Implemented quiz behavior:

- question list/detail responses do not expose answers
- submit returns correctness, score, and explanation
- unauthenticated users can submit practice answers, but wrong questions are not persisted
- logged-in users can create quiz attempts and list only their own attempts
- authenticated wrong answers create or update user-scoped wrong-question records
- weak-point reporting currently returns per-course wrong-count totals

Implemented blog behavior:

- public blog list/detail endpoints only return `published` posts
- logged-in, non-frozen users can submit blog posts; submissions always enter `pending`
- blog submission validates required title, lowercase URL slug, and content length
- reviewer/admin users can list pending/published/rejected blog posts through `/admin/blog/posts`
- approving a blog post sets `status=published` and records `reviewerId`, `reviewedAt`, and optional `reviewReason`
- rejecting a blog post sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public endpoints
- blog review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected posts return HTTP 409 with `post_not_reviewable`

Implemented forum behavior:

- public forum board list returns only `published` boards
- public forum post list/detail endpoints only return `published` posts with `visibility=public` under a published board
- public forum post responses use a public DTO and do not expose `reviewerId` or `reviewReason`
- logged-in, non-frozen users can submit forum posts; submissions always enter `pending`
- forum submission validates required board, title, content, and post type
- supported post types in the MVP are `normal` and `question`; reward posts remain later work
- logged-in, non-frozen users can submit replies to published public forum posts; replies always enter `pending`
- reviewer/admin users can list draft/pending/needs_changes/published/rejected forum posts through `/admin/forum/posts`
- reviewer/admin users can list draft/pending/needs_changes/published/rejected forum replies through `/admin/forum/replies`
- approving a forum post sets `status=published` and records `reviewerId`, `reviewedAt`, and optional `reviewReason`
- rejecting a forum post sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public endpoints
- forum post review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected posts return HTTP 409 with `forum_post_not_reviewable`
- approving a forum reply sets `status=published`, records reviewer metadata, increments the parent post `commentCount` once, and makes it visible through public post detail
- rejecting a forum reply sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public post detail
- forum reply review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected replies return HTTP 409 with `forum_reply_not_reviewable`
- reward settlement, best-answer flow, reply editing, and point grants remain later work

Implemented wiki behavior:

- public wiki list/detail endpoints only return `published` and `visibility=public` entries
- public wiki endpoints use a public DTO and do not expose `reviewerId` or `reviewReason`
- wiki entries bound to an unpublished course are hidden from public wiki endpoints
- creator/admin users can submit wiki entries; submissions always enter `pending`
- wiki submission validates required title, lowercase URL slug, content length, and optional published course binding
- the initial wiki submission creates a `wiki_edit_histories` version-1 row in the same transaction
- creator/admin users can submit edit proposals for already-published public wiki entries through `POST /api/v1/wiki/entries/:id/proposals`
- edit proposals capture the published entry `baseVersion` and keep public content unchanged until reviewer approval
- reviewer/admin users can list draft/pending/needs_changes/published/rejected wiki entries through `/admin/wiki/entries`
- reviewer/admin users can list draft/pending/needs_changes/published/rejected wiki edit proposals through `/admin/wiki/proposals`; list rows include base-version history content, current live entry content/version/status, and an `isStale` conflict flag
- approving a wiki entry sets `status=published` and records `reviewerId`, `reviewedAt`, and optional `reviewReason`
- rejecting a wiki entry sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public endpoints
- wiki review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected entries return HTTP 409 with `entry_not_reviewable`
- approving a wiki edit proposal requires the live entry version to still match `baseVersion`, updates the public entry, increments `version`, writes a new `wiki_edit_histories` row, and records operation logs in the same transaction
- stale wiki edit proposals return HTTP 409 with `proposal_stale`; the proposal remains pending and the public entry is not changed
- rejecting a wiki edit proposal requires `reviewReason`, records reviewer metadata, and does not change the public entry
- wiki edit proposal review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected proposals return HTTP 409 with `proposal_not_reviewable`

Implemented admin behavior:

- all admin endpoints require an authenticated `admin` or `super_admin` role
- review endpoints under `/api/v1/admin/ai/*`, `/api/v1/admin/material-reviews`, `/api/v1/admin/materials/:id/approve|reject`, `/api/v1/admin/wiki/entries*`, `/api/v1/admin/wiki/proposals*`, `/api/v1/admin/blog/posts*`, `/api/v1/admin/forum/posts*`, and `/api/v1/admin/forum/replies*` allow `reviewer`, `admin`, or `super_admin`
- `reviewer` users remain blocked from material CRUD, course CRUD, download audit, analytics, operation logs, and other admin-only APIs
- organization/course/material delete operations archive by setting `status=archived`
- admin course list returns all course statuses; public course list/detail returns only `published`
- course create/update accepts only `draft`, `published`, or `archived`
- admin material list returns all material statuses; public material list/detail returns only `published`
- material create/upload defaults to `draft` when status is omitted
- material status updates accept only `draft`, `pending`, `published`, `rejected`, or `archived`
- material review approve/reject is only allowed from `pending`; already published/rejected/archived materials return HTTP 409 with `material_not_reviewable`
- approving a material sets `status=published` and records `reviewerId`, `reviewedAt`, and optional `reviewReason`
- rejecting a material sets `status=rejected`, requires `reviewReason`, and records `reviewerId` and `reviewedAt`
- material create/update/upload accept only known material types and access levels
- material metadata update rejects `storageKey`, `storage_key`, `fileName`, `file_name`, `fileSize`, and `file_size`; file replacement must use upload flow
- material upload uses server-generated storage keys under `materials/{courseId}/`
- upload accepts only `.pdf`, `.txt`, `.md`, and `.docx`; PDFs must start with a PDF header
- upload rejects files larger than 20 MiB
- manually supplied `storageKey` values with path traversal are rejected
- admin analytics overview returns read-only totals, 14-day successful-download trend, top materials, course demand, and access-level breakdown
- admin operation logs support filtering by operator, action, target, created time range, and limit
- operation log CSV export reuses the same filters, requires admin role, and caps output by `OPERATION_LOG_EXPORT_LIMIT`
- operation log retention policy is exposed read-only through `/admin/operation-logs/retention`; automatic deletion is not enabled in the MVP

Implemented AI behavior:

- logged-in users can create AI tasks and query only their own tasks
- supported task types are `chat`, `wrong_question_analysis`, `targeted_question`, `paper_generation`, and `draft_review`
- reviewer, admin, and super_admin users can list AI tasks and AI drafts
- Redis Stream enqueue is best-effort; database task creation remains the source of truth
- worker mock mode turns pending tasks into pending AI drafts
- approving a draft accepts optional `reviewReason`, marks the draft reviewed, and does not publish generated content automatically
- rejecting a draft requires `reviewReason`, marks the draft rejected, and does not delete the source task or generated content
- AI draft review is only allowed from `draft`, `pending`, or `needs_changes`; already reviewed drafts return HTTP 409 with `draft_not_reviewable`
- the Vue admin `/ai/drafts` page is a UI wrapper over these reviewer-capable AI review endpoints
