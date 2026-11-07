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

The 2026-11-07 Library readiness slice replaces the unimplemented management
forms with an owner-confirmed capability read. `/api/v1/library/readiness`
requires the existing `library.read` permission and product Scope. Confirmed
management-not-ready, unconfirmed/dependency failure, denied, and expired states
remain distinct. The page reads no private workspace collections and makes no
management writes or pending-command replays; existing pending metadata is not
discarded as evidence of failure. The separately verified public directory is
linked without importing its counts into Console. Existing Workspace and command
client contracts remain available for older consumers. A future ready owner does
not make this Console version a management implementation.
The Overview leads with fresh global Food and unresolved-ticket facts and their
existing destinations. Combined facts require `console.overview.read` and the
platform Scope; every private module also requires its own exact permission and
Scope. Overview denial does not expire the Console Session; summary dependency
failure stays in one diagnostic disclosure without hiding confirmed business
facts. Native reads have independent two-second budgets and a three-second
overall bound. Late or previous-actor responses cannot refill the page.
Platform facts describe only the current Inbox window and the returned Mail
aggregate: its generated timestamp is a list creation fence, not a historical
status snapshot or an independent Mail timestamp. Notice counts describe the
recent50-version window; queued distribution is not delivery. Missing Notice
configuration and unready Library management are capability boundaries, not
claims that the public products are down. Membership and points keep their
existing authorized destinations without invented global KPIs.

The Account ticket workspace reads global counts and the matching server-filtered
facts window of at most100 references/titles. It accepts the explicit all,
unresolved, open, in_progress or resolved status, with an explicit URL filter
taking precedence over a restored one. Search applies only to titles/references
in that window; matched_total and has_more describe its full filter scope.
Source time belongs to that owner read; a later ID detail GET reads the current
version rather than replaying a historical snapshot. Actor-bound session context
retains only the status filter, selected Ticket ID and list position; reply
drafts and title/reference searches remain in page memory. Pending commands
retain only actor, original operation/key, Ticket ID and expected revision.
Recovery only queries the original key; unknown or rejected result reads never
replay a command. Local generations protect selection, actor changes and
component unmounts in addition to the Shell's Session boundary. Conflicts keep
the current in-memory reply draft and require a new explicit submit after the
owner revision refreshes.

Membership grants/revocations and point adjustments also recover only through
the original actor/operation/key result GET. Their pending records retain the
target ID and expected revision or signed amount, never search text, names,
emails, reasons or complete commands. Existing actor-bound full pending records
are converted to that minimum query metadata, never replayed. Storage failure
blocks a new write. A confirmed result remains attached to its original target;
late responses cannot replace a new selection, identity or pending operation.
Membership null/404 is uninitialized, never a confirmed free entitlement.
Unfiltered membership page/target context may be restored after fresh authorized
search; sensitive searches and reason drafts remain in memory. Points require a
fresh full-email lookup to associate a recovered fact with a visible identity.
Points show only the owner's balance at that adjustment and one public entry,
not a live balance read or complete ledger. The membership contract has a
revision but no update timestamp, so the page does not invent one.

HC-517 adds explicit human-targeted confirmation for Platform Session revocation and account status/grant replacement. Platform and Notice retain only minimal unresolved command metadata in session-scoped browser storage, bind it to the current Console user id, and query the original idempotency key after reload without replaying the write.

HC-516 keeps independent cursor history in the browser and offers an explicit refresh that starts again from the four first pages. Signed continuation cursors remain opaque to the browser. The snapshot is a creation-time upper fence, not historical mutable state.

Maturity ticket 09 stores only actor-bound Platform page/cursor history, that creation fence, the Inbox status/selection, and window scroll in session storage. Reload, return and same-actor relogin restore these safe fields; account identity search text/results/search cursors and all editor drafts remain transient. Invalid server cursors require an explicit first-page reload. Local generations discard previous-actor and unmounted reads/writes. Inbox source URLs never become navigation authority: the only verified producer is QuizCraft feedback, whose detail is not integrated in this Console; other source/type contracts are unverified and no origin deep links are advertised.

HC-04 provides the responsive six-module Overview presentation states. HC-09 integrates the same-origin Console Gateway Session endpoint. HC-10 replaces metric fixtures with the Gateway's six-module aggregation response. HC-12 adds a responsive Platform Operations workspace. HC-516 gives each Platform Operations collection an independent bounded page and adds body-only account search by Display Name or full email; the permission panel now reflects the separately verified write capability that controls mutation UI. HC-13 adds Notice review and distribution. HC-14 adds the responsive Library workspace at desktop and 390px, limited to courses, materials, downloads, submission review, and corrections. HC-15 adds responsive Food submission, anomaly-ticket, and tier-adjustment operations. HC-357 changes membership operations to a server-bounded user page: operators search by Display Name or email, select exactly one user identified visibly by Display Name and email, then use the existing grant/revoke workflow; UUIDs remain transport identifiers rather than primary UI labels. HC-441 keeps the existing deep-path login link: an unauthenticated operator at `/food` enters the Portal-owned Account Center and returns only after Console Gateway establishes the authenticated Console Session. Owner writes carry generated idempotency keys, surface optimistic conflicts, and resolve unknown outcomes before resubmission; the browser receives no exchange token, owner-service credential, or product database access.

HC-444 makes that boundary part of the same parameterized release gate used by
Portal. Console must pass signed-out and existing-Core-Session paths,
fail-closed variants, 360px keyboard and reduced-motion checks, public-copy
checks, and browser/log redaction before release artifacts can be produced.
