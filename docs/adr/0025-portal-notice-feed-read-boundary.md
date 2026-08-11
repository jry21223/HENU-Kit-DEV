---
status: accepted
amends: 0013
---

# Portal Notice feed is a separately authorized, actor-bound Owner read

Portal may show a signed-in student's campus-wide in-app notices. This is a
read exception to Portal Gateway's default public-proxy behavior, not a
Console Notice snapshot and not a general Notice audience policy.

## Decision

- The only browser route is `GET /api/v1/notices`. Portal Gateway first
  validates its Portal Session, then asks Platform Core whether the same
  session actor has `portal.notice.read` at `{kind: product, product_code:
  portal}`. A Platform 401 or 403 ends the request before Gateway contacts
  Notice. The Owner never receives a browser-selected actor.
- Platform Core grants that permission through the narrow
  `portal-notice-reader` role at the `portal` product scope. Migration
  backfill and registration grant it only to active, email-verified HENU
  users. A suspended user, revoked role, or revoked grant stays denied; a
  backfill never reactivates a revoked grant. The active role must contain
  exactly this permission; an unexpected extra role permission prevents both
  backfill and registration from granting it. The Account Portfolio
  release-time role-permission helper explicitly rejects this baseline role,
  so it cannot attach Account operator permissions to it.
- Notice has a dedicated Portal-read credential and the fixed Owner endpoint
  `GET /api/v1/portal/notices`. It is distinct from Console credentials and
  cannot call Console management routes. Its HMAC canonical form includes the
  verified actor as a sixth component, so replacing `X-Actor-User-Id` after
  Gateway signs a request invalidates it. The fixed route carries no
  caller-selected permission or scope, query string, or request body.
- The Owner selects the public feed itself, before ordering and limiting: a
  version is included only when its lifecycle state is `distributed` and an
  associated distribution is exactly `channel=in_app`,
  `audience_kind=all_students`, with no audience value. Email-only,
  college-targeted, role-targeted, and non-distributed versions are excluded.
  `distributed` means Notice accepted the distribution into its lifecycle; it
  does not claim provider delivery completion.
- `notice_sources.canonical_url` is the Notice-owner-managed public-source
  whitelist. A new source and version must use a public HTTPS URL whose
  decoded UTF-8 form is at most 2,048 bytes, with a valid ASCII DNS hostname
  (1–63-character alphanumeric/hyphen labels without edge hyphens, total at
  most 253), no user info, and no local/private/loopback/link-local or numeric
  IP host;
  UTF-8 path and query text remain supported. Ports must be numeric 1–65535,
  and the implicit HTTPS default is normalized with any numeric `:443`
  spelling. A version must have the same normalized HTTPS origin as its source
  canonical URL. No DNS lookup or remote allowlist is introduced.
  The Owner reapplies this policy to the 200 newest legacy database candidates
  before accumulating its final 50-item feed. Within that fixed window, a bad
  old row cannot consume a final item or poison Gateway's bounded response;
  rows outside the window are not promised by this bounded read.
  After those eligibility checks and before the 50-item cap, it also uses the
  exact encoded JSON response (including the request envelope) to enforce a
  5 MiB Owner budget. An item that would exceed that budget is skipped and
  later eligible items are still considered within the candidate window. To
  bound owner work, each request considers only the 200 newest ordered
  database candidates after its lifecycle and audience SQL predicates; invalid
  or over-budget candidates do not consume the final 50-item cap but do
  consume this candidate window, and records outside the window are not
  promised. This remains below
  Gateway's separate 6 MiB read limit; it is not a reason to raise Gateway
  memory use.
- The Owner returns only the public feed fields: notice id, title, body,
  source name and URL, and intake time. It must not return the Console
  lifecycle snapshot, revisions, hashes, distribution counts/statuses,
  audience values, or management metadata. Targeted college and role feeds
  require a separate authority decision.
- Gateway uses only the exact internal Owner origin `http://notice:8094`
  (or its same-port loopback equivalent for local execution), with proxies
  disabled and redirects refused. It validates the bounded Owner response
  before passing the typed public fields to the browser. Owner failure is an
  opaque Gateway 503; malformed Owner data is an opaque 502.

## Executable acceptance seams

The public contracts are tested red-to-green at these boundaries:

1. Notice Owner's actor-bound Portal-feed HTTP contract and its
   filter-before-limit rule.
2. Portal Gateway `GET /api/v1/notices`, which requires Portal Session plus
   the exact Core authorization scope before it calls Owner.
3. Platform Core's active-user grant migration and registration transaction.
4. Portal's `/notice` browser page at desktop and 390px widths, including
   signed-out, denied, empty, unavailable, and valid feed states.

## Consequences

- Existing Console Notice routes keep their credential and management contract;
  this ADR does not broaden them or retrofit their historical signature.
- Deployment needs two explicitly provisioned Notice credential families:
  Console management and Portal read. Compose/release configuration fails
  closed when either required family is absent or the two are reused.
- The Portal copy labels the timestamp as the time the Notice service recorded
  the item; it never presents lifecycle distribution as a completed external
  delivery or invents a college audience claim.
