# final-review-platform → HENUKitDev Monorepo 迁移计划

> 状态：执行版 v1.1  
> 迁移策略：非破坏性、可回滚、按部署单元推进  
> 目标开发仓库：当前 `jry21223/final-review-platform`，完成里程碑后改名为 `jry21223/HENUKitDev`  
> 公开项目仓库：`jry21223/HENU-Kit` 保持原名和公开入口职责

## 1. 迁移目标

将当前大型学习平台仓库重构为 HENU Kit 的统一开发 Monorepo，使主站、资料库、QuizCraft、平台核心、共享契约和基础设施位于同一个开发仓，同时保持：

- 现有 `study.superhuazai.me` 在迁移期间持续可部署。
- 资料下载、审核、支付安全测试和备份能力不丢失。
- QuizCraft 在导入后仍可按原 FastAPI + React/Vite 方式运行。
- 新平台核心使用 Go，但不触发一次性 Python 重写。
- 每个路径移动、认证切换和数据库迁移均可独立回滚。
- 公开 `HENU-Kit` 仓库继续服务项目介绍、索引、公开路线图和社区信息。

## 2. 仓库职责

### HENUKitDev

- 产品实现代码。
- Platform Core 和业务服务。
- OpenAPI、事件 schema、测试、CI/CD 和部署配置。
- 内部技术文档和迁移 Runbook。

### HENU-Kit

- 公开介绍和项目索引。
- 对外路线图和社区说明。
- 公开贡献指南。

两个仓库不得并行维护同一份实现代码。需要公开的内容从开发仓经评审后同步。

## 3. 代码确认的起点

当前主仓已经具备：

- `apps/web`：Next.js Web。
- `apps/study-legacy-admin`：已物理隔离的 Vue Study Legacy Admin。
- `apps/console`：不含旧路由和 Element Plus 的六模块 Mock Console 壳；尚未接入真实 Gateway、会话或产品数据。
- `services/api`：Gin + GORM + PostgreSQL API。
- `services/worker`：Go + Redis Streams Worker。
- `services/platform-core`：独立的 Go/chi/pgx/sqlc 身份基础切片，当前只覆盖已有 Core Session、S256 授权码与短期 exchange Session。
- 课程、资料、下载审计、刷题、社区、积分、会员、支付、AI、通知和管理后台。
- Compose、生产示例、备份恢复脚本和自动部署工作流。

现有 `services/api` 不是可直接改名为 Platform Core 的纯公共服务。它同时持有资料、刷题、支付、社区和用户数据，必须先建立模块边界，再迁移数据 Owner。

QuizCraft 当前：

- React + TypeScript + Vite。
- FastAPI + Pydantic + psycopg。
- PostgreSQL 题库、用户统计、题目统计、反馈和排行榜。
- `POST /api/practice/submit` 接受客户端提供的可选 `user_id` 并直接更新统计。
- 本地 JSON fallback 与 PostgreSQL 并存。

因此不能先切作答写流量，必须先建立可信匿名会话、统一用户绑定和 append-only 作答记录。

## 4. 身份与访问基线

从 Platform Core 第一张表开始就区分：

- 主体类型：游客、登录用户、服务账号。
- 权限角色：学生、创作者、审核员、运营、管理员、超级管理员。
- 会员档位：Free、VIP。
- Entitlement：具体内容、额度和资源授权。

VIP 是权益，不是权限角色。一个用户可以拥有多个有作用域的角色。详细规则见 `docs/architecture/ACCESS_CONTROL.md`。

目标数据至少新增：

- `user_roles`
- `memberships`
- `entitlement_grants`
- `anonymous_subjects`（由业务模块持有）
- `account_links`

现有 Study `users.role` 和会员数据先保留，采用 expand / migrate / contract。

## 5. 总体迁移序列

```text
M0 仓库基线与导入
→ M1 产品边界、主站骨架和 Design Tokens
→ M2 Platform Core、角色/会员模型和 OpenAPI
→ M3 统一账户最小闭环
→ M4 事件、通知和邮件
→ M5 QuizCraft 可信匿名身份和统一账号绑定
→ M6 资料库产品收敛
→ M7 QuizCraft API 渐进 Go 化
→ M8 目录物理重排
→ M9 开发仓改名为 HENUKitDev
```

目录物理重排放在业务边界和部署兼容完成之后。否则排障时无法区分路径错误和业务迁移错误。

---

# M0：仓库基线与外部代码导入

## 目标

让当前仓库成为实际开发事实来源，但不改变线上服务。

## 变更

