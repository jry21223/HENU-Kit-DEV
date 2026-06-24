# V2 API

Base path: `/api/v1`

Health semantics:

- `GET /healthz` and `GET /api/v1/healthz` are liveness endpoints. They return HTTP 200 with dependency status details even when a dependency is not ready.
- `GET /readyz` and `GET /api/v1/readyz` are readiness endpoints. They return HTTP 200 only when PostgreSQL and Redis are reachable; otherwise they return HTTP 503 with `not_ready`.

Currently implemented endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/healthz`
- `GET /api/v1/readyz`
- `GET /api/v1/version`
- `GET /api/v1/search?q=&limit=`
- `GET /api/v1/leaderboards/wiki?limit=`
- `GET /api/v1/leaderboards/quiz?limit=`
- `GET /api/v1/leaderboards/overall?limit=`
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
- `GET /api/v1/membership/plans`
- `POST /api/v1/membership/redeem`
- `POST /api/v1/orders`
- `GET /api/v1/orders/:id`
- `GET /api/v1/orders/:id/status`
- `POST /api/v1/payments/wechat/native`
- `POST /api/v1/payments/wechat/close`
- `POST /api/v1/payments/wechat/notify`
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
- `POST /api/v1/forum/replies/:id/mark-best`
- `GET /api/v1/moments?limit=`
- `POST /api/v1/moments`
- `POST /api/v1/moments/images`
- `DELETE /api/v1/moments/:id`
- `POST /api/v1/moments/:id/like`
- `POST /api/v1/moments/:id/comments`
- `DELETE /api/v1/moments/comments/:id`
- `GET /api/v1/wiki/entries?courseId=&limit=`
- `GET /api/v1/wiki/entries/:id`
- `GET /api/v1/wiki/creator-applications/me?limit=`
- `POST /api/v1/wiki/creator-applications`
- `POST /api/v1/wiki/entries`
- `POST /api/v1/wiki/entries/:id/proposals`
- `POST /api/v1/quiz/attempts`
- `GET /api/v1/me/quiz-attempts`
- `GET /api/v1/me/wrong-questions`
- `DELETE /api/v1/me/wrong-questions/:id`
- `GET /api/v1/me/weakness-report`
- `GET /api/v1/me/points`
- `GET /api/v1/me/points/logs?limit=`
- `GET /api/v1/me/membership`
- `GET /api/v1/me/downloads`
- `GET /api/v1/me/entitlements`
- `GET /api/v1/me/wiki-entries?limit=`
- `PATCH /api/v1/me/wiki-entries/:id`
- `GET /api/v1/me/wiki-proposals?limit=`
- `PATCH /api/v1/me/wiki-proposals/:id`
- `GET /api/v1/me/forum-posts?limit=`
- `PATCH /api/v1/me/forum-posts/:id`
- `GET /api/v1/me/forum-replies?limit=`
- `PATCH /api/v1/me/forum-replies/:id`
- `GET /api/v1/me/notifications?limit=&unread=true`
- `POST /api/v1/me/notifications/:id/read`
- `POST /api/v1/me/notifications/read-all`
- `GET /api/v1/me/following?limit=`
- `GET /api/v1/me/followers?limit=`
- `GET /api/v1/me/friends?limit=`
- `POST /api/v1/users/:id/follow`
- `POST /api/v1/users/:id/unfollow`
- `POST /api/v1/users/:id/block`
- `POST /api/v1/users/:id/unblock`
- `GET /api/v1/users/:id`
- `POST /api/v1/ai/tasks`
- `GET /api/v1/ai/tasks/:id`
- `POST /api/v1/reports`
- `GET /api/v1/admin/users?email=&role=&status=&limit=`
- `PATCH /api/v1/admin/users/:id`
- `GET /api/v1/admin/media-assets?usage=&status=&ownerEmail=&momentId=&limit=`
- `POST /api/v1/admin/media-assets/cleanup`
- `GET /api/v1/admin/points/logs?userId=&reason=&limit=`
- `GET /api/v1/admin/points/rules`
- `POST /api/v1/admin/points/rules`
- `PATCH /api/v1/admin/points/rules/:id`
- `GET /api/v1/admin/memberships?userId=&planCode=&status=&limit=`
- `POST /api/v1/admin/memberships/grant`
- `POST /api/v1/admin/memberships/:id/revoke`
- `GET /api/v1/admin/access-grants?userId=&materialId=&packageId=&source=&active=&limit=`
- `POST /api/v1/admin/access-grants`
- `DELETE /api/v1/admin/access-grants/:id`
- `GET /api/v1/admin/orders?status=&userEmail=&outTradeNo=&packageId=&paymentProvider=&productType=&riskFlag=&riskOnly=&limit=`
- `GET /api/v1/admin/payment-reconciliation?issueType=&severity=&limit=`
- `GET /api/v1/admin/payment-incidents?status=&incidentType=&orderId=&outTradeNo=&transactionId=&limit=`
- `POST /api/v1/admin/payment-incidents/:id/resolve`
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
- `GET /api/v1/admin/reports?status=pending|approved|rejected|all&targetType=&limit=`
- `POST /api/v1/admin/reports/:id/resolve`
- `POST /api/v1/admin/reports/:id/reject`
- `GET /api/v1/admin/wiki/entries?status=draft|pending|needs_changes|published|rejected&authorId=&courseId=&limit=`
- `POST /api/v1/admin/wiki/entries/:id/approve`
- `POST /api/v1/admin/wiki/entries/:id/reject`
- `GET /api/v1/admin/wiki/creator-applications?status=draft|pending|needs_changes|approved|published|rejected&userId=&limit=`
- `POST /api/v1/admin/wiki/creator-applications/:id/approve`
- `POST /api/v1/admin/wiki/creator-applications/:id/reject`
- `GET /api/v1/admin/wiki/proposals?status=draft|pending|needs_changes|published|rejected&entryId=&editorId=&limit=`
- `POST /api/v1/admin/wiki/proposals/:id/approve`
- `POST /api/v1/admin/wiki/proposals/:id/reject`
- `GET /api/v1/admin/analytics/overview`
- `GET /api/v1/admin/operation-logs?operatorId=&action=&targetType=&targetId=&createdFrom=&createdTo=&limit=`
- `GET /api/v1/admin/operation-logs/export?operatorId=&action=&targetType=&targetId=&createdFrom=&createdTo=&limit=`
- `GET /api/v1/admin/operation-logs/retention`

Search:

- `GET /api/v1/search?q=&limit=` performs a conservative public search across currently implemented content types.
- `q` is trimmed and limited to 80 characters.
- `limit` defaults to 8 and is capped at 30 per group.
- Results are grouped into `courses`, `materials`, `packages`, `wiki`, `blog`, and `forum`.
- Only public/published content is returned. Draft, pending, rejected, archived, and private content is excluded.
- Material search responses do not expose `storageKey`.

Leaderboards:

- `GET /api/v1/leaderboards/wiki?limit=` returns all-time public Wiki contribution rankings.
- `GET /api/v1/leaderboards/quiz?limit=` returns all-time quiz practice rankings based on submitted answer score and correctness aggregates.
- `GET /api/v1/leaderboards/overall?limit=` returns a conservative all-time score from points, published Wiki contributions, and correct quiz answers.
- `limit` defaults to 10 and is capped at 50.
- Responses include only `userId`, display `name`, `role`, `score`, rank, and aggregate metrics. They do not expose email, answers, review fields, or internal content.
- The first version uses live aggregates. Scheduled `leaderboard_snapshots`, weekly periods, and anti-gaming controls remain future hardening work.

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

Later stages add payment-backed membership purchase, richer wiki conflict resolution, expanded admin APIs, and more notification sources.

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
- frozen users cannot download `login_required`, `paid`, or `member_only` materials even when they already have a grant
- public material responses from `/materials`, `/materials/:id`, `/courses/:id/materials`, and `/packages/:id` use a redacted material DTO. They include public display and permission fields such as `id`, `courseId`, `title`, `type`, `description`, `fileName`, `fileSize`, `previewContent`, `accessLevel`, `status`, `createdAt`, and `updatedAt`, but must not expose `storageKey`, `createdBy`, `reviewerId`, `reviewedAt`, or `reviewReason`.
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
- public package detail must filter both `items` and `materials` to published material resources; package items that point at draft, pending, rejected, or archived materials must not appear in the public response, even as raw resource ids
- Web `/packages` lists published course packages; Web `/packages/[id]` uses public package detail plus the logged-in user's `/me/entitlements` to display locked/unlocked state, create/reuse pending package orders, request a WeChat Native code URL, render a local QR code, and poll read-only order status; it does not mark payment success or grant access from the frontend
- a `material_access_grants.package_id` grant unlocks paid package materials on the server side
- expired package grants do not unlock paid materials
- `/me/entitlements` returns direct material grants, published package grants, included materials, and summary counts for the current user only
- expired grants and grants for unpublished packages are excluded from `/me/entitlements`
- manifest import supports `-dry-run`, which executes the same validation/upsert/package-bind/report path in a rolled-back transaction and reports planned counts without writing rows
- manifest import responses include `report.filesChecked`, `report.totalFileBytes`, `report.accessLevels`, `report.statuses`, `report.types`, `report.paidMaterials`, `report.packageItemLinks`, per-package summaries, and `report.duplicateFiles` for preflight acceptance
- manifest-delivery smoke coverage imports temporary mounted files, verifies public package detail hides storage keys, checks free/login_required/paid download behavior, grants the imported package, and verifies paid download audit logging

Implemented admin package-management contract:

- `GET /api/v1/admin/packages?schoolId=&majorId=&courseId=&grade=&status=`
- `POST /api/v1/admin/packages`
- `PATCH /api/v1/admin/packages/:id`
- `DELETE /api/v1/admin/packages/:id`
- `GET /api/v1/admin/packages/:id/items`
- `POST /api/v1/admin/packages/:id/items`
- `DELETE /api/v1/admin/packages/:id/items/:itemId`

Expected boundaries:

- admin package endpoints require an authenticated, non-frozen `admin` or `super_admin` role
- package statuses are `draft`, `published`, and `archived`; public package APIs expose only `published`
- prices are stored and accepted as integer cents through `priceFen` / `price_fen`; clients must not send floating-point yuan values
- package create/update validates organization ids and optional course binding before publication
- package item binding initially supports `resourceType=material`
- binding a material to a package does not grant user access by itself; downloads still require direct material grant, package grant, or later paid entitlement
- duplicate active item bindings return the existing binding with `alreadyExists=true` instead of creating another row
- unbinding a package item removes only the package-item relation and must not delete or mutate the material
- course package create/update/archive and package-item bind/unbind mutations should write `operation_logs`

Implemented order foundation:

- `POST /api/v1/orders` currently supports `packageId` for published course packages
- the server reads price, currency, and title from `course_packages`; client-submitted amount/provider fields are ignored
- created orders use `productType=course_package`, `paymentProvider=wechat_native`, and `status=pending`
- repeat order creation for the same user/package reuses the latest unexpired pending/paying WeChat Native order instead of creating duplicate rows
- stale pending/paying WeChat Native orders with `expires_at <= now` are marked `expired` before status reads, duplicate-order reuse checks, Native payment creation, and admin order listing
- users who already have an active package grant receive `alreadyOwned=true`; no new order is created
- `GET /api/v1/orders/:id` and `GET /api/v1/orders/:id/status` are user-scoped; admins may inspect all orders
- order status is read-only and does not grant entitlement; paid access still requires a package/material grant created by a trusted server-side flow
- admins can inspect orders through `/admin/orders` with status, buyer email, package, provider, product type, and out-trade-number filters; this endpoint is read-only
- admin order inspection can filter `riskOnly=true` or partial `riskFlag`; this exposes existing payment risk markers for triage but does not auto-resolve payment exceptions
- `GET /api/v1/admin/payment-reconciliation` is admin-only and read-only. It cross-checks local orders, payment records, order-source grants, risk flags, and open payment incidents for anomalies such as paid orders missing payment records, paid orders missing entitlements, non-paid orders with order entitlements, duplicate transaction ids, mismatched record amounts, risk flags, and open incident rows.
- the reconciliation report returns `issues`, `total`, and a `summary` grouped by severity and issue type. It never marks orders paid, inserts trusted payment records, resolves incidents, or grants entitlement.
- WeChat callback anomalies are also captured in `payment_incidents` for manual triage; current incident types include `order_not_found`, `amount_mismatch`, and `transaction_conflict`
- `GET /api/v1/admin/payment-incidents` is admin-only, defaults to `status=open`, and can filter by incident type, order id, out-trade number, transaction id, and limit
- the payment incident list response includes `incidents` and `total`; Vue Admin uses `total` to surface open payment incident alerts on the dashboard
- when `PAYMENT_INCIDENT_WEBHOOK_URL` is configured, newly created incident rows post a best-effort `payment_incident.opened` JSON webhook with an optional `X-Final-Review-Signature` HMAC header; duplicate idempotent incidents do not re-alert
- `POST /api/v1/admin/payment-incidents/:id/resolve` marks an open incident as `resolved` or `ignored`, records the handling admin and note, and writes an operation log
- resolving or ignoring a payment incident is deliberately non-financial: it does not mark an order paid, does not create a payment record, and does not grant entitlement

Implemented WeChat Native payment boundary:

- `POST /api/v1/payments/wechat/native` accepts `orderId` and requires the logged-in, non-frozen order owner
- only `pending` or `paying` `wechat_native` orders with positive integer-cent amounts can create a Native payment request
- development/test `WECHAT_PAY_MODE=mock` returns a mock `weixin://wxpay/mock/...` `codeUrl`, stores transient Native metadata plus `orders.expires_at`, and moves the local order to `status=paying`
- development/test `POST /api/v1/payments/wechat/notify` accepts mock callback JSON signed with `X-WeChat-Mock-Signature = hmac_sha256(WECHAT_PAY_API_V3_KEY, raw_body)`; this is only a local test harness, not the real WeChat callback verifier
- mock notify rejects missing/invalid signatures, missing mock secret, unknown orders, and amount mismatches without granting entitlement
- a successful mock notify changes the matching order to `paid`, records a `payment_records` row, and creates one idempotent package grant with `source=order`
- live mode builds a signed WeChat Native `POST /v3/pay/transactions/native` request using the merchant private key, sends integer-cent server-side package pricing, requires a matching platform certificate/public key for response signature verification, and stores the returned `code_url` only after the signed response verifies
- `POST /api/v1/payments/wechat/close` accepts `orderId`, requires the logged-in non-frozen order owner or an admin/super_admin, closes only `pending`/`paying` WeChat Native orders, and updates local status to `closed`; closed orders are not reused by new package order creation
- expired WeChat Native orders cannot create another code URL or be closed through the user close endpoint; users should create a new order for a fresh QR code
- live close for `paying` orders calls WeChat Pay `POST /v3/pay/transactions/out-trade-no/{out_trade_no}/close` before changing the local status
- live `POST /api/v1/payments/wechat/notify` verifies the WeChat callback signature against the configured platform certificate/public key, decrypts the AES-256-GCM `resource`, checks appid/mchid, validates `out_trade_no` and integer-cent amount against the local order, records a payment record, and idempotently grants the package entitlement on `SUCCESS`
- production rejects `WECHAT_PAY_MODE=mock` with `wechat_mock_forbidden_in_production`
- `WECHAT_PAY_MODE=live` requires `WECHAT_PAY_API_BASE_URL`, appid, mchid, API v3 key, merchant serial number, merchant private key or key path, platform certs dir, and notify URL before use
- Native code URL creation never marks an order paid, never grants entitlement, and never changes paid material access; Web QR rendering is only a display layer over the server-returned `codeUrl`
- real merchant end-to-end payment and close-order verification, certificate rotation automation, refund handling, and operational alerting remain later work

