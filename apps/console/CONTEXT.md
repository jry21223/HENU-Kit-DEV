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

HC-04 provides the responsive six-module Overview presentation states. HC-09 integrates the same-origin Console Gateway Session endpoint. HC-10 replaces metric fixtures with the Gateway's six-module aggregation response: signed-out and denied states expose no metrics; authenticated cards display only bounded owner summaries with honest partial, stale, unavailable, observation-time, last-success, and request-tracing metadata. Product operation routes remain planned.
