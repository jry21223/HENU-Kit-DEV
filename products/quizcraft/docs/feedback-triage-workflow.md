# Feedback triage workflow

This is the production workflow for QuizCraft feedback handling.

## Source of truth

- Treat `/opt/quizcraft-cn/tiku/*.json` as the durable question-bank file source.
- PostgreSQL `bank_questions` is the production runtime source when `DATABASE_URL` is configured and `QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB` is not enabled. A durable production repair must keep both the JSON file source and PostgreSQL row in sync.
- Use `(bank_key, question_id)` as the repair key for a concrete question.
- Feedback status values are `pending`, `resolved`, and `archived`. Use `archived` for feedback that should be preserved but removed from the active pending queue.

## Mandatory durable-fix gate

Every feedback repair must follow this order:

1. Fix `/opt/quizcraft-cn/tiku/<bank>.json`.
2. Apply the same fix to PostgreSQL `bank_questions`, or run an explicit one-time sync with `QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB=1`.
3. Verify the live PostgreSQL `bank_questions` row and the public page/API behavior.
4. Mark the feedback `resolved`.

Do not mark feedback as resolved after editing only one side. JSON-only changes may not reach production runtime when startup DB loading is enabled; PostgreSQL-only changes can be lost if a later explicit sync overwrites them from JSON.

## Processing steps

1. Export or inspect pending feedback from the MCP/API.
2. Locate the matching question in `/opt/quizcraft-cn/tiku/<bank>.json`.
3. If only answer/options/analysis change, edit that question in the JSON source and restart/sync.
4. If splitting, inserting, deleting, or reordering questions, update IDs deliberately:
   - `id` must stay unique and stable.
   - Numeric IDs such as `maogai_0032` must remain continuous with the question order.
   - Do not invent suffix IDs such as `maogai_0032_08` for inserted questions.
   - Build an old-to-new `question_id` migration map for every shifted question.
   - Apply the same map to related PostgreSQL tables such as `question_stats` and `feedbacks`.
5. Run `scripts/validate_question_bank_identity.py` on every touched bank.
6. Restart `quizcraft-cn.service` when runtime cache needs to reload, then verify:
   - `/api/banks` or practice start can load the bank.
   - `bank_questions` count matches the JSON count.
   - The repaired question can be fetched/submitted by its final `question_id`.
7. Mark feedback `resolved` only after the durable JSON source, live PostgreSQL row, and runtime API are verified.

## Common failure mode

If a fix only updates PostgreSQL, it may be overwritten by a later explicit JSON-to-DB sync. If a fix only updates JSON, production may keep serving the old PostgreSQL row because startup sync is disabled by default. Always patch both sources or run a deliberate sync, then verify the runtime API.
