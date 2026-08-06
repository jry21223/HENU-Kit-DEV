---
status: accepted
amends: 0018
---

# Portal Gateway forwards three default-off favorites commands

ADR-0018 kept Portal Gateway's QuizCraft Practice write exception to exactly
two commands and listed favorites among the product commands that "need their
own explicit decision before Portal can write them". This ADR is that
decision: a signed-in Portal user may favorite and unfavorite questions in
their own bank and start a practice session from their own available
favorites.

## Decision

- The browser-visible routes are `PUT /api/v1/practice/banks/{bank_id}/favorites/{question_id}`,
  `DELETE /api/v1/practice/banks/{bank_id}/favorites/{question_id}`, and
  `POST /api/v1/practice/banks/{bank_id}/favorites/practice-sessions`. Gateway
  maps them to QuizCraft's private `/api/v1/portal/practice/*` favorites
  routes; browsers never call QuizCraft directly, and favorites reads stay
  actor-bound on the same private path.
- Favorites writes are signed-in-only: the actor comes exclusively from a
  valid Portal Session, an invalid or missing session is a `401`, and a
  request without a session is never downgraded to a guest actor. Guest
  favorites do not exist.
- Every favorites write requires an `Idempotency-Key` header and a logical
  retry replays the same key; QuizCraft owns the persisted replay outcome.
- The commands fail closed: when the practice command client is not
  configured, all three routes answer `503`, and private response headers
  (`Cache-Control: no-store`, `X-Content-Type-Options: nosniff`) are set on
  every response.
- A favorites practice session is created on the folder page and travels to
  the quiz page through the existing sessionStorage handoff
  (`henukit.practice.session.v1:{session_id}`), because no endpoint re-reads
  a session by id; the quiz page still renders only question content the
  session owns.
- Enabling this surface still requires the same deliberate provisioning as
  ADR-0018 (the service gates plus the independent secret pair); the Portal
  UI cannot turn it on by itself.

## Consequences

- The exception now covers favorite/unfavorite writes and favorites-session
  creation for the signed-in actor. Rankings, feedback, learning summaries,
  Hero mastery, payment, and every other product command still need their own
  explicit decision before Portal can write them.
- A browser never sees another actor's favorites: reads and writes are bound
  to the verified Portal Session actor, and Core rejects a mismatched actor
  signature.
- Favorites remain reference-only in the Portal: question content is rendered
  only from a created practice session, and unavailable favorites keep their
  relationship but never enter a favorites practice session.
- This ADR supplements ADR-0018: it replaces 0018's blanket exclusion of
  favorites with this bounded decision while leaving 0018's two-command
  boundary and its default-off provisioning otherwise intact.
