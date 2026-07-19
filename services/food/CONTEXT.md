# Food Operations Context

## Language

**Food Module**:
HENUKit Console 中处理餐饮信息投稿、异常票和档位调整的 Active Product Module；业务事实始终由 Food 服务拥有。
_Avoid_: Console 餐饮数据库、Gateway Food 后台

**Food Submission**:
等待 Food 运营人员批准或拒绝的餐饮地点与菜品投稿。
_Avoid_: Notice 投稿、Library 投稿

**Food Anomaly Ticket**:
Food 对重复、垃圾、质量或位置异常的可版本化处理记录。
_Avoid_: Platform Operations Inbox 正文

**Food Tier Adjustment**:
由 Food 记录并等待运营确认或拒绝的地点档位变更建议。
_Avoid_: Console 排名配置

## Owns

- Food submissions, anomaly tickets, tier-adjustment requests, and their optimistic versions.
- The Food HTTP/OpenAPI contract, validation, dedicated service credentials, durable actor-scoped idempotency ledger, and append-only audit events.
- Explicit `ok`, `empty`, and `stale` read states.

## Does not own

- Platform accounts, Console Sessions, permissions, Scope, or presentation state.
- Console Gateway authorization policy or browser session credentials.
- Notice, Library, QuizCraft, payment, membership, or platform operations data.

## Current boundary

HC-15 exposes only Food submission review, anomaly resolution/dismissal, and tier-adjustment confirmation/rejection. Console Gateway verifies one exact `food.*` permission within product Scope `food`, signs the actor context, and never connects to the Food database. Every write requires an actor-scoped idempotency key and expected version; its result remains queryable after an uncertain client response.