1. 使用分支 `codex/henukit-monorepo-foundation`。
2. 根 README 改为 HENU Kit Monorepo 说明。
3. 迁入产品边界、设计系统、架构、开发和迁移文档。
4. 通过 subtree 导入 `quizcraft-cn` 到 `products/quizcraft`。
5. 保存公开 `HENU-Kit` 在导入时的规划快照到 `archive/henukit-planning`，但不替代或归档公开仓库。
6. 增加来源 commit、许可证和更新记录。
7. 不修改现有 Study 构建路径。

## 验收

- 原 Study CI/build 不因导入失败。
- `products/quizcraft` 包含原仓完整代码和来源 SHA。
- 根 README 明确 HENUKitDev 与 HENU-Kit 的仓库分工。
- 没有真实数据或敏感配置被导入。

## 回滚

关闭或 revert Foundation PR；`main` 和线上部署不受影响。

---

# M1：主站骨架与共享设计变量

## 目录

```text
apps/portal/
packages/design-tokens/
```

## 首批页面

- `/`：美食榜单、工具箱、学习三个一级入口。
- `/tools`：工具目录、维护状态和更新时间。
- `/learn`：资料库、刷题和接毕设二期占位。
- `/food`：美食榜单入口或 MVP。
- `/status`：子产品可用状态。
- `/legal/non-official`：主体和数据来源说明。

主站不代理资料正文、题库作答、验证码、通知队列或业务后台。

## 验收

- 360px 宽度下一级入口可用。
- 固定展示“学生自主运营 · 非河南大学官方项目”。
- 主色只称 Kit 墨绿。
- 主站不是简单外链列表，具备统一账户状态和继续任务能力。

---

# M2：Platform Core、角色和会员模型

## 目录

```text
services/platform-core/
services/platform-worker/
packages/api-contracts/
```

## 技术基线

- Go 1.26.x 固定补丁版本。
- `net/http`、chi、pgx、sqlc、PostgreSQL 与 Redis；不使用 Gin、GORM 或运行时 AutoMigrate。
- API 与 Worker 分进程。
- OpenAPI 3.1 是新接口唯一契约来源。
- JSON 结构化日志、request_id、Health 和 Readiness。

## 数据表

- `users`
- `email_identities`
- `verification_codes`
- `oauth_clients`
- `authorization_codes`
- `sessions`
- `account_links`
- `user_roles`
- `memberships`
- `entitlement_grants`
- `service_credentials`
- `events`
- `outbox_events`
- `notifications`
- `notification_preferences`
- `email_deliveries`
- `dead_letters`
- `daily_user_metrics`

## 访问模型

- 游客不写入统一用户表。
- 登录用户默认获得 `student` 角色。
- Creator、Reviewer、Operator、Admin、Super Admin 使用多角色和作用域表。
- Free/VIP 使用 membership，不进入 role。
- 业务接口依据 entitlement 授权。

## 验收

- OpenAPI lint 通过。
- Migration 升级和回滚测试通过。
- Free/VIP 和多角色权限测试可由 Mock 编写。
- 浏览器自报的 user_id、role、membership、entitlement 不被信任。

---

# M3：统一账户最小闭环

## 流程

1. 业务站跳转当前已备案域名 `account.superhuazai.me`。
2. 用户输入学生邮箱。
3. Platform Worker 通过 DirectMail 发送验证码。
4. 验证后创建或读取统一用户。
5. 生成短期单次授权码。
6. 跳回白名单 callback。
7. 业务站后端交换统一身份、roles、membership 和 entitlement 摘要。
8. 业务站建立自己的 HttpOnly、Secure Session。

## 要求

- state 防 CSRF。
- callback 与 return_to 白名单。
- 授权码单次使用和短期过期。
- 服务端交换。
- 不跨主域共享 Cookie。
- 不在 URL 或 localStorage 保存长期 Token。

## 验收

- 学生邮箱真实送达。
- 同一授权码只能成功交换一次。
- 测试业务站能区分游客、Free 学生和 VIP 学生。
- 日志不泄露邮箱、验证码、Cookie 或 Token。

---

# M4：事件、通知和邮件

## 流程

```text
业务事件
→ 服务认证和幂等校验
→ events + outbox 同事务
→ Redis Stream
→ Platform Worker
→ 站内通知和偏好检查
→ DirectMail
→ 投递状态、重试或死信
```

关键事件：投稿收到、审核通过/驳回、纠错处理、积分到账、校园通知新增/更新、会员开通/到期。

验证码和安全提醒使用 critical 队列，不被摘要邮件阻塞。

## 验收

- 重复事件不产生重复通知、积分或会员权益。
- Redis 暂时不可用时 Outbox 保留待投递事件。
- Worker 重启可继续处理。
- 重试耗尽进入死信并有人工处理入口。

