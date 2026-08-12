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

HC-09 establishes the login, callback, logout, and server-verified Console Access Context flow. HC-10 and HC-11 add the six-owner Overview aggregation and first real Portal owner endpoint. HC-12 adds thin signed forwarding for Platform Operations reads, idempotent Session/access writes, and unknown-result lookup. HC-13 adds the same thin boundary for Notice. HC-14 adds signed forwarding to the bounded Library Compatibility Adapter. HC-15 adds the equivalent thin Food boundary: Gateway verifies the exact `food.*` permission against product Scope `food`, signs actor context, and owns no Food state or database credential. HC-168 adds the Account Portfolio support-ticket operations workspace. Ticket responses are augmented through one bounded Platform Core identity batch authorized by the exact `account.tickets.read`, `account.tickets.reply`, or `account.tickets.transition` permission already required by the operation; the Gateway owns no identity copy. HC-170 adds bounded membership lookup, grant, and revocation: the Gateway verifies only `account.membership.write` against product Scope `account-portfolio`, forwards its Console Session actor with a separate actor-bound service credential, and never lets the browser select an operator identity or receive target/operator audit fields. HC-169 adds the matching point-adjustment command boundary: Gateway verifies exactly `account.points.adjust` against product Scope `account-portfolio`, preserves the browser's raw idempotent command, and returns only the owner-confirmed balance and public ledger entry. Browser responses never contain the exchange token, service credentials, or point-audit identities.

HC-270 wires the Portal, Notice, Library, and Food owner summaries into the production Overview. Plain HTTP is accepted only for loopback test/local endpoints or the exact owner-specific Compose host (`portal` → `portal-summary`, `notice` → `notice`, `library` → `library`, `food` → `food`); cross-owner hosts are rejected before any credential is attached or sent. Platform and QuizCraft remain closed as `not_onboarded`. An explicitly empty endpoint for an onboarded owner is a distinct operator decision reported as typed `operator_disabled`; runtime owner failures remain ordinary unavailable/stale behavior and are never presented as zero data.
