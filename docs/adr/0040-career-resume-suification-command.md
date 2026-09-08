---
status: accepted
amends: 0013, 0017
---

# Portal Gateway forwards one transient Career Resume Suification command

Portal Gateway's read-only default gains one exact self-service exception:
`POST /api/v1/career/profile/suifications`. A signed-in Lifetime user's Portal
Session supplies the actor; the browser supplies only the current Resume Text
and a required idempotency key. Gateway uses its existing Career owner
credential, signs the actor-bound request, and never exposes provider settings
or service credentials to the browser.

Career sends the Resume Text to its operator-configured Career LLM and returns
an entertainment-only Suification Draft. It does not update the durable Career
Profile. The browser may discard the draft or apply it to the editable form;
the ordinary explicit profile `PUT` remains the only save.

## Idempotency and failure semantics

- Career binds each idempotency key to the authenticated actor and a SHA-256
  hash of the exact Resume Text. The same key and body replay the same draft
  without a second provider call or rate-limit charge; a different body
  conflicts, and an active same-body request asks the caller to retry.
- Redis holds only the request hash, lock metadata, and returned draft for at
  most 10 minutes. It is coordination and safe replay state, not a Career
  Profile write. Redis, provider, authentication, or configuration failure
  fails closed; there is no mock or legacy fallback.
- The existing operator-approved plaintext exception for Resume Extraction
  does not automatically authorize Suification. Public plaintext Suification
  remains unavailable unless the separate
  `CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP=1` gate is explicitly enabled for the
  exact approved endpoint. HTTPS and loopback providers need no exception.

## Consequences

- No other Career or product command inherits this exception.
- Applying a draft changes only browser state and remains recoverable until the
  user edits again or saves; an in-flight result is discarded if the source
  Resume Text is replaced by editing or a new extraction.
- The provider instruction treats Resume Text as untrusted data and prohibits
  invented employers, roles, projects, technologies, awards, or metrics; the
  user-visible surface still labels the result as entertainment and requires
  factual review.
