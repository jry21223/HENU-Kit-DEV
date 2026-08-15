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

**Food Post**:
由登录学生直接发布到公开榜单的餐饮推荐;创建即公开,没有待审核或草稿状态,展示投稿账号的真实显示名。
_Avoid_: Food Submission(那是等待运营审核的旧投稿模型)、portal_food_posts(portal-api 的冻结遗留数据)

## Owns

- Food submissions, anomaly tickets, tier-adjustment requests, and their optimistic versions.
- Food Post creation, public list/detail/venue reads, the actor-scoped mine read, stored photos, and the daily submission cap.
- The Food HTTP/OpenAPI contract, validation, dedicated service credentials, durable actor-scoped idempotency ledger, and append-only audit events.
- Explicit `ok`, `empty`, and `stale` read states.

## Does not own

- Platform accounts, Console Sessions, permissions, Scope, or presentation state.
- Console Gateway authorization policy or browser session credentials.
- Portal Session verification; Portal Gateway binds the signed-in actor (UserID + Display Name snapshot) into the signed Food Post create command.
- Notice, Library, QuizCraft, payment, membership, or platform operations data.

## Current boundary

HC-15 exposes only Food submission review, anomaly resolution/dismissal, and tier-adjustment confirmation/rejection. Console Gateway verifies one exact `food.*` permission within product Scope `food`, signs the actor context, and never connects to the Food database. Every write requires an actor-scoped idempotency key and expected version; its result remains queryable after an uncertain client response.

Food Post routes form a separate credential ring from the Console ring: `POST /api/v1/food/posts` (create, actor-bound) uses the food-post-create credential, while the public reads, the actor-scoped `GET /api/v1/food/posts/mine`, and the image route use the food-post-read credential; the Console `food.*` credential works on none of them, and neither Post credential works on Console routes. A post is public the moment it is created, is capped at three per actor per calendar day, and its display name is snapshotted at creation and never re-resolved. See ADR-0032.
