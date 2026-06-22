# V2 Database

The V2 database is created from Go/GORM models in `services/api/internal/platform/model`. The archived Prisma schema is reference-only.

Current model coverage:

- users and email verification codes
- schools, colleges, majors, courses
- materials, course packages, package items, and material access grants
- orders and payment records
- quiz questions, attempts, answers, wrong questions, weakness reports
- wiki, blog, forum, moments, relations
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
- `material_access_grants.material_id` is the active paid-material unlock path in the current foundation.
- `material_access_grants.package_id` unlocks paid materials included through `course_package_items` when the package is published and the grant has not expired.
- `wrong_questions` uses a unique user/question pair so repeated wrong answers increment `wrong_count` instead of creating duplicate rows.