Implemented quiz behavior:

- question list/detail responses do not expose answers
- submit returns correctness, score, and explanation
- unauthenticated users can submit practice answers, but wrong questions are not persisted
- logged-in users can create quiz attempts and list only their own attempts
- authenticated wrong answers create or update user-scoped wrong-question records
- weak-point reporting currently returns per-course wrong-count totals
- Web `/me/wrong-questions` consumes the user-scoped wrong-question and weakness endpoints, fetches public question DTOs without answers, and can delete only the current user's wrong-question rows

Implemented blog behavior:

- public blog list/detail endpoints only return `published` posts
- public blog endpoints use a public DTO and do not expose `reviewerId`, `reviewReason`, or internal `status`
- Web `/blog` and `/blog/[id]` consume those public endpoints as read-only student-facing pages; they do not submit posts or expose review metadata
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
- supported post types are `normal`, `question`, and `reward`
- reward posts require a positive `rewardPoints` value, freeze points on submission with a `forum_reward_escrow` points log, and remain hidden while pending review
- logged-in, non-frozen users can submit replies to published public forum posts; replies always enter `pending`
- logged-in users can list only their own forum posts through `/me/forum-posts`, including `status` and their own `reviewReason` for rejected/needs-changes content
- logged-in users can list only their own forum replies through `/me/forum-replies`, including parent post title/status, reply `status`, and their own `reviewReason`
- `/me/forum-posts` and `/me/forum-replies` do not expose other users' submissions, `reviewerId`, `reviewedAt`, or admin-only review metadata
- logged-in, non-frozen users can edit and resubmit only their own `draft`, `pending`, `needs_changes`, or `rejected` forum posts through `PATCH /me/forum-posts/:id`; published/archived posts return HTTP 409
- forum post resubmission updates title/content only, resets status to `pending`, clears old reviewer metadata and review reason, and keeps the post hidden from public endpoints until review
- rejected/refunded reward posts re-freeze the original `rewardPoints` on resubmission with a `forum_reward_reescrow` points log; insufficient points reject the resubmission and keep the post rejected/refunded
- logged-in, non-frozen users can edit and resubmit only their own `draft`, `pending`, `needs_changes`, or `rejected` replies through `PATCH /me/forum-replies/:id`; published replies return HTTP 409
- reply resubmission updates content only, requires the parent post to still be published/public, resets status to `pending`, clears old reviewer metadata and review reason, and remains hidden from public detail until review
- reviewer/admin users can list draft/pending/needs_changes/published/rejected forum posts through `/admin/forum/posts`
- reviewer/admin users can list draft/pending/needs_changes/published/rejected forum replies through `/admin/forum/replies`
- approving a forum post sets `status=published` and records `reviewerId`, `reviewedAt`, and optional `reviewReason`
- rejecting a forum post sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public endpoints
- forum post review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected posts return HTTP 409 with `forum_post_not_reviewable`
- approving a forum reply sets `status=published`, records reviewer metadata, increments the parent post `commentCount` once, and makes it visible through public post detail
- rejecting a forum reply sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public post detail
- forum reply review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected replies return HTTP 409 with `forum_reply_not_reviewable`
- rejecting a reward post refunds frozen points with a `forum_reward_refund` points log; approving a reward post keeps points escrowed until best-answer selection
- post authors, `admin`, and `super_admin` users can select one published reply as best answer through `POST /api/v1/forum/replies/:id/mark-best`
- best-answer selection rejects unauthenticated users, unrelated users, unpublished replies, owner self-answers, and repeat selections
- reward-post best-answer selection requires `rewardStatus=escrowed`, marks the reply `isBest=true`, sets the post `rewardStatus=settled`, grants points to the reply author, and writes a `forum_reward_settlement` points log
- normal/question best-answer selection marks the reply `isBest=true` without changing points
- reply editing and UI-level best-answer controls remain later work

