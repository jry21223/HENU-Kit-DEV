---
status: accepted
---

# Bind authentication cookies to trusted HTTPS origins

HENU Kit authentication cookies carry Session authority and must not depend on untrusted forwarding headers or ambiguous development defaults. Production services therefore distinguish a verified HTTPS request from local HTTP before selecting cookie names and attributes.

## Considered options

- Always mark cookies Secure and use one `__Host-` name in every environment. Rejected because local HTTP cannot accept the cookie and developers are tempted to weaken browser security checks.
- Trust `X-Forwarded-Proto` from every peer. Rejected because a direct client could claim HTTPS and influence security-sensitive request handling.
- Use separate production and local cookie names, and honor forwarded HTTPS only from configured trusted proxies. Accepted because production stays fail closed while local HTTP remains explicit.

## Consequences

- Production authentication cookies use the `__Host-` prefix with `HttpOnly`, `Secure`, `Path=/`, no `Domain`, and `SameSite=Lax`.
- A request is considered externally HTTPS only when its direct connection is TLS or its remote address belongs to a configured trusted-proxy CIDR that supplied the forwarded HTTPS scheme.
- Forwarding headers from untrusted peers do not enable production cookie behavior.
- Local HTTP uses an explicitly configured non-`__Host-` cookie name and never silently reuses a production Session cookie.
- Logout and every route that clears or rotates a Session cookie apply the same environment and proxy decision as Session issuance.
