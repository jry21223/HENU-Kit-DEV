# Campus membership benefits (#561 / #562)

Deploy Account Portfolio, Platform Core, Portal and the accompanying LangBot
changes together before advertising campus benefits as available. This work does
not deploy production or issue real supplier/seat orders. Existing membership
price and payment workflow are unchanged. Members incur no additional charge or
point debit for these services; the site pays procurement costs.

## Configuration

Provision one independent membership-only service credential. Account Portfolio
receives `ACCOUNT_PORTFOLIO_MEMBERSHIP_CLIENT_ID`, `_KEY_ID`, `_SECRET`.
Core receives the matching `PLATFORM_CORE_MEMBERSHIP_CLIENT_ID`, `_KEY_ID`,
`_SECRET`, and `PLATFORM_CORE_MEMBERSHIP_BASE_URL` (for a private service mesh:
`http://account-portfolio:8097`). Keep both sets empty to leave the benefit read
unavailable. Partial configuration fails startup. Do not reuse Portal/Console,
Bot or cursor credentials. Only Core receives the read credential; Bot continues
using its own provisioned signed QQ service identity. No credential is public.

## HTTP boundary

Bot sends `POST /api/v1/qq-bindings/benefits` with only `{"subject":"sender"}`
and the existing five-line Basic+HMAC headers and nonce. Core resolves its own
app-scoped binding with active/verified identity. After committing that read it
makes one bounded, three-second, six-line signed owner GET request. Redirects
are rejected and response bodies are limited to 4 KiB. It rechecks binding after
the owner returns to prevent a concurrent unlink or account switch granting old
rights. There is no membership cache.

Success returns `data` containing `bound:true`, `plan:free|lifetime`, `lifetime`,
`allowed`, `benefits`, `checked_at` in UTC RFC3339. Lifetime returns fixed codes
`yuketang`, `ucampus`, `chaoxing`, `library_schedule`; free returns `[]` and
`allowed:false`. `ucampus_ai` uses `ucampus`. No KIT UUID is returned.
Unbound, suspended or unverified identities return `404 NOT_BOUND`.
Owner/configuration/contract failures return `503 MEMBERSHIP_UNAVAILABLE`.
All responses are `no-store`; failure never grants a benefit. Bot must recheck
at each actual action, not treat this result as a durable authorization.

Portfolio permits the membership-only caller solely on
`GET /api/v1/account/membership` without query parameters. It reads the signed
actor's plan, returns free for a missing row without initializing account data,
and rejects all other owner routes. Nonce replay protection remains active.