Implemented moment and relation behavior:

- `GET /api/v1/moments` is public with optional auth; anonymous users see only `published` moments with `visibility=public`.
- authenticated users still cannot see moments from users who blocked them or whom they blocked.
- `visibility=mutual_friends` moments are visible only to the author and users where both sides follow each other.
- logged-in, non-frozen users can create moments with up to 500 characters, up to 9 uploaded image URLs, and `visibility=public|mutual_friends`.
- accepted image URLs must come from `POST /api/v1/moments/images`; arbitrary external URLs, another user's uploaded image URL, duplicate images, already-attached images, and traversal-style local paths are rejected.
- moment likes are idempotent per user/moment and increment `likeCount` only once.
- logged-in users can comment on visible moments; comment counts update on create and delete.
- comment deletion is allowed for the comment author, the moment author, admin, or super_admin.
- users can follow/unfollow/block/unblock other users; self-follow/self-block is rejected.
- blocking a user removes follow edges between the two users and prevents future follow until unblocked.
- `/me/following`, `/me/followers`, and `/me/friends` are user-scoped and hide blocked relationships.
- relation-list responses contain only public user summaries (`id`, `name`, `role`) and do not expose email addresses.
- `POST /api/v1/moments/images` accepts authenticated image uploads only. It creates a `media_assets` row, stores generated files under `LOCAL_UPLOAD_DIR/moments/{userId}/`, allows JPG/PNG/WEBP/GIF, caps each file at 5MB, checks image magic bytes, and returns `/api/v1/moments/images/{mediaId}`.
- `GET /api/v1/moments/images/:id` serves the image through the Go API. Unattached uploads are visible only to the owner; attached public-moment images are publicly readable; attached mutual-friends images reuse the moment visibility and block checks.
- `POST /api/v1/moments` accepts only existing uploaded image URLs owned by the current user, then binds each media asset to the created moment. Frontend-supplied storage keys are never accepted.
- Admin-only `GET /api/v1/admin/media-assets` lists moment media assets by usage, status, owner email, moment id, and limit. Responses include owner metadata and `hasFile`, but do not serialize raw storage keys.
- Admin-only `POST /api/v1/admin/media-assets/cleanup` defaults to dry-run mode. It can archive old unattached `moment_image` uploads and remove their local files after an explicit `{"dryRun": false}` request. It refuses unsafe storage keys, records a `media_asset.cleanup` operation log, and never touches attached moment images.
- Web `/moments` now exposes a basic feed/composer for public and mutual-friends moments with local image upload and preview. Vue Admin `/media-assets` exposes the asset audit and stale-unattached-upload cleanup workflow. Video media and cloud object storage remain future work.

