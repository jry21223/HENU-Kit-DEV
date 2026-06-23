# Points and Membership

Most points and membership APIs are planned after the MVP course/material/quiz loop. The current exception is forum reward-post escrow and settlement, which uses the shared user balance and points ledger.

Rules:

- Every point change must write a ledger row.
- Balances must not go below zero.
- Forum reward posts deduct points at submission with `forum_reward_escrow`, refund rejected reward posts with `forum_reward_refund`, and grant escrowed points to the selected best-answer author with `forum_reward_settlement`.
- Reward settlement is triggered only by server-side best-answer selection and is guarded by a one-best-answer-per-post check plus idempotent points-log keys.
- Membership grants and revocations must be auditable.
- AI and paper privileges should be checked through reusable entitlement logic.
