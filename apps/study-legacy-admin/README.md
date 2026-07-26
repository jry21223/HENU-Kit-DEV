# Study Legacy Admin — RETIRED

**RETIRED from HENU Kit production deploy.** This package is not the operator UI and must not be the default admin entry.

- Production admin surface: `apps/console` (HENUKit Console) only.
- This tree is retained solely for emergency break-glass / rollback during migration.
- Do not add new cross-product navigation, permissions, or deploy paths here.
- Root `pnpm run build` no longer builds this package; use `pnpm run build:study-legacy-admin` or `pnpm run build:all` explicitly when needed.
