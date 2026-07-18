# Points and Membership

This module is now a basic service-side foundation, not a full commercial loop.

## Implemented

- `GET /api/v1/me/points`
  - returns the authenticated user's current points balance.
- `GET /api/v1/me/points/logs`
  - returns only the authenticated user's own points ledger rows.
- `GET /api/v1/admin/points/logs`
  - admin-only points ledger query with `userId`, `reason`, and `limit` filters.
- `GET /api/v1/admin/points/rules`
- `POST /api/v1/admin/points/rules`
- `PATCH /api/v1/admin/points/rules/:id`
  - admin-only rule management; create/update operations write operation logs.
- `GET /api/v1/membership/plans`
  - public, published membership plans only.
- `GET /api/v1/me/membership`
  - authenticated user's active, non-expired memberships plus a best-effort current plan.
- `POST /api/v1/membership/redeem`
  - authenticated, non-frozen users can redeem a published membership plan with positive `pointsCost` and `durationDays`; the request must include a client `requestId` for idempotency.
- `GET /api/v1/admin/memberships`
- `POST /api/v1/admin/memberships/grant`
- `POST /api/v1/admin/memberships/:id/revoke`
  - admin-only manual grant/revoke flow; both actions write operation logs.
- Web pages:
  - `/me/points`
  - `/me/membership`

## Existing Points Rules

- Every point balance change must write a `points_logs` row.
- Balances must not go below zero.
- Forum reward posts deduct points at submission with `forum_reward_escrow`.
- Rejected reward posts refund with `forum_reward_refund`.
- Best-answer selection settles escrowed points with `forum_reward_settlement`.
- Reward settlement is server-side only and guarded by one-best-answer-per-post plus idempotent points-log keys.

## Membership Rules

- Published plans are visible to users.
- Points redemption is server-side only:
  - plan must be published and explicitly redeemable with `pointsCost > 0` and `durationDays > 0`
  - user balance is deducted in the same transaction as membership grant/extension
  - a `membership_redeem` points log records the deduction, final balance, and membership reference
  - duplicate `requestId` values return the existing result without charging again
- Manual admin grant requires:
  - existing user
  - published plan
  - optional future `expiresAt`
- Re-granting the same active manual membership updates the existing record instead of creating unlimited duplicates.
- Revoked memberships are immediately expired and hidden from `/me/membership`.
- Memberships can unlock `member_only` material downloads through the existing material download permission check.
- AI task quota currently reuses memberships and points:
  - free users pay server-defined point costs by task type
  - `tier1` makes wrong-question analysis free and discounts other AI tasks
  - `tier2` makes supported AI tasks free
  - every non-free AI task writes `points_logs.reason=ai_task_usage`
  - every AI task creation writes an `ai_usage_logs` quota row with `source` and `pointsCost`

## Not Implemented Yet

- Payment-backed membership purchase.
- Dedicated AI quota packages, generated-paper packages, or course-package privilege redemption.
- Membership upgrade/downgrade policy beyond same-plan points extension.
- Payment-driven automatic membership activation.
- Membership notifications such as grant, revoke, or expiry reminders.
- Real model-token billing and quota package accounting.
