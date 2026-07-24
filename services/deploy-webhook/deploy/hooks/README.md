# Deployment hook contract

The root-owned directory configured by `HENUKIT_DEPLOY_HOOK_DIR` contains the
server-specific release hooks. Files are executed in lexical order and receive
one phase argument:

- `prepare`: build, lint, preflight, backup, and stage without changing live traffic.
- `activate`: atomically switch or restart the configured deploy unit.
- `verify`: run readiness and public smoke checks against the live unit.
- `rollback`: restore the previous application release after activation or verification failure.

The runner exports only validated event metadata and release paths:

- `HENUKIT_RELEASE_SHA`
- `HENUKIT_RELEASE_DIR`
- `HENUKIT_PREVIOUS_RELEASE_SHA`
- `HENUKIT_PREVIOUS_RELEASE_DIR`
- `HENUKIT_DELIVERY_ID`
- `HENUKIT_REPOSITORY`
- `HENUKIT_RELEASE_REF`

Hooks must be owned by root, must not be writable by the deploy user, and must
never read secrets from the checked-out release. Secrets and service topology
belong under `/etc/henukit-deploy` or the service-specific root-owned files.
