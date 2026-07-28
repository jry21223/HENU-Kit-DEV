# Account Portfolio Context

## Owns

- Persistent user points, an immutable point ledger, memberships, membership
  orders, account notifications, and support tickets.
- Its dedicated PostgreSQL database, additive migrations, and signed internal
  HTTP contract.
- The initial durable account state: zero points, free membership, and empty
  notification/ticket collections on the first authenticated owner read.

## Does not own

- Platform users, Display Names, Core Sessions, OAuth, or permissions.
- Browser sessions, Portal presentation, or browser-provided actor identities.
- Legacy Study points, memberships, orders, sign-in rewards, or profile-spend
  behavior.
- A live payment Provider; provider enablement requires a separate Spike and
  later ticket.

## Current boundary

HC-167 establishes the owner read boundary. Portal Gateway forwards only the
verified Portal Session user ID in a replay-protected signed request. Every
read reconciles the user’s missing default rows transactionally and returns a
real persisted zero state; dependency failure is explicit, never a mock
success. Later tickets add user and operator writes without changing owner
identity or the initial-state rule.
