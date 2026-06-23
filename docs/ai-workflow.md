# AI Workflow

The current V2 AI flow is a mock, review-first foundation. Real LLM and RAG integration are later stages.

Implemented flow:

1. A logged-in user creates an AI task with `POST /api/v1/ai/tasks`.
2. The API stores the task as `pending`.
3. If Redis is available, the API also writes a lightweight event to the `AI_TASK_STREAM` stream.
4. The worker reads Redis stream events and also scans pending database tasks.
5. In mock mode, the worker marks the task `completed`, writes a result payload, records usage, and creates an `ai_drafts` row with `status=pending`.
6. Admin users can review drafts with approve/reject endpoints.
7. The Vue admin console exposes the review surface at `/ai/drafts`.

Rules:

- AI-generated questions, papers, wiki-like content, and quick review drafts are never published directly.
- Task creation accepts only the current V2 task types: `chat`, `wrong_question_analysis`, `targeted_question`, `paper_generation`, and `draft_review`.
- Database task state is the source of truth; Redis is only a queue signal.
- Mock LLM output is for local workflow testing only.
- Admin or reviewer approval is required before generated content can become official product content.
- The current approve/reject endpoints do not publish drafts into official resources. Publish-to-resource flows are a later implementation step.
