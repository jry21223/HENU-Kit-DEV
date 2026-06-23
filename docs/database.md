# V2 Database

The V2 database is created from Go/GORM models in `services/api/internal/platform/model`. The archived Prisma schema is reference-only.

Current model coverage:

- users and email verification codes
- schools, colleges, majors, courses
- materials, course packages, package items, material access grants, and download logs
- orders and payment records
- quiz questions, attempts, answers, wrong questions, weakness reports
- wiki entries, edit histories, edit proposals, creator applications, blog, forum, moments, relations
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

- `materials.storage_key` is internal-only and is not serialized in public API responses.
- `materials.reviewer_id`, `materials.reviewed_at`, and `materials.review_reason` record the current material-review decision; public material endpoints still expose only `published` rows.
- `material_access_grants.material_id` is the active paid-material unlock path in the current foundation.
- `material_access_grants.package_id` unlocks paid materials included through `course_package_items` when the package is published and the grant has not expired.
- `material_download_logs` records only successful file downloads after permission checks and storage-key validation.
- PDF watermarking is generated at response time from the stored source file; no separate watermarked file record is persisted.
- `wrong_questions` uses a unique user/question pair so repeated wrong answers increment `wrong_count` instead of creating duplicate rows.
- `wiki_edit_proposals` stores proposed edits to published wiki entries with `base_version`, review fields, and proposed title/content; approval updates the live entry and writes `wiki_edit_histories` only when the live entry version still matches.
- `forum_posts.reward_points` and `forum_posts.reward_status` track reward-post escrow and settlement state; reward submission, rejection, and best-answer settlement are mirrored by idempotent `points_logs` rows.
- `forum_replies.is_best` marks the selected best answer; application logic allows only one best answer per published post.
