# Study Legacy Admin Context

## Status

**RETIRED from HENU Kit product entry and default deploy.** Production operator UI is `apps/console` only. Keep this package for emergency break-glass / rollback; do not restore it as the default admin surface.

## Owns

- The preserved Study administration routes and their existing Vue, Element Plus, Pinia, and Study API behavior.
- The legacy `admin` deployment service, port, public origin, and release artifact used only for emergency rollback during migration.

## Boundary

This application is not HENUKit Console. New cross-product navigation, product modules, permissions, and API contracts must not be added here. Retirement work may remove capability only after an explicit replacement gate and rollback window have passed.
