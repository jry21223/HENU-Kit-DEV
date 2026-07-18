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

HC-04 provides the responsive six-module Mock Overview and its loading, empty, partial, stale, unavailable, and denied presentation states. The cards use fixture data only: Console Gateway integration, authentication, authoritative authorization decisions, routes beyond Overview, and production operations remain planned.
