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

HC-04 provides the responsive six-module Overview presentation states. HC-09 integrates the same-origin Console Gateway Session endpoint. HC-10 replaces metric fixtures with the Gateway's six-module aggregation response. HC-12 adds a responsive Platform Operations workspace. HC-13 adds the responsive Notice review and distribution workspace at desktop and 390px. Notice writes carry generated idempotency keys, surface optimistic conflicts, and resolve unknown outcomes before resubmission; the browser receives no exchange token or Notice service credential.
