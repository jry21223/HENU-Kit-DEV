# QQ binding release gate (#522)

Source implementation is not production activation. Keep #523 blocked until
the binding journey is integrated and accepted; do not route Bot Food writes
through the current public `create_food_post` tool.

## Provisioning

1. Back up Platform Core and apply migration 000021 using the established
   release procedure. Do not roll this migration down after accepting users.
2. From the verified, extracted fixed-SHA runtime, provision an independent
   high-entropy secret with `bin/provision-qq-binding-client.sh` (source:
   `services/platform-core/scripts/provision-qq-binding-client.sh`). The
   controlled execution host must provide `python3`, `psql`, `openssl`, and `xxd`. The
   required environment keys are DATABASE_URL, HENU_KIT_QQ_APP_ID,
   HENU_KIT_CLIENT_ID (henu-bot-*),
   HENU_KIT_KEY_ID and HENU_KIT_SECRET. Supply `DATABASE_URL` privately from
   the verified Core configuration. On the production host, its `postgres`
   hostname resolves only inside Compose: identify the **currently running**
   Postgres container and its verified Compose network, then set
   `HENU_KIT_DATABASE_HOSTADDR` to that container's current bridge IP. Recheck the
   container identity and IP immediately before provisioning. The script
   parses the URI into libpq environment variables; the URI and its password
   never enter `psql` arguments. Do not pass secrets on command lines or save
   them in an issue. Client ID, application mapping, and key-ID collisions fail
   closed. Repeating the active key with the same secret is a no-op.
   Rotation retains the preceding key as `retiring` for a controlled rollout
   window. After verifying the plugin uses the new key, explicitly set the old
   client's key to `revoked` through the normal credential-management procedure;
   rotation alone does not immediately revoke the old secret.
3. Deploy Core, Gateway and Portal from one reviewed SHA, preserving existing
   product configuration. The new website page is `/bind/qq`.
4. Configure the LangBot plugin's private `.env` (0600): HENU_KIT_CORE_URL,
   HENU_KIT_PORTAL_URL (HTTPS origins), HENU_KIT_CLIENT_ID, HENU_KIT_KEY_ID,
   HENU_KIT_SECRET and HENU_KIT_BOT_UUID. BOT_UUID is the LangBot instance's
   configured official-QQ bot UUID, not the QQ application ID. Only events from
   that exact Bot may use this credential.
5. Against the exact running custom LangBot image, apply the plugin's
   `ops/qq-binding-reply-route.patch`, then
   `ops/qq-binding-source-time.patch`, then
   `ops/qq-binding-receipt-route.patch`, each with zero fuzz. Against the
   exact running plugin-runtime image, also apply
   `ops/qq-binding-sdk-quote.patch` with zero fuzz so the verified reference
   survives multiple plugin serialization hops. The first main-image patch
   keeps the initial private reply route for the later account preview; the
   second retains the official QQ event time; the third returns a validated
   QQ C2C text-send ID, server timestamp and reference index to the plugin,
   carries a strictly verified WebSocket origin marker for every KIT command
   and the incoming QQ quote reference, rejects unsigned webhook delivery to
   this WebSocket Bot, and removes a debug log that could record the private
   binding link. Run the plugin's
   `ops/verify-qq-source-time.py`, `ops/verify-qq-receipt-route.py`, and
   `ops/verify-qq-sdk-quote.py` against both sets of patched files as directed
   in `HENU_Assistant/ops/qq-binding.md`. Verify both exact live image bases and
   the runtime SDK hash first. In a controlled candidate stage, use consenting
   QQ private test messages to verify the real send response includes the
   required fields, the adapter returns them to the plugin, and a real quote
   reaches the plugin with the same reference index. Keep binding disabled if
   any field is absent or mismatched; fixtures alone cannot establish this
   contract. Preserve all existing account/login/storage/QQ hotfixes. Never
   replace the complete live plugin tree with the older published branch.

## Acceptance

Use consenting test accounts. Confirm link -> website login -> explicit consent
-> account preview in the original QQ -> quote that exact preview and send
plain `确认` -> status on both sides. An unquoted `确认` may request the preview
again but must never create the binding.

Also check another QQ, another Bot, group messages, a quote of a different
message, a malformed quote, disabled-webhook delivery, conflicting KIT
identity, expired links, login revocation and repeated confirmation. First,
use two harmless C2C prompts to verify that their outbound reference indices
are distinct and an inbound quote carries the matching index; neither prompt
can authorize binding. In a consenting binding flow, issue two target-account
previews, reject a quote of the older one and accept only a quote of the latest
one. Verify plain, unquoted `确认` unlinks, while quoted unlink confirmation
does not. Verify unlink removes future resolution even if the account is
temporarily disabled. If the preview was not sent, the next QQ confirmation
shows it and requires a new message. A QQ send receipt proves server
acceptance, not delivery or that the person saw the preview; confirm this with
the consenting test account rather than inferring it from the receipt.

No secrets or tokens belong in screenshots or acceptance logs. Health 200 is
not evidence of the QQ delivery journey. Roll back application code/disable the
Bot credential if required, retaining durable binding and audit data.
