# V2 Database

The V2 database is created from Go/GORM models in `services/api/internal/platform/model`. The archived Prisma schema is reference-only.

Current model coverage:

- users and email verification codes
- schools, colleges, majors, courses
- materials, course packages, package items, material access grants, and download logs
- orders, payment records, and payment incidents
- quiz questions, attempts, answers, wrong questions, weakness reports
- wiki entries, edit histories, edit proposals, creator applications, blog, forum, moments, media assets, relations
- points, memberships, AI tasks, notifications, reports, operation logs
- system configs and leaderboard snapshots

Common fields:

- `id`
- `created_at`
- `updated_at`
- optional `deleted_at`

Reviewable content includes reviewer and review result fields.

`services/api/migrations/0001_v2_schema.sql` records the bootstrap SQL prerequisite. During this greenfield stage, GORM AutoMigrate is enabled by default through `AUTO_MIGRATE=true`.

Current access-control notes:

- `users.role` drives role middleware and currently accepts `user`, `creator`, `reviewer`, `operator`, `admin`, and `super_admin`.
- `users.status` drives account availability and currently accepts `active` or `frozen`; `users.frozen_until` can also block writes while it is in the future.
- Admin user management updates only `users.name`, `users.role`, and `users.status` in the MVP and records those mutations in `operation_logs`.
- `materials.storage_key` is internal-only and is not serialized in public API responses.
- `materials.reviewer_id`, `materials.reviewed_at`, and `materials.review_reason` record the current material-review decision; public material endpoints still expose only `published` rows.
- `material_access_grants.material_id` is the active paid-material unlock path in the current foundation.
- `material_access_grants.package_id` unlocks paid materials included through `course_package_items` when the package is published and the grant has not expired.
- `course_packages.status` controls public package visibility. Only `published` packages should be returned by public package list/detail APIs; admin package APIs may list `draft`, `published`, and `archived`.
- `course_packages.price_fen` stores price as an integer number of cents. Application code should avoid floating-point yuan values for order or package pricing.
- `orders.expires_at` stores the current WeChat Native QR/order expiry boundary. The API marks stale pending/paying WeChat orders as `expired` before reuse, status, payment-create, and admin-order reads.
- `payment_records.idempotency_key` keeps trusted payment callback processing idempotent; the same transaction id cannot be reused to pay a different order.
- `payment_incidents` stores rejected/quarantined WeChat callback anomalies such as unknown orders, amount mismatches, and transaction-id conflicts. Its `idempotency_key` deduplicates repeated identical anomaly callbacks.
- Resolving or ignoring a `payment_incidents` row is an operations note only. It does not mutate `orders.status`, insert a trusted `payment_records` success, or create `material_access_grants`.
- `course_package_items` is the package-to-resource binding table. The current product scope should use `resource_type=material`; future resource types need explicit service-layer allowlists before becoming public.
- Package-item rows may reference unpublished materials for admin staging, but public package detail must filter both the returned material list and the returned item list to published materials only. Returning a raw item id for a draft/pending/rejected/archived material is treated as metadata leakage.
- Active duplicate package-item bindings for the same package/resource pair should be prevented by service logic and, where possible, a uniqueness constraint. Removing a package item should remove the relation only and must not delete the material row.
- Admin-created manual grants use `material_access_grants.source=manual_admin`; duplicate active grants for the same user/resource are not recreated.
- `/me/entitlements` expands only published direct-granted materials and published package grants, so draft/pending/archived materials are not exposed through the personal entitlement summary.
- `material_download_logs` records only successful file downloads after permission checks and storage-key validation.
- PDF watermarking is generated at response time from the stored source file; no separate watermarked file record is persisted.
- `wrong_questions` uses a unique user/question pair so repeated wrong answers increment `wrong_count` instead of creating duplicate rows.
- `wiki_edit_proposals` stores proposed edits to published wiki entries with `base_version`, review fields, and proposed title/content; approval updates the live entry and writes `wiki_edit_histories` only when the live entry version still matches.
- `forum_posts.reward_points` and `forum_posts.reward_status` track reward-post escrow and settlement state; reward submission, rejection, and best-answer settlement are mirrored by idempotent `points_logs` rows.
- `membership_plans.points_cost` and `membership_plans.duration_days` define whether a published plan is redeemable with points. Plans with zero values are visible but cannot be redeemed through `/membership/redeem`.
- Points-based membership redemption deducts `users.points_balance`, creates a `points_logs` row with `reason=membership_redeem`, and creates or extends a `memberships.source=points_redeem` row in one transaction. The points-log idempotency key is built from the user id and client `requestId`.
- AI task quota deductions use `points_logs.reason=ai_task_usage` with `reference_type=ai_task`; `ai_usage_logs.source` distinguishes `points`, `membership_tier1`, `membership_tier1_discount`, `membership_tier2`, and `role_exempt`, while `ai_usage_logs.points_cost` records the charged points.
- `forum_replies.is_best` marks the selected best answer; application logic allows only one best answer per published post.
- `media_assets` stores uploaded dynamic images with owner, usage, storage key, file metadata, status, and optional `moment_id`. Moment image URLs expose only `/api/v1/moments/images/{mediaId}`; raw storage keys are not serialized.
- Unattached `media_assets` are owner-only previews. After a moment is created, the API binds uploaded assets to the moment and image reads reuse the same public/mutual-friends/block visibility checks as the moment itself.
- Admin cleanup archives stale unattached `moment_image` rows by setting `status=archived` after local file removal or missing-file confirmation. Attached rows with `moment_id` are excluded from cleanup, and `storage_key` remains internal-only.