---

# M5：QuizCraft 身份接入

## 先做

- 服务端匿名 Session。
- `anonymous_subject_id` 持有证明。
- `account_links` 唯一绑定。
- append-only practice sessions / answers。
- 服务端从本地 Session 获得平台用户 ID。

## 绑定事务

1. 锁定匿名主体。
2. 检查是否已绑定其他用户。
3. 创建唯一 link。
4. 迁移练习和作答归属。
5. 重算聚合统计和排行榜。
6. 写审计和绑定事件。
7. 标记匿名主体不可再次绑定。

## 验收

- 伪造 body `user_id` 无效。
- 匿名用户绑定不丢练习数据。
- 冲突绑定不覆盖历史数据。
- 可切回旧认证入口。

---

# M6：资料库产品收敛

## 保留

- 课程。
- 资料检索、详情、预览、下载。
- 投稿、审核、纠错。
- 下载审计和必要管理后台。

## 隐藏或冻结

- 第二套刷题。
- Wiki、Blog、论坛、动态和关系。
- 泛 AI。
- 积分、会员和支付的前台入口，直到迁移和对账完成。

资料页“去刷题”携带课程映射跳转 QuizCraft。

资料库保留“打开资料册”创意，但动画必须一次性、轻量、可关闭并尊重减少动态设置。

## 验收

- 学生端只展示资料库。
- 原下载权限和记录继续有效。
- 移动端核心流程可用。
- 隐藏功能可由 feature flag 恢复。

---

# M7：QuizCraft API 渐进 Go 化

## 顺序

1. Go Adapter 和 FastAPI fallback。
2. 题库列表和只读查询。
3. 练习开始只读逻辑。
4. Append-only 作答写入。
5. 错题和进度。
6. 排行榜双算。
7. 反馈。
8. 题库工坊最后迁移。

每批迁移要求：OpenAPI 契约、影子流量、结果对比、功能开关、回滚开关和至少一个完整观察周期。

一次性重写不进入计划。

---

# M8：目录物理重排

目标路径：

```text
apps/web -> apps/study-web
apps/study-legacy-admin (former apps/admin; preserved rollback unit)
services/api -> services/study-api
services/worker -> services/study-worker
products/quizcraft/web-app -> apps/quiz-web
products/quizcraft FastAPI -> services/quiz-api-legacy
```

目录移动 PR 不修改业务语义和数据库 schema。必须同步 CI、Compose、Dockerfile、部署脚本和文档，并保留可回退 commit。

---

# M9：开发仓改名

## 前置条件

- Portal、Platform Core、Study、Quiz 均可独立构建和部署。
- GitHub Actions、Deploy Key、Secrets、服务器 remote、脚本和文档已完成名称清单。
- 生产回滚演练通过。
- 外部 webhook 和 GitHub App 权限已核验。

## 唯一改名动作

```text
jry21223/final-review-platform -> jry21223/HENUKitDev
```

现有：

```text
jry21223/HENU-Kit
```

保持不变。

## 回滚

GitHub 通常保留仓库重定向，但仍需在 Runbook 中记录：旧 remote、部署服务器 remote、Actions、webhook、文档链接和本地开发者更新命令。出现部署或权限异常时先恢复原仓库名和 remote，再排查外围集成。

## 6. 发布原则

- main 禁止直接 push。
- PR CI 全通过。
- 镜像使用 commit SHA。
- 生产人工批准。
- Migration 使用 expand / migrate / contract。
- 不自动执行破坏性 Migration。
- 发布前验证备份。
- 灰度失败自动停止放量。
- 每个部署单元独立回滚。

## 7. 第一批执行任务

1. 评审并合并 Foundation PR。
2. 完成 HENUKitDev 与 HENU-Kit 仓库分工说明。
3. 建立 Access Control 契约和测试矩阵。
4. 建立 Portal 可运行骨架。
5. 建立 Platform Core API/Worker 骨架。
6. 建立 user_roles、memberships 和 entitlement migration。
7. 建立验证码与授权码 Mock 契约测试。
8. 收敛 Study 前台导航。
9. 为 QuizCraft 增加可信匿名 Session。
10. 建立课程 ID 与题库 ID 映射。

## 8. 明确不做

- 不替换或改名公开 `HENU-Kit` 仓库。
- 不一次性删除原学习平台功能。
- 不一次性重写 QuizCraft。
- 不把 VIP 当成管理角色。
- 不把游客自动注册成统一用户。
- 不把所有业务数据库合并。
- 不在 Foundation PR 切生产流量。
