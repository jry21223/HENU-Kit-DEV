---
status: accepted
amends: 0017, 0018
---

# Food owns Food Post creation and public reads

Students could not add a spot to the food ranking inside HENU Kit: the
`/food/publish` submit button linked out to a third-party sandbox domain with
no account, data-owner, or review connection, and neither `services/food`'s
legacy `food_submissions` model nor `services/portal-api`'s `portal_food_posts`
table had any write path. This decision graduates a new "Food Post" capability
into `services/food`, following the same owner-graduation pattern Library used
in ADR-0027 to move its OSS-active catalog off Portal API.

## Decision

- `services/food` owns Food Post creation and public reads. Portal Gateway
  adds its third exception to the default read-only proxy (alongside ADR-0017
  and ADR-0018): a signed-in-only `POST /api/v1/food/posts` create command plus
  shadowed `GET /api/v1/food/posts`, `GET /api/v1/food/posts/{id}`,
  `GET /api/v1/food/posts/{id}/images/{position}`, and
  `GET /api/v1/food/venues` routes that call Food instead of Portal API.
- A **Food Post** is a public student recommendation created by a signed-in
  actor. Required fields are venue name, campus (`minglun`/`jinming`/
  `longzihu`), one of five self-selected tiers (夯/顶级/人上人/NPC/拉完了), and
  free-text review. Optional fields are price reference, hours reference, up
  to six dishes (name/price/reason), and up to six photos (≤2 MiB each, stored
  as Postgres bytes — no object storage). No coordinates are collected; a post
  renders with the existing `(0, 0)` map fallback.
- A post is **public the instant it is created** (`hidden = false` always).
  There is no draft, queue, or approval state for this flow, and Portal copy
  must never describe one.
- Gateway requires a valid Portal Session for the create command and the
  actor-scoped `GET /api/v1/food/posts/mine` read; there is no guest
  downgrade. Gateway binds the session's UserID and Display Name snapshot into
  the signed command — the browser cannot supply actor headers. Food snapshots
  that display name onto the post at creation and never re-resolves it.
- Food enforces server-side, before any write, an actor-scoped idempotency key
  and a hard cap of three posts per submitter per calendar day
  (Asia/Shanghai). Replaying the same key returns the original result; the
  same key with a different body conflicts; a fourth same-day submission is
  rejected with `429 DAILY_POST_CAP_REACHED` and writes nothing.
- The create command and the read routes use dedicated credentials that are
  independent of each other and of Console Gateway's existing `food.*`
  permission pair; the three identities are not interchangeable. The five-line
  canonical signature format is unchanged; actor headers are trusted
  signed-request headers, not an extra canonical line.
- `services/portal-api`'s `portal_food_posts` table is frozen legacy data for
  this feature: it is not written, not read, and not merged into the `/food`
  listing by this change. The external Food Desk hand-off
  (`NEXT_PUBLIC_FOOD_DESK_URL`) and every related configuration reference are
  removed with no fallback.

## Consequences

- A logged-in student can publish a recommendation without leaving the site,
  and the account console lists only that student's own posts.
- Portal and Portal API stop being the shape authority for this slice; only
  the exact Gateway-to-Food seam can create a Food Post.
- Portal Gateway now has three documented exceptions to its read-only default
  (ADR-0017, ADR-0018, and this decision); every other proxy path is unchanged.
- Post-publish moderation (hiding, tier disputes) remains deliberately absent:
  Food's existing Anomaly Ticket and Tier Adjustment concepts are not wired to
  Food Posts, and unifying the seven legacy rows with new posts is a separate,
  undecided follow-up.
