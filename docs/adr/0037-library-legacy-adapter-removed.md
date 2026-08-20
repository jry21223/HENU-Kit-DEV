---
status: accepted
supersedes: 0020
---

# Library legacy Study API adapter removed

ADR-0020 onboarded the Library module as an independent data owner with a
bounded HTTP translation layer to the Study Legacy API (`services/api`) for
courses, materials, submissions, corrections, and downloads. The legacy
service has since been physically retired: the 2026-08-19 production audit
found no study containers, no `:8080` listener, and the `/study-api/healthz`
route returning 404; the server env pointed `STUDY_LEGACY_API_URL` at a dead
port (`127.0.0.1:1`). The adapter had no reachable upstream and could not
function. This ADR removes the adapter and its startup coupling.

## Decision

- The `legacyAdapter` (`services/library/adapter.go`), its legacy workspace
  and command execution, the legacy types, and the operation ledger machinery
  that recorded legacy write outcomes are removed from `services/library`.
- The `/api/v1/commands` and `/api/v1/operations/{operation}` routes remain
  registered but return an honest `503 LIBRARY_COMMANDS_UNAVAILABLE`; the
  workspace and console summary return a degraded (`status: partial`) payload
  with an explicit message that the catalog migration (T1) is pending.
- The startup fail-closed check on `STUDY_LEGACY_API_URL` /
  `STUDY_LEGACY_ADMIN_TOKEN` (`cmd/server/main.go`) is deleted; both variables
  are removed from compose, env examples, and the server env during the next
  deploy.
- `ADR-0020` is superseded by this decision. The catalog migration task
  `T1` in `docs/migrations/M2-DATA-MIGRATION-TASKS.md` will restore real
  courses/materials data into library-owned tables (`library_courses` /
  `library_materials`), after which Console library operations are served from
  library's own database. Until then the degraded workspace is the honest
  state.

## Consequences

- Library no longer depends on any legacy Study runtime; it starts with only
  its own database, Redis, OSS store, and service credentials.
- Console library operations views show the degraded/empty state until T1
  lands; this matches production reality where the legacy upstream was
  already unreachable.
- The `library_adapter_operations` / `library_adapter_audit_events` tables
  remain (append-only audit evidence); new commands will reuse or replace
  them per the T1 design.
- Removing the legacy route surface closes the last code-level dependency of
  the new stack on `services/api`, completing the retirement of the Study
  legacy runtime from the product path.
