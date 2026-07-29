---
status: accepted
amends: 0013
---

# Portal Gateway forwards exactly two default-off QuizCraft Practice commands

ADR-0013 keeps Portal Gateway read-only by default, and ADR-0017 already
defines a bounded Account Portfolio self-service exception. QuizCraft needs a
separate exception because a browser must create a real server-selected
Practice Session and submit an answer without receiving a question answer key.
This ADR permits only those two commands; it does not create a generic product
write proxy or move QuizCraft business facts into Portal Gateway.

## Decision

- The browser-visible routes are only `POST /api/v1/practice/sessions` and
  `POST /api/v1/practice/sessions/{session_id}/answers`. Gateway maps them to
  QuizCraft's private `/api/v1/portal/practice/*` routes; browsers never call
  QuizCraft directly.
- Gateway obtains a signed-in actor exclusively from a valid Portal Session.
  It binds that UUID to the six-part service HMAC. A request without a Portal
  Session is a guest request: it has no actor header and uses the five-part
  HMAC. An invalid Portal Session is a `401`, never a guest downgrade.
- Guest identity is only QuizCraft's `quizcraft_anonymous` HttpOnly cookie.
  Gateway forwards and reissues only that cookie after requiring `Path=/`,
  `Secure`, `HttpOnly`, `SameSite=Lax`, and no `Domain`; it never sends the
  Portal cookie or arbitrary browser cookies to QuizCraft.
- Practice command credentials are distinct from the Portal catalog/read
  credentials. QuizCraft verifies Basic credentials, exact raw-body HMAC,
  timestamp, nonce replay protection, and the optional signed actor before
  adding the actor to private request context.
- `PORTAL_PRACTICE_COMMANDS_ENABLED`,
  `QUIZCRAFT_PORTAL_COMMANDS_ENABLED`, and `QUIZCRAFT_WRITES_ENABLED` remain
  off by default. Ticket #161 prepares the dark path only. Ticket #166 is the
  sole cutover window that may enable all three after migration evidence and
  browser acceptance are complete.
- Portal renders question content only from the created session and renders
  correctness, expected answer, and analysis only from the answer response.
  A dependency failure, invalid response, empty selection, or disabled gate
  is an honest loading/empty/error state, never a mock successful session.

## Consequences

- Gateway remains stateless and does not own bank selection, answer keys,
  scoring, attempts, learning state, or guest identities.
- The two paths require an idempotency key. The client preserves that key for
  a logical retry; QuizCraft owns the persisted replay outcome and concurrent
  attempt behavior.
- Favorites, rankings, feedback, learning summaries, Hero mastery, payment,
  and every other product command remain outside this exception and need their
  own explicit decision before Portal can write them.
- A deployment cannot enable this boundary merely by changing the Portal UI:
  both service gates and the independent secret pair must be deliberately
  provisioned together during the approved cutover.