Implemented public user profile behavior:

- `GET /api/v1/users/:id` is public with optional auth.
- responses never include user email, verification code data, review metadata, or hidden draft/pending/rejected content.
- anonymous users see active public profile metadata, published public moments, published public blog posts, published public forum posts, and published replies under published public forum posts.
- authenticated users can additionally see mutual-friends moments when both users follow each other.
- if either side has blocked the other, authenticated profile lookup returns `404 user_not_found`.
- relation state fields include `followingByMe`, `followsMe`, `mutualFriend`, `blockedByMe`, and `blockedMe`.

Implemented notification behavior:

- logged-in users can list only their own notifications through `/me/notifications`
- `?unread=true` filters the returned list to unread notifications while still returning total unread count for the current user
- logged-in users can mark their own notification as read through `/me/notifications/:id/read`; reading another user's notification returns HTTP 404
- logged-in users can mark all of their own unread notifications as read through `/me/notifications/read-all`
- forum post/reply approve or reject actions create a `forum_review` notification for the content author in the same transaction as the review update and operation log
- material, wiki entry/proposal, blog post, and AI draft approve/reject actions create a `content_review` notification for the original author/editor/task owner in the same transaction as the review update and operation log
- report resolve/reject actions create a `report_result` notification for the reporter in the same transaction as the report update and operation log
- review notifications include safe data fields (`resourceType`, `resourceId`, `status`) and do not expose reviewer ids
- report result notifications include safe data fields (`reportId`, `targetType`, `targetId`, `status`) and do not expose reviewer ids
- payment and membership notifications remain later work

