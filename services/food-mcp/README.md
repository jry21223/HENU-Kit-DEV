# Food Post MCP

Streamable HTTP tools for Food-owned public posts. Every request requires the
configured MCP Bearer token.

`create_food_post` requires only `venue_name`, `campus`, `tier` and
`review_text`. Prices, hours, dishes and images remain optional. Creation and
`list_my_food_posts` accept no account UUID or display-name argument.

## Optional Kit account

The host associates the current user's existing Kit Portal Session by carrying
its cookie on every MCP HTTP request. Gateway's `GET /api/v1/session` resolves
the canonical UID and display-name snapshot. Only the selected cookie is
forwarded; MCP credentials, unrelated cookies and caller actor headers are
not forwarded. Legacy actor tool arguments are rejected.

Browser clients on the Kit origin can use their existing cookie. Desktop and
server MCP clients do not automatically inherit browser login; their host
must supply the Session to associate an account. Never ask a person to paste
a cookie, UUID or password into chat or tool arguments.

| Configuration | Default / behavior |
| --- | --- |
| `FOOD_MCP_KIT_SESSION_URL` | Compose: `http://portal-gateway:8084/api/v1/session`; empty disables lookup |
| `FOOD_MCP_KIT_SESSION_COOKIE_NAME` | `__Host-henukit_portal_session`; local HTTP must match Gateway's `PORTAL_LOCAL_SESSION_COOKIE_NAME` |

Lookup refuses redirects and has a three-second timeout. Gateway validates
cookie authenticity and expiry; UID/name are login-time snapshots, and the
existing route does not refresh Core revocation/suspension. Secure-cookie
forwarding over internal HTTP sets `X-Forwarded-Proto: https`; Gateway must
trust the MCP container's network, already configured in Compose.

Missing, expired or unavailable accounts do not block content submission.
The system creates a guest identity labelled `游客`, stable for that MCP
session, and includes a binding or reconnect prompt. Food's daily cap and
mine read use this identity. Reconnecting starts a new guest identity; earlier
posts remain public but are not attached to a later account association.
Food errors still fail the tool; binding prompts do not substitute for Food
success. The separate QQ review queue is not served by these tools (ADR-0046).

See ADR-0047 for the optional-identity amendment to ADR-0033.

## Validation

```sh
go test ./... -count=1 -timeout=60s
go vet ./...
```
