# QQ binding release gate (#522)

Source implementation is not production activation. Keep #523 blocked until
the binding journey is integrated and accepted; do not route Bot Food writes
through the current public `create_food_post` tool.

## Provisioning

1. Back up Platform Core and apply migration 000021 using the established
   release procedure. Do not roll this migration down after accepting users.
2. Provision an independent high-entropy secret with the Core-owned
   `services/platform-core/scripts/provision-qq-binding-client.sh`. Its environment
   requires DATABASE_URL, HENU_KIT_QQ_APP_ID, HENU_KIT_CLIENT_ID (henu-bot-*),
   HENU_KIT_KEY_ID and HENU_KIT_SECRET. Do not pass secrets on command lines or
   save them in an issue. App/client remapping fails closed.
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
5. Apply the plugin's `ops/qq-binding-reply-route.patch` to the existing custom
   LangBot image so the initial private message can receive the later account
   preview. Preserve all existing account/login/storage/QQ hotfixes. Never
   replace the complete live plugin tree with the older published branch.

## Acceptance

Use consenting test accounts. Confirm link -> website login -> explicit consent
-> account preview in the original QQ -> plain 确认 -> status on both sides.
Also check another QQ, another Bot, group messages, conflicting KIT identity,
expired links, login revocation, repeated confirm and unlink. Verify unlink
removes future resolution even if the account is temporarily disabled. A lost
preview is safe: the next QQ confirmation shows the preview and requires a new
message; it never silently binds an unseen account.

No secrets or tokens belong in screenshots or acceptance logs. Health 200 is
not evidence of the QQ delivery journey. Roll back application code/disable the
Bot credential if required, retaining durable binding and audit data.