Points and membership:

- `GET /me/points` returns only the authenticated user's current `pointsBalance`.
- `GET /me/points/logs` returns only the authenticated user's points ledger rows.
- Admin points log listing supports `userId` and `reason` filters; ordinary users cannot access it.
- Admin points rules can be created and updated; those operations write operation logs.
- Public membership plans return only `published` plans.
- `GET /me/membership` returns only active, non-expired memberships for the authenticated user and a best-effort `current` membership.
- `POST /membership/redeem` requires an authenticated non-frozen user, a published plan with positive `pointsCost` and `durationDays`, and a client `requestId`; it deducts points, creates a `membership_redeem` points log, grants or extends a `points_redeem` membership, and treats duplicate `requestId` values as idempotent.
- Admin membership grant requires an existing user and a published plan. Re-granting the same active manual membership updates it instead of creating unlimited duplicates.
- Admin membership revoke marks the membership `revoked`, expires it immediately, and writes an operation log.
- Payment-backed membership purchase, membership expiry notifications, real model-token billing, and richer AI quota packages remain later work.

Implemented report behavior:

- logged-in, non-frozen users can submit reports through `POST /api/v1/reports`
- Web report buttons are currently wired on material detail pages, wiki entries, blog posts, forum posts, and forum replies
- reportable targets are `material`, `wiki_entry`, `blog_post`, `forum_post`, `forum_reply`, and `user`
- content targets must be public/published where applicable; hidden draft/pending/rejected content returns HTTP 404
- duplicate pending reports from the same reporter for the same target return the existing pending report instead of creating another row
- reviewer/admin users can list reports and resolve/reject pending reports through `/admin/reports`
- rejecting a report requires `reviewReason`; already handled reports return HTTP 409 with `report_not_reviewable`
- report handling writes an operation log and notifies the reporter in the same database transaction

