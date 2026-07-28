---
status: accepted
amends: 0013
---

# Portal Gateway forwards bounded Account Portfolio self-service commands

ADR-0013's read-only proxy rule remains the default for Portal Gateway. Account
Portfolio support-ticket and notification commands are the narrow exception:
the Gateway may forward an authenticated Portal Session user's create,
follow-up, detail, and mark-read requests to that owner because the user must
be able to create durable account facts. The Gateway remains stateless and
thin, never accepts a browser-selected actor or service credential, signs the
actor-bound request, requires command idempotency, and fails closed.

## Consequences

- The exception is limited to a user's own Account Portfolio support tickets
  and notifications; it does not permit Portal writes for points, memberships,
  membership orders, or any other product owner.
- Console operator commands stay on the separately authenticated Console
  Gateway path and require exact Account Portfolio product permissions.
- A future product command does not inherit this exception; it needs its own
  explicit architecture decision.
