---
status: accepted
amends: 0032, 0033
---

# Food MCP reads optional Kit identity from the current request

The user requested automatic, optional account identity instead of asking
people to remember a UUID. This amends ADR-0033 for independent Food MCP.
ADR-0046's separate QQ review queue remains in force.

## Decision

- Remove account UUID/name from creation and UUID from mine tool arguments.
  Venue, campus, tier and review remain required. Models cannot select an actor.
- Read the configured Portal Session cookie from each current MCP request and
  ask Gateway's existing `GET /api/v1/session` for UID, display-name snapshot
  and expiry. The host supplies transport metadata, never tool arguments.
  Only that cookie is forwarded; redirects are refused and lookup is bounded
  to three seconds. MCP credentials and unrelated cookies are omitted.
- Missing, expired or unusable sessions, unavailable Gateway, or installations
  without account integration use a system-generated guest UUID and `游客`.
  Successful results include a binding prompt. Lookup failures instead explain
  that the account could not be read. Submission remains available.
- Keep one guest identity per authenticated SDK-managed MCP session. Food
  retains signing, validation, idempotency and its daily cap. Guest cap and
  mine history apply within that session; reconnecting starts a new identity.
  Later account association affects future calls and does not reassign posts.
- Inherit existing Portal Session semantics: Gateway validates encrypted
  cookie authenticity and expiry. UID/name are login-time snapshots; the
  route does not freshly check Core revocation or suspension. MCP claims no
  stronger check than website Food publication. Missing legacy names use
  the label `Kit 用户`.
- MCP Bearer authentication remains required. Guests are behind the trusted
  client boundary; this is not an unauthenticated public write endpoint.

## Consequences

Browser clients on the Kit origin can carry their Portal cookie. Desktop and
server clients do not inherit browser login: their host must supply the user's
existing Session or use the guest path. This change adds no new binding flow
and shares no Portal encryption key or Core credential with MCP.

`FOOD_MCP_KIT_SESSION_URL` optionally points to the fixed Gateway session route;
empty disables lookup. Compose defaults to its internal URL.
`FOOD_MCP_KIT_SESSION_COOKIE_NAME` defaults to the production secure cookie;
local HTTP may select Gateway's local cookie. Internal secure-profile lookup
requires Gateway to trust the MCP container network, already set in Compose.

SDK/HTTP boundary tests cover automatic account preference, optional lookup
failure, schema, guest stability and isolation, signing, validation and caps.
