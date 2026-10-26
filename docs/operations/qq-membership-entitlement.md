# QQ Bot membership entitlement reader

The QQ timed-library feature resolves the current Bot + sender binding through
Platform Core before every plan creation, confirmation, and scheduled execution.
It then reads the bound UID's durable entitlement from Account Portfolio. No
membership fact is copied into Core or inferred from a QQ message.

`GET /api/v1/bot/membership-entitlement` uses the existing six-line actor-bound
HMAC service authentication with replay protection. The dedicated Bot reader
credential can access only this endpoint; Portal and Console credentials cannot
access it. Response data is `{user_id: UUID, lifetime: boolean}`. Missing owner
membership returns false without initializing account rows. Dependency failures
return 503, never a successful default entitlement. Revocation to free takes
effect on the next read. This endpoint grants no payment or mutation authority.

Optional Account Portfolio deployment configuration:

- `ACCOUNT_PORTFOLIO_BOT_CLIENT_ID`
- `ACCOUNT_PORTFOLIO_BOT_KEY_ID`
- `ACCOUNT_PORTFOLIO_BOT_SECRET` (independent of Portal, Console, and cursor keys)

All three must be set together. Without them the feature remains unavailable.
Expose only this exact route to the authenticated Bot over HTTPS. The plugin
uses corresponding `HENU_KIT_ENTITLEMENT_URL`, `CLIENT_ID`, `KEY_ID`, `SECRET`
variables (all with the `HENU_KIT_ENTITLEMENT_` prefix).

Roll out the owner endpoint and dedicated credential before enabling plugin
plans. One active plugin runtime owns the durable scheduler registry; active
replicas require a storage-level compare-and-set lease before enabling.
No deployment or membership change is performed by this implementation.
