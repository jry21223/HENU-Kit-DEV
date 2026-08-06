---
status: accepted
amends: 0018
---

# Portal Gateway forwards the signed-in user's own ranking profile write

ADR-0018 permits exactly two default-off QuizCraft Practice commands and
explicitly lists "rankings" among the product commands that need their own
decision before Portal can write them. This ADR is that decision for the one
ranking command: updating the signed-in user's own public ranking identity
(nickname, system avatar, and ranking visibility).

Portal Gateway may forward an authenticated Portal Session user's ranking
profile write to QuizCraft Core's `PATCH /api/v1/ranking-profile` under the
same constraints ADR-0018 established for Practice commands: the Gateway stays
stateless and thin, never accepts a browser-selected actor or service
credential, binds the actor from the verified Portal Session, signs the
actor-bound request with the dedicated practice command credential pair,
requires a command idempotency key, and fails closed on any dependency
failure.

## Decision

- The browser-visible route is only `PATCH /api/v1/ranking-profile`, the same
  path Core publishes in `packages/api-contracts/openapi/quizcraft.yaml`
  (operationId `updateRankingProfile`). The route is registered only when the
  V2 read gate is enabled, and it is a pure relay: Gateway validates the
  envelope shape and forwards the exact accepted body to Core.
- The write is actor-bound. The actor comes exclusively from a valid Portal
  Session; a request without a Portal Session is a `401`, never a guest
  downgrade, and an invalid session is also a `401`. The actor UUID is bound
  to the practice command HMAC (`SignWithActor`), so Core can reject requests
  that claim an actor the Gateway did not verify.
- The write is idempotent. An `Idempotency-Key` header is required and
  validated with the same rule as the Practice commands; Core owns the
  per-user replay history, and a repeated logical write is answered with the
  already-persisted `OperationEnvelope` (or a `409` service-replay response
  where the contract says so).
- The boundary fails closed. When practice commands are not enabled
  (`PORTAL_PRACTICE_COMMANDS_ENABLED` off, `practiceCommands` client not
  created), the route answers `503` instead of forwarding. The write uses the
  same dedicated practice command credential pair as ADR-0018's commands,
  never the catalog/read credentials.
- Response safety matches ADR-0018: Gateway relays Core's accepted
  `OperationEnvelope` unchanged and invents no browser-side facts. The
  nickname rules themselves (Han ranges, reserved words, length) stay owned
  by Core; the Portal settings form mirrors them only for immediate feedback.
- Routing choice: unlike the internal `/api/v1/portal/practice/*` commands,
  this operation is a public user-session route in the contract, so the
  curated `cmd/quizcraftcontractgen` does not emit its path constant yet. The
  path is declared alongside the generated contract in
  `internal/practice/contract_not_yet_generated.go` with a cutover note to
  fold it into the generator once the generator grows a user-route write
  mode. This keeps the ranking profile write inside the same ADR-0009
  contract-first lineage without weakening the generator's strict validation
  of the internal commands it does emit.

## Consequences

- The ADR-0018 practice command exception now covers sessions, answers,
  feedback corrections, and the user's own ranking profile write. Favorites,
  learning summaries, Hero mastery, payment, and every other product command
  still need their own explicit decision before Portal can write them.
- The write cannot be enabled by a Portal UI change alone: the route 503s
  until the practice command client and its independent secret pair are
  deliberately provisioned during the approved cutover window.
- Nickname/avatar/visibility normalization stays in Core; Portal only relays.
  A future read endpoint for the current profile (Core currently exposes
  none) would be a separate decision.
- Until `updateRankingProfile` is folded into `cmd/quizcraftcontractgen`, the
  hand-written path constant is a small, explicitly marked ADR-0009
  exception; any contract rename must update that constant in the same
  change.