Implemented wiki behavior:

- public wiki list/detail endpoints only return `published` and `visibility=public` entries
- public wiki endpoints use a public DTO and do not expose `reviewerId` or `reviewReason`
- Web `/wiki` and `/wiki/[id]` consume those public endpoints as student-facing pages; public responses still hide review metadata
- Web `/wiki` includes the student creator-application panel; logged-in users can view their own application status through `GET /api/v1/wiki/creator-applications/me`, which returns a self DTO without reviewer ids
- Web `/wiki/new` provides a creator/admin-only entry submission form that calls `POST /api/v1/wiki/entries`; submitted entries stay pending until reviewer/admin approval
- logged-in users can list only their own Wiki entries through `/me/wiki-entries`, including `status` and their own `reviewReason` for rejected/needs-changes content
- logged-in users can list only their own Wiki edit proposals through `/me/wiki-proposals`, including the target entry title/status and their own `reviewReason`
- `/me/wiki-entries` and `/me/wiki-proposals` do not expose other users' submissions, `reviewerId`, or admin-only review metadata; Web `/me/wiki` consumes these endpoints
- non-frozen logged-in users can revise and resubmit only their own `draft`/`pending`/`needs_changes`/`rejected` Wiki entries through `PATCH /me/wiki-entries/:id`; published/archived entries return `409` and other users' entries return `404`
- non-frozen logged-in users can revise and resubmit only their own editable Wiki edit proposals through `PATCH /me/wiki-proposals/:id`; resubmission refreshes `baseVersion` from the current published entry and clears previous review metadata
- Web `/wiki/[id]` includes a creator/admin-only edit-proposal form that submits to `POST /api/v1/wiki/entries/:id/proposals`; normal users see the review boundary instead of the editable form
- logged-in active normal users can submit creator applications through `POST /api/v1/wiki/creator-applications`; users who are already creator/admin/super_admin are rejected with `already_creator`, and other elevated single-role users are not converted implicitly
- users can have only one pending/draft/needs_changes creator application at a time
- wiki entries bound to an unpublished course are hidden from public wiki endpoints
- creator/admin users can submit wiki entries; submissions always enter `pending`
- wiki submission validates required title, lowercase URL slug, content length, and optional published course binding
- the initial wiki submission creates a `wiki_edit_histories` version-1 row in the same transaction
- creator/admin users can submit edit proposals for already-published public wiki entries through `POST /api/v1/wiki/entries/:id/proposals`
- edit proposals capture the published entry `baseVersion` and keep public content unchanged until reviewer approval
- reviewer/admin users can list draft/pending/needs_changes/published/rejected wiki entries through `/admin/wiki/entries`
- reviewer/admin users can list and review creator applications through `/admin/wiki/creator-applications`; approval sets application status to `approved` and promotes a normal user to `creator`
- reviewer/admin users can list draft/pending/needs_changes/published/rejected wiki edit proposals through `/admin/wiki/proposals`; list rows include base-version history content, current live entry content/version/status, and an `isStale` conflict flag
- approving a wiki entry sets `status=published` and records `reviewerId`, `reviewedAt`, and optional `reviewReason`
- rejecting a wiki entry sets `status=rejected`, requires `reviewReason`, records reviewer metadata, and keeps it hidden from public endpoints
- wiki review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected entries return HTTP 409 with `entry_not_reviewable`
- approving a wiki edit proposal requires the live entry version to still match `baseVersion`, updates the public entry, increments `version`, writes a new `wiki_edit_histories` row, and records operation logs in the same transaction
- stale wiki edit proposals return HTTP 409 with `proposal_stale`; the proposal remains pending and the public entry is not changed
- rejecting a wiki edit proposal requires `reviewReason`, records reviewer metadata, and does not change the public entry
- wiki edit proposal review is only allowed from `draft`, `pending`, or `needs_changes`; already published/rejected proposals return HTTP 409 with `proposal_not_reviewable`

