# API Migrations

New production schema changes use versioned `*.up.sql` and `*.down.sql` migrations. Runtime AutoMigrate remains a development-only compatibility path and must be disabled in production.

- `0001_v2_schema.sql`: original schema anchor.
- `0002_admin_dashboard_v1.up.sql`: unified admin domain tables and indexes.
- `0002_admin_dashboard_v1.down.sql`: isolated rollback for the admin V1 additions.
- `0003_admin_domain_workflows.up.sql`: notice review audit fields used by the admin workflow.
- `0003_admin_domain_workflows.down.sql`: rollback for the notice review fields.
- `0004_mail_worker.up.sql`: mail payload, claim/retry state, attempts, and suppression tables.
- `0004_mail_worker.down.sql`: rollback for the mail worker additions.
