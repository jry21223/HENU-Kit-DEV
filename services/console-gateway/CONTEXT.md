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

HC-09 establishes the login, callback, logout, and server-verified Console Access Context flow. HC-10 and HC-11 add the six-owner Overview aggregation and first real Portal owner endpoint. HC-12 adds thin signed forwarding for Platform Operations reads, idempotent Session/access writes, and unknown-result lookup. HC-13 adds the same thin boundary for Notice. HC-14 adds signed forwarding to the bounded Library Compatibility Adapter. HC-15 adds the equivalent thin Food boundary: Gateway verifies the exact `food.*` permission against product Scope `food`, signs actor context, and owns no Food state or database credential. Browser responses never contain the exchange token or service credentials.
