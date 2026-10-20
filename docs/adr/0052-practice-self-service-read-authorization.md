---
status: accepted
amends: 0013, 0021, 0036
---

# Practice statistics and favorites reads authenticate the Portal Session actor

The Portal lets a signed-in learner submit answers and favorite/unfavorite
questions through the bounded, actor-signed owner commands in ADR-0018 and
ADR-0021. Reading the same learner's statistics and favorite references must
use that self-service identity boundary. These reads do not administer a bank
or read another learner's data.

The Gateway previously sent `portal.practice.read` to Platform Core's
`/api/v1/authorization/check` without its required structured Scope. Platform
Core correctly rejected that malformed request, making ordinary authenticated
statistics and favorites reads return an authorization-unavailable error before
reaching QuizCraft. Adding `scope: {kind: product, product_code: quizcraft}` alone
would not establish an end-user contract: the repository does not register
`portal.practice.read` as a user role permission or grant it to learners. It is
the existing service read permission asserted on the Gateway-to-QuizCraft
request, distinct from the registered `quizcraft.workshop.*` operator permissions.

## Decision

- Exactly these browser reads authenticate the existing Portal Session without
  a separate Platform Core RBAC check:
  - `GET /api/v1/practice/stats`
  - `GET /api/v1/practice/favorites`
  - `GET /api/v1/practice/banks/{bank_id}/favorites`
- The Gateway requires a valid AES-GCM Portal Session with an unexpired cookie
  payload and an unpadded, non-nil UUID actor. Missing, tampered, expired, or
  malformed sessions return `401` before any owner request. Browser headers,
  query parameters, and the anonymous Practice cookie cannot select the actor
  or downgrade an invalid signed-in request to a guest.
- The existing read enablement gate and private response headers remain.
  Gateway signs the verified Session actor into the sixth HMAC canonical line
  with the independent QuizCraft read credential. QuizCraft still checks that
  service signature, timestamp, nonce, `portal.practice.read`, product Scope,
  and actor before reading that actor's own records.
- Owner credential rejection, unavailability, or invalid responses fail closed;
  they do not become a fake empty collection, zero statistics, or a Portal API
  fallback. Internal owner authentication failures remain a service error,
  rather than asking a correctly authenticated browser to log in again.
- Favorites writes and replay behavior remain ADR-0021's existing self-service
  commands. Generic Platform Core authorization, operator reads/writes, and
  the feedback-status permission check are unchanged. No role grant, migration,
  runtime flag, or credential change implements this correction.

## Consequences

The browser sees its own persisted learning data under the same Portal Session
identity policy as the existing self-service commands. Platform Core still owns
login and the Session exchange; QuizCraft still owns learning data, per-actor
queries, and write/replay facts. This decision does not add live Core Session
introspection or a new revocation-propagation guarantee. It relies on the
existing encrypted Portal Session lifetime, as do the existing commands.

The regression boundary is the normal browser Gateway read: authenticated
self-service success without an operator permission request, Session rejection
before owner access, actor isolation with signed owner requests, and honest
owner failure. Production acceptance additionally reads real statistics and
favorites, adds a favorite through the normal UI, reloads it, then removes and
reads it back.
