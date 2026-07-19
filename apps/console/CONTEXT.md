# HENUKit Console Context

## Owns

- The independently built HENUKit Console browser bundle.
- Console navigation and presentation state.
- Browser-visible representations of authorization and product-module summaries received from Console Gateway contracts.

## Does not own

- Study Legacy Admin routes, Element Plus registration, or Study business API types.
- Product data, cross-product authorization decisions, or direct database access.
- Console Gateway, Platform Core, or any product service implementation.

## Current boundary

HC-04 provides the responsive six-module Overview presentation states. HC-09 integrates the same-origin Console Gateway Session endpoint. HC-10 replaces metric fixtures with the Gateway's six-module aggregation response. HC-12 adds a responsive Platform Operations workspace for bounded account grants/Scope, Session revocation, mail status, Operations Inbox references, audit, and dependency health. Writes carry generated idempotency keys, surface conflicts, and resolve unknown outcomes before retry; no Session token, mail recipient, secret, or source-product content enters browser state.
