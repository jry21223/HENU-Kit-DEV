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

HC-04 provides the responsive six-module Overview presentation states. HC-09 integrates the same-origin Console Gateway Session endpoint. HC-10 replaces metric fixtures with the Gateway's six-module aggregation response. HC-12 adds a responsive Platform Operations workspace. HC-13 adds Notice review and distribution. HC-14 adds the responsive Library workspace at desktop and 390px, limited to courses, materials, downloads, submission review, and corrections. HC-15 adds responsive Food submission, anomaly-ticket, and tier-adjustment operations. HC-357 changes membership operations to a server-bounded user page: operators search by Display Name or email, select exactly one user identified visibly by Display Name and email, then use the existing grant/revoke workflow; UUIDs remain transport identifiers rather than primary UI labels. Owner writes carry generated idempotency keys, surface optimistic conflicts, and resolve unknown outcomes before resubmission; the browser receives no exchange token, owner-service credential, or product database access.
