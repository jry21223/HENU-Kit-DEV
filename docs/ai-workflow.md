# AI Workflow

The current V2 AI flow is a mock, review-first foundation. Real LLM and RAG integration are later stages.

Implemented flow:

1. A logged-in, non-frozen user creates an AI task with `POST /api/v1/ai/tasks`.
2. The API calculates the server-side quota cost for the task type and current membership.
3. The API stores the task as `pending`, records an `ai_usage_logs` quota row, and if needed deducts points with a matching `points_logs` row in the same transaction.
4. If Redis is available, the API also writes a lightweight event to the `AI_TASK_STREAM` stream.
5. The worker reads Redis stream events and also scans pending database tasks.
6. In mock mode, the worker marks the task `completed`, writes a result payload, records model usage, and creates an `ai_drafts` row with `status=pending`.
7. Reviewer, admin, and super-admin users can review drafts with approve/reject endpoints and persist review notes.
8. The Vue admin console exposes the review surface at `/ai/drafts`.

Rules:

- AI-generated questions, papers, wiki-like content, and quick review drafts are never published directly.
- Task creation accepts only the current V2 task types: `chat`, `wrong_question_analysis`, `targeted_question`, `paper_generation`, and `draft_review`.
- Task base point costs are currently: `chat=2`, `wrong_question_analysis=5`, `targeted_question=10`, `paper_generation=30`, and `draft_review=15`.
- `tier2` membership makes supported AI tasks free. `tier1` makes wrong-question analysis free and applies a 50% rounded-up point discount to other AI tasks.
- Users without enough points receive `insufficient_ai_points` and the task is not created.
- Reviewer, operator, admin, and super-admin roles are quota-exempt for review and operations workflows.
- Database task state is the source of truth; Redis is only a queue signal.
- Mock LLM output is for local workflow testing only.
- Admin or reviewer approval is required before generated content can become official product content.
- Approval can include an optional note; rejection requires a review reason for traceability.
- Review transitions are one-way for the MVP: only `draft`, `pending`, and `needs_changes` drafts can be reviewed, and terminal review records cannot be overwritten by repeat API calls.
- The current approve/reject endpoints do not publish drafts into official resources. Publish-to-resource flows are a later implementation step.
