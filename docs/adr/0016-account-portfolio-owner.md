---
status: accepted
supersedes: 0002 (points and membership Candidate Capability statements only)
---

# Account Portfolio owns durable user account facts

Points, memberships, membership orders, account notifications, and support
tickets require one durable owner distinct from Platform Core identity and from
the retiring Study implementation. Account Portfolio is therefore an
independently deployed Go service with its own PostgreSQL database and
versioned OpenAPI contract.

## Considered options

- Keep the existing Study tables or Portal mock stores as the account source.
  Rejected: they produce divergent cross-device facts and silently preserve
  legacy product rules.
- Add account balances and ticket data to Platform Core. Rejected: identity,
  authorization, and account financial/workflow facts have different
  lifecycles and operational boundaries.
- Establish Account Portfolio as a separate durable owner. Accepted: it
  separates legacy data, supports auditable future writes, and gives Portal one
  fail-closed account boundary.

## Consequences

- The first authenticated Account Portfolio read transactionally creates a
  durable zero point balance and free membership. Empty notifications and
  tickets are real empty collections, not session fixtures.
- Portal browsers call only Portal Gateway. The Gateway takes the actor from
  the encrypted Portal Session and signs a replay-protected internal request;
  a browser cannot select another user or receive service credentials.
- Platform Core remains the Display Name source of truth. A Portal Session may
  carry its login-time Display Name snapshot for presentation, but no account
  page uses a UID as a human label.
- No Study points, memberships, or payment state are automatically migrated;
  new users begin at zero points and free membership. Sign-in rewards and
  profile-spend behavior are not introduced.
- The ¥9.9 lifetime membership order model may exist, but every real payment
  Provider remains disabled until the dedicated Provider Spike concludes.
- This ADR changes the points and membership Candidate Capability statements in
  ADR-0002 only. The Console navigation and operational write paths remain
  governed by their own tickets and acceptance gates.
