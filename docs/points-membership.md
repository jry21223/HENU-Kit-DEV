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
- Manual admin grant requires:
  - existing user
  - published plan
  - optional future `expiresAt`
- Re-granting the same active manual membership updates the existing record instead of creating unlimited duplicates.
- Revoked memberships are immediately expired and hidden from `/me/membership`.
- Memberships can unlock `member_only` material downloads through the existing material download permission check.

## Not Implemented Yet

- User self-service membership purchase.
- Points redemption for AI usage, generated papers, or course-package privileges.
- Membership renewal and upgrade/downgrade logic.
- Payment-driven automatic membership activation.
- Admin UI for points rules and memberships.
- Membership notifications such as grant, revoke, or expiry reminders.
- Reusable AI quota/membership entitlement middleware.
