# Console Gateway Context

## Owns

- The Console-local authorization-code callback and Console Session cookie.
- One-time OAuth state, browser-binding nonce, PKCE verifier, and same-origin return path coordination across a short-lived host-only cookie and Redis.
- Server-to-server validation of Console permission codes and Scope through Platform Core.

## Does not own

- Platform users, Core Sessions, Authorization Codes, permission grants, or product data.
- Business migrations, product database credentials, or Study Legacy API behavior.
- Browser authorization decisions based on roles, `isAdmin`, or hidden controls.

## Current boundary

HC-09 establishes the login, callback, logout, and server-verified Console Access Context flow. HC-10 and HC-11 add the six-owner Overview aggregation and first real Portal owner endpoint. HC-12 adds thin signed forwarding for Platform Operations reads, idempotent Session/access writes, and unknown-result lookup. Every operation uses the server-held exchange token and is authorized again by Platform Core against permission code plus platform Scope; browser responses never contain that token or Platform Core credentials.
