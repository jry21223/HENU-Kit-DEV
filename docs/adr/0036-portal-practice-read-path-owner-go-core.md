---
status: accepted
amends: 0013, 0018
---

# Portal Practice reads resolve through Gateway to the QuizCraft Go core

Portal Gateway proxies the Portal frontend's product reads by default
(ADR-0013) and forwards exactly two default-off QuizCraft Practice commands
(ADR-0018). Today the Portal `/practice` read path is a third, undeclared
route: `portal-api` reads the QuizCraft Go-contract tables directly
(`services/portal-api/internal/practice/db.go`) behind the Gateway's
`/api/v1/practice/*` wildcard. That direct read has two known defects — ranking
standings fall back to the user UUID as a display name when the nickname is
empty (`db.go:290-291`), and question answers are delivered with the question
payload (`db.go:148-189`) — and it keeps `PRACTICE_SERVICE_URL` overloaded
between "portal-api direct-read target" (default compose) and "QuizCraft Core
address" (prebuilt compose). Decision D1 (2026-08-17, user-confirmed) resolves
the read path to the Go core.

## Decision

- **Read path convergence.** All Portal `/practice` reads — catalog
  (`/api/v1/practice/catalog`), rankings (`/api/v1/rankings/*`), and
  actor-bound reads (stats, favorites, feedback status) — are routed by Portal
  Gateway as explicit routes to the QuizCraft Go core's Portal read contract.
  The `portal-api` direct database read path
  (`services/portal-api/internal/practice/db.go` and its five router entries)
  is a transition state only and is deleted after cutover.
- **Privacy contract is mandatory.** Public ranking responses contain only
  `rank / nickname / system_avatar / correct_answer_count`; user UUIDs never
  appear. Learners without a visible profile are absent from the ranking;
  empty nicknames render the neutral label `匿名学习者`. Question answers and
  analysis are delivered only from the session answer response (ADR-0018
  boundary). Until the direct read is removed, its settlement display must
  obey the same rule: no `user_id` fallback as nickname.
- **Single wiring semantics.** `PRACTICE_SERVICE_URL` is converged to the
  single meaning "QuizCraft Core service address" (commands + catalog reads);
  its default compose value pointing at `portal-api` is removed. A wiring
  matrix documents its relationship with `QUIZCRAFT_CORE_URL`,
  `PORTAL_ENABLE_QUIZCRAFT_CATALOG` / `PORTAL_ENABLE_QUIZCRAFT_V2_READS`, and
  the baked browser flags, and is a configuration gate for cutover.
- **Cutover gate.** The ADR-0018 cutover window (#166) is the sole window for
  switching the read path: the three server-side gates and the two baked
  browser flags are enabled together, aligned pairwise, in one release
  bundle. After the browser gate (desktop + 390px) passes for practice,
  favorites, feedback, and rankings, `portal-api`'s direct read is removed.
  Read failures are honest errors/503s; there is no mock or legacy fallback.
- **Credential boundary unchanged.** Catalog/read credentials and command/write
  credentials remain distinct and non-shareable. Gateway holds no product
  database connection (ADR-0013 unchanged). QuizCraft owns the
  Practice/Favorites/Ranking/Feedback/Workshop contracts (QuizCraft CONTEXT
  unchanged).

## Consequences

- The portal-api practice module, its `QUIZCRAFT_DATABASE_URL` connection, and
  its practice tests are deleted after cutover; the Gateway wildcard proxy for
  `/api/v1/practice/*` no longer routes to `portal-api`.
- Portal UI switches to the already-implemented V2 catalog/rankings/stats data
  sources; the legacy schools/banks → gateway cache → mock fallback chain is
  removed.
- One read implementation (Go core contract) replaces the parallel SQL reader;
  contract-drift tests are extended to the Gateway side (ranking responses
  contain no `user_id`).
- Any deployment that keeps the direct read alive after cutover violates this
  ADR and the privacy contract.

## Amended by ADR-0038

ADR-0038 replaces the Ranking Profile mechanism with platform identity and
amends §2 (privacy contract) of this ADR: the internal Core → Gateway ranking
contract may carry a nullable `user_id`, Portal Gateway derives
`nickname`/`system_avatar` itself and remains the single stripping point;
"learners without a visible profile are absent" and the `匿名学习者` label are
replaced by "guests and learners without a display name rank with the neutral
`游客x` label". The browser-facing response shape is unchanged.

