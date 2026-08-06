---
status: accepted
amends: 0013
---

# Library becomes an independent data-owner service wired into Console

`services/library` implements the Console materials operation surface
(`library.read` / `library.manage` / `library.review`) as its own Go service
with its own Postgres database and Redis-backed HMAC nonce store. It wraps the
retired Study admin API over HTTP for legacy data, and it must be deployable
as a fixed-SHA release unit and reachable from Console Gateway through the same
credential pairing Notice and Food use.

## Decision

- Console Gateway gains a `library` owner client only when
  `LIBRARY_API_URL` is non-empty; an empty value keeps the Console
  "资料库运营" module on its existing 503 degradation, exactly like
  `NOTICE_API_URL` and `FOOD_API_URL`. The deployment enables the module by
  setting `LIBRARY_API_URL=http://library:8095` in the production
  environment file, never by changing the default compose wiring.
- `services/library` fails closed at startup when any of
  `LIBRARY_DATABASE_URL`, `LIBRARY_REDIS_URL`,
  `LIBRARY_SERVICE_CLIENT_ID` / `_KEY_ID` / `_SECRET`, or
  `STUDY_LEGACY_API_URL` / `STUDY_LEGACY_ADMIN_TOKEN` is missing. A missing
  legacy credential is therefore a startup failure with a clear log line, not
  a silent partial workspace.
- The service credentials are the same `LIBRARY_SUMMARY_*` pair Console
  Gateway already presents, mirroring the `NOTICE_SUMMARY_*` / `FOOD_SUMMARY_*`
  reuse; no new secret family is introduced. `STUDY_LEGACY_ADMIN_TOKEN` is a
  separate long-lived Bearer token for the retired Study admin API and is
  provisioned by ops, not derived from the summary pair.
- The release unit follows the Notice/Food onboarding: a `henukit-library`
  fixed-SHA image in the build matrix, `migrations/library` packaged into the
  runtime tarball, `apply_owner_migrations library` in the artifact helper, a
  distroless healthcheck binary probed by Compose, and the release-contract
  test asserting all thirteen images.
- Library owns its own database and ledger tables
  (`library_adapter_operations`, `library_adapter_audit_events`); it never
  reads the retired Study database directly.

## Consequences

- The Console module degrades to 503 until ops provisions `LIBRARY_API_URL`
  plus the legacy Study admin credentials; clearing `LIBRARY_API_URL` and
  restarting Console Gateway reverts the module to that degradation.
- Legacy Study admin API failures surface as `partial` / `unavailable`
  workspace states with honest error fields, never fabricated empty data.
- A missing or wrong legacy credential stops the Library service at startup
  (fail closed); it does not put the module into a broken 502 path, because
  the gateway only creates the client after the operator explicitly enabled
  `LIBRARY_API_URL`.
- The static legacy token is an adapter compatibility choice permitted for
  retired interfaces; a future replacement data owner should use the HMAC
  scheme of the API communication spec.
