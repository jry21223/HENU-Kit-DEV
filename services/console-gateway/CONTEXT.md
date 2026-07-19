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

HC-09 establishes the login, callback, logout, and server-verified Console Access Context flow. The browser receives only a host-only HttpOnly/Secure/SameSite=Lax encrypted Console Session cookie; the Platform Core exchange token is never returned to JavaScript. Overview aggregation and product operation forwarding remain planned.
