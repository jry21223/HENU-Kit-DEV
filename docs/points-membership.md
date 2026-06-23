# Points and Membership

Most points and membership APIs are planned after the MVP course/material/quiz loop. The current exception is forum reward-post escrow, which uses the shared user balance and points ledger.

Rules:

- Every point change must write a ledger row.
- Balances must not go below zero.
- Forum reward posts deduct points at submission with `forum_reward_escrow` and refund rejected reward posts with `forum_reward_refund`.
- Membership grants and revocations must be auditable.
- AI and paper privileges should be checked through reusable entitlement logic.