Implemented admin behavior:

- admin-only endpoints require an authenticated, non-frozen `admin` or `super_admin` role
- admin users can list users through `/admin/users`, filtered by email substring, role, status, and limit
- admin users can update another user's display name, role, and active/frozen status through `PATCH /admin/users/:id`
- valid user roles are `user`, `creator`, `reviewer`, `operator`, `admin`, and `super_admin`; valid user statuses are `active` and `frozen`
- admins cannot change their own role or status from this endpoint, which prevents accidental self-lockout
- only `super_admin` users can edit an existing `super_admin` account or grant the `super_admin` role
- setting a user back to `active` clears `frozenUntil`; frozen users remain blocked by `RequireNotFrozen` write endpoints
- user updates write `operation_logs` with previous/current role and status metadata
- admin users can list, manually create, and revoke access grants through `/admin/access-grants`
- manual access grants use source `manual_admin`; they do not create payment orders and do not mark any order as paid
- manual material grants are limited to `published` materials whose `accessLevel` is `paid` or `member_only`
- manual package grants are limited to `published` course packages
- duplicate active grants for the same user/resource return the existing grant with `alreadyGranted=true` instead of creating another row
- revoking a grant soft-deletes the grant, immediately removing it from `/me/entitlements` and paid download checks
- access grant create/revoke operations write `operation_logs`
- review endpoints under `/api/v1/admin/ai/*`, `/api/v1/admin/material-reviews`, `/api/v1/admin/materials/:id/approve|reject`, `/api/v1/admin/wiki/entries*`, `/api/v1/admin/wiki/proposals*`, `/api/v1/admin/blog/posts*`, `/api/v1/admin/forum/posts*`, and `/api/v1/admin/forum/replies*` allow `reviewer`, `admin`, or `super_admin`
- `reviewer` users remain blocked from user management, material CRUD, course CRUD, download audit, analytics, operation logs, and other admin-only APIs
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
- admin analytics overview returns read-only totals, 14-day successful-download trend, top materials, course demand, access-level breakdown, and report target/status breakdown
- admin operation logs support filtering by operator, action, target, created time range, and limit
- operation log CSV export reuses the same filters, requires admin role, and caps output by `OPERATION_LOG_EXPORT_LIMIT`
- operation log retention policy is exposed read-only through `/admin/operation-logs/retention`; automatic deletion is not enabled in the MVP

Implemented AI behavior:

- logged-in users can create AI tasks and query only their own tasks
- supported task types are `chat`, `wrong_question_analysis`, `targeted_question`, `paper_generation`, and `draft_review`
- creating an AI task requires a non-frozen user and passes through server-side quota accounting before the task is persisted
- free users pay points by task type; `tier1` makes wrong-question analysis free and discounts other AI task costs; `tier2` makes supported AI tasks free
- insufficient points returns `400 insufficient_ai_points` and rolls back task creation
- task creation writes `ai_usage_logs`; point-paid tasks also write `points_logs.reason=ai_task_usage`
- reviewer, admin, and super_admin users can list AI tasks and AI drafts
- Redis Stream enqueue is best-effort; database task creation remains the source of truth
- worker mock mode turns pending tasks into pending AI drafts
- approving a draft accepts optional `reviewReason`, marks the draft reviewed, and does not publish generated content automatically
- rejecting a draft requires `reviewReason`, marks the draft rejected, and does not delete the source task or generated content
- AI draft review is only allowed from `draft`, `pending`, or `needs_changes`; already reviewed drafts return HTTP 409 with `draft_not_reviewable`
- the Vue admin `/ai/drafts` page is a UI wrapper over these reviewer-capable AI review endpoints
