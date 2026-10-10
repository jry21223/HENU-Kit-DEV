---
status: accepted
amends: 0032, 0033
---

# Food MCP reads optional Kit identity from the current request

The user requested automatic, optional account identity instead of asking
people to remember a UUID. This amends ADR-0033 for independent Food MCP.
The user explicitly also requires unbound QQ users to continue submitting.
This narrowly amends ADR-0046/#523's bound-only rule for the already-enabled
Food MCP channel, preserving its existing immediate publication behavior.
The separate proposed pending-review queue is not implemented by this change.

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
- In standalone mode, keep one guest identity per authenticated SDK-managed MCP session. Food
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

## Shared Bot integration amendment

QQ pipelines must use the plugin-owned `henu_food` tool instead of the shared
independent Food connector. The plugin obtains trusted SDK Bot+sender context,
resolves the existing signed Core binding endpoint afresh, and signs per-call
MCP metadata with a dedicated secret. Valid bound accounts are preferred;
missing/unavailable binding uses a stable opaque guest scope derived from
Bot+sender without disclosing QQ identity to Food. Unbinding affects future
calls. There is no automatic reassignment of older guest posts.

MCP verifies the HMAC, tool and canonical argument digest, timestamp, kind,
UUID/name and nonce before invoking Food. In shared-bot production it requires
this context for create/mine. Invalid context never silently falls back.
The nonce is reused as Food's durable idempotency key: proof replay repeats
the original operation, while changed arguments are rejected. All five Food
capabilities remain available through the plugin tool. Other MCPs and campus
CLI behavior remain unchanged.
