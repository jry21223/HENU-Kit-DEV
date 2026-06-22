# V2 Database

The V2 database will be created from new Go/GORM migrations. The archived Prisma schema is reference-only.

Stage 2 will add core tables for:

- users and email verification codes
- schools, colleges, majors, courses
- materials and material access grants
- orders and payment records
- quiz questions, attempts, answers, wrong questions, weakness reports
- wiki, blog, forum, moments, relations
- points, memberships, AI tasks, notifications, reports, operation logs

Common fields:

- `id`
- `created_at`
- `updated_at`
- optional `deleted_at`

Reviewable content includes reviewer and review result fields.
