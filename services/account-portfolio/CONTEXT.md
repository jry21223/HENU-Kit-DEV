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
and creates one user notification in the same transaction. HC-169 adds a
separate point-adjustment boundary: the owner derives an operator solely from
the signed Console request, may initialize the target’s durable Account
Portfolio rows inside that same transaction, appends linked private audit and
ledger facts, and creates the target user’s notification atomically. A debit
uses the ledger-derived balance and fails before any audit, ledger, projection,
or notification write when funds are insufficient. It never imports legacy
Study points or membership state.

HC-171 adds the durable Membership Order kernel. Its state machine may record
`created`, `pending_payment`, `paid`, `closed`, `failed`, and `refunded`, but a
Membership Entitlement changes only in the same transaction as a
Provider-verified payment fact. The local order and a stable merchant-order
intent commit before an adapter can create an external order; retry and a
verified callback can recover a Provider-created but locally unbound order
without creating another external order. The merchant ID is an opaque,
service-only intent value, never the public order ID; a short committed lease
coalesces concurrent retry dispatches and can be reclaimed after a crash. The
current payment fact is the membership's explicit ownership reference, so
refunding an older paid order cannot revoke a later valid lifetime entitlement.
The production process has no enabled payment Provider; Fake Provider behavior
exists only for contract and lifecycle tests. Provider callbacks record bounded
audit codes and payload digests, never raw signatures, secrets, or payment
payloads. ADR-0017 still keeps membership-order commands out of Portal Gateway,
so the browser purchase surface remains closed.

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
An immutable transition from `free` to `lifetime` or from `lifetime` to
`free`. An operator command identifies its authorized operator; a
Provider-verified payment transition identifies its verified Payment Fact
instead. Each transition records its stated reason and idempotency evidence and
has exactly one associated user notification. It is private audit data, not a
Portal membership response.
_Avoid_: Editable membership history, browser audit log

**Membership Revision**:
The positive version of a Membership Entitlement. Console grant and revocation
commands must present the current revision; stale or repeated state changes
fail without creating an event or notification.
_Avoid_: Timestamp-only concurrency, last-write-wins

**Membership Order**:
A durable request for the single ¥9.9 lifetime product. Its local state is
separate from Provider protocol state and can advance only through the
controlled order lifecycle. Its stable merchant-order intent is committed
before an external Provider call, is private to the service and adapter, and
is reused by recovery and retry.
_Avoid_: QR code, successful payment, temporary checkout

**Verified Payment Fact**:
An immutable Provider-verified notification correlated to one Membership Order.
It is the only payment evidence that may grant or revoke a Membership
Entitlement, and replay or stale facts do not repeat an entitlement change. A
payment-backed membership records its current paid fact, so an older order's
refund cannot revoke a later still-valid paid entitlement.
_Avoid_: Browser success callback, unsigned provider payload, session flag

**Payment Provider**:
An isolated adapter responsible for signing, creating and querying external
orders, verifying notifications, and refund protocol behavior. No real Payment
Provider is enabled without a separate Spike and later authorization.
_Avoid_: Portal API, browser secret, mock purchase success

**Point Ledger**:
The ordered, immutable sequence of a user’s point facts. Its signed amounts,
not a mutable balance projection, are the source of truth for both displayed
balance and debit eligibility. Portal sees only its own paged entries and no
operator or audit identity.
_Avoid_: Editable balance history, client-side points cache

**Point Adjustment**:
An authorized Console credit or debit command with a nonzero signed amount,
target user, reason, and idempotency key. The target is command input; the
operator comes only from the actor-bound Console credential and is never a
browser-selected field.
_Avoid_: Browser-admin action, anonymous balance edit

**Point Adjustment Audit**:
The private immutable record of one successful Point Adjustment’s operator,
target, amount, reason, idempotency key, timestamp, and exactly one linked
Point Ledger entry. It is not a Portal wallet response.
_Avoid_: Editable audit note, public operator history

**Point Ledger Cursor**:
An opaque continuation token for the user-owned, descending immutable ledger
page. Portal forwards it as an uninspected bounded query value; clients do not
derive it from an identifier or timestamp.
_Avoid_: Offset pagination, exposed audit identifier
