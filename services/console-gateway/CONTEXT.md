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

HC-09 establishes the login, callback, logout, and server-verified Console Access Context flow. HC-10 adds concurrent reads of exactly six configured module-summary endpoints with a two-second module limit, three-second overall deadline, one idempotent retry, and a five-minute Redis stale fallback. The browser receives no exchange token or sensitive detail; product operation forwarding remains planned.
