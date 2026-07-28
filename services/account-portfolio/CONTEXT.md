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
success. HC-170 adds the membership operator boundary: only the separately
configured, actor-bound Console Gateway credential may read or mutate an
already initialized target membership. A Console mutation changes the durable
entitlement and membership revision, appends one immutable membership event,
and creates one user notification in the same transaction. It never creates
an Account Portfolio account for an arbitrary target and never imports legacy
Study membership state.

## Language

**Support Ticket**:
A user-owned durable support conversation with a stable Ticket Reference,
messages, a lifecycle state, and a revision. It is not a Platform Core
identity record or a Console-owned workflow.
_Avoid_: Session ticket, temporary support form

**Ticket Reference**:
The stable human-facing identifier `HKT-<canonical UUID>` derived from a
Support Ticket's persisted UUID. It is not an enumerable sequence or a user
identifier.
_Avoid_: UID, short ticket number

**Ticket State**:
The lifecycle of a Support Ticket: `open`, `in_progress`, or `resolved`. A
user follow-up to a resolved ticket reopens that same ticket; it does not
create a second conversation.
_Avoid_: Closed ticket, deleted ticket

**Operator Reply**:
A durable message written by an authorized Console operator to a Support
Ticket. It identifies the operator and produces a user notification without
copying ownership to Console.
_Avoid_: Console note, browser response

**Membership Entitlement**:
The persisted `free` or `lifetime` Account Portfolio plan for an initialized
user. New users start at `free`; a lifetime entitlement is the durable benefit
associated with the ¥9.9 product, not evidence that a payment Provider ran.
_Avoid_: Study membership, client-side flag, paid receipt

**Membership Event**:
An immutable, operator-attributed transition from `free` to `lifetime` or from
`lifetime` to `free`. It records the stated reason and idempotency key, and
has exactly one associated user notification. It is private audit data, not a
Portal membership response.
_Avoid_: Editable membership history, browser audit log

**Membership Revision**:
The positive version of a Membership Entitlement. Console grant and revocation
commands must present the current revision; stale or repeated state changes
fail without creating an event or notification.
_Avoid_: Timestamp-only concurrency, last-write-wins
