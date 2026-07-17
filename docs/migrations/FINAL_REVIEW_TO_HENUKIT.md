# final-review-platform → HENU Kit Monorepo 迁移计划

> 状态：执行版 v1.0  
> 迁移策略：非破坏性、可回滚、按部署单元推进  
> 目标仓库：当前 `jry21223/final-review-platform`，完成里程碑后改名为 `jry21223/HENU-Kit`

## 1. 迁移目标

将当前大型学习平台仓库重构为 HENU Kit 的唯一开发仓库，使主站、资料库、QuizCraft、平台核心、共享契约和基础设施位于同一个 monorepo，同时保持：

- 现有 `study.superhuazai.me` 在迁移期间持续可部署。
- 资料下载、审核、支付安全测试和备份能力不丢失。
- QuizCraft 在导入后仍可按原 FastAPI + React/Vite 方式运行。
- 新平台核心使用 Go，但不触发一次性 Python 重写。
- 每个路径移动、认证切换和数据库迁移均可独立回滚。

## 2. 代码确认的起点

当前仓库已经具备：

- `apps/web`：Next.js Web。
- `apps/admin`：Vue Admin。
- `services/api`：Gin + GORM + PostgreSQL API。
- `services/worker`：Go + Redis Streams Worker。
- 课程、资料、下载审计、刷题、社区、积分、会员、支付、AI、通知和管理后台。
- `docker-compose.dev.yml`、生产示例、备份恢复脚本和自动部署工作流。

现有 `services/api` 不是可直接改名为“平台核心”的纯公共服务。它同时持有资料、刷题、支付、社区和用户数据，必须先建立模块边界，再迁移数据 Owner。

QuizCraft 当前：

- React 18 + TypeScript + Vite。
- FastAPI + Pydantic + psycopg。
- PostgreSQL 题库、用户统计、题目统计、反馈和排行榜。
- `POST /api/practice/submit` 接受客户端提供的可选 `user_id` 并直接更新统计。
- 本地 JSON fallback 与 PostgreSQL 并存。

因此不能先把作答写流量切到新 Go 服务。

## 3. 总体迁移序列

```text
M0 仓库基线与导入
→ M1 产品边界与主站骨架
→ M2 平台核心骨架和契约
→ M3 统一账户最小闭环
→ M4 事件、通知和邮件
→ M5 QuizCraft 统一身份接入
→ M6 资料库产品收敛
→ M7 QuizCraft API 渐进 Go 化
→ M8 目录物理重排
→ M9 仓库改名与旧仓库归档
```

目录物理重排放在业务边界和部署兼容完成之后。否则排障时无法区分“路径错误”和“业务迁移错误”。

---

# M0：仓库基线与外部仓库导入

## 目标

让当前仓库成为 HENU Kit 的事实来源，但不改变线上服务。

## 变更

1. 新建迁移分支 `codex/henukit-monorepo-foundation`。
2. 根 README 改为 HENU Kit monorepo 说明。
3. 迁入产品边界、设计系统、架构、开发和迁移文档。
4. 通过 `git subtree --squash` 导入：
   - `jry21223/quizcraft-cn` → `products/quizcraft`
   - `jry21223/HENU-Kit` → `archive/henukit-planning`
5. 增加来源 commit、许可证和更新命令记录。
6. 不修改现有 `apps/web`、`apps/admin`、`services/api`、`services/worker` 的构建路径。

## 验收

- 当前学习平台原有 CI/build 不因文档和导入失败。
- `products/quizcraft` 中包含原仓库完整代码。
- 旧 HENU-Kit 规划文档可追溯。
- 根 README 明确资料库与 QuizCraft 的唯一职责。
- 没有生产配置、真实数据或密钥被导入。

## 回滚

关闭 PR 即可；`main` 和线上部署不受影响。

---

# M1：主站骨架与共享设计变量

## 目标

建立 `henukit.cn` 的真实代码入口，不把主站做成外链列表。

## 目录

```text
apps/portal/
packages/design-tokens/
```

## 主站首批页面

- `/`：三个一级入口和继续上次任务。
- `/tools`：工具目录、维护状态和更新时间。
- `/learn`：资料库、刷题和接毕设（二期占位）。
- `/food`：美食榜单入口或 MVP。
- `/status`：子产品可用状态。
- `/legal/non-official`：主体与数据来源说明。

## 主站不做

- 不代理资料详情正文。
- 不代理题库和作答。
- 不建立第二套用户表。
- 不把通知、邮件和平台核心放在一级导航。

## 共享设计变量

`packages/design-tokens` 第一批只提供框架无关的 CSS/JSON：

- Kit 墨绿 `#0C6B45`
- Kit 墨绿深色 `#05603A`
- Kit 墨绿浅色 `#E4EFE9`
- 纸白 `#F5F1E7`
- 墨色 `#343D36`
- Kit 麦金 `#F0BE44`
- 字级、间距、圆角、阴影和状态色

不在第一阶段开发跨 React/Vue 的统一组件实现；先统一 token、交互和视觉验收。

## 验收

- 360px 宽度下三个一级入口可用。
- 页脚固定展示“学生自主运营 · 非河南大学官方项目”。
- 主色文档和代码均称为 Kit 墨绿。
- 主站与子产品的 Logo 返回行为一致。

## 回滚

主站独立部署，DNS 不切换即可回滚；不会影响资料库或 QuizCraft。

---

# M2：平台核心骨架与 API 契约

## 目标

建立新公共能力的代码边界，不复制现有全部业务。

## 目录

```text
services/platform-core/
services/platform-worker/
packages/api-contracts/openapi/
packages/api-contracts/events/
```

## 技术栈

- Go 1.26.x，CI 固定具体补丁版本。
- Gin、GORM、go-redis，优先复用团队已有经验。
- PostgreSQL。
- Redis Streams。
- OpenAPI 3.1。
- API 和 Worker 分进程。
- JSON 结构化日志。
- `/healthz` 与 `/readyz`。
- 显式 SQL migration；不使用生产 `AutoMigrate` 代替版本迁移。

## 模块

- identity
- email_identity
- verification
- oauth_client
- authorization_code
- session
- account_link
- service_credential
- event
- outbox
- notification
- email_delivery
- metrics
- audit

## API 基线

- `/api/v1`
- `snake_case`
- `request_id`
- `Idempotency-Key`
- UTC ISO 8601
- 统一错误码
- 浏览器 API 与内部 API 分离

## 兼容原则

现有 `services/api` 的 `{code,message,data}` 不立即修改。由资料库适配器把平台核心新格式转换为现有前端需要的格式，或者逐页面迁移消费方。

## 验收

- OpenAPI lint 通过。
- 生成代码无未提交差异。
- PostgreSQL/Redis 集成测试通过。
- Migration upgrade、rollback 和空库重放通过。
- API/Worker readiness 正确反映依赖状态。

## 回滚

平台核心尚无生产调用时直接停止部署；接入后通过 client 级功能开关回到旧认证。

---

# M3：统一账户最小闭环

## 目标

完成一个测试业务站的跨主域登录，不切换所有用户。

## 流程

1. 业务站后端创建 `state` 和登录意图。
2. 浏览器跳转 `account.henukit.cn/api/v1/authorize`。
3. 用户输入河南大学学生邮箱。
4. 平台创建验证码记录和 critical 邮件事件。
5. Worker 通过 DirectMail 发送。
6. 用户验证验证码。
7. 平台创建/读取统一用户。
8. 平台签发短期、单次授权码。
9. 浏览器跳回白名单 callback。
10. 业务站后端交换身份。
11. 业务站建立自己的 HttpOnly、Secure session。

## 关键约束

- 授权码建议 60–120 秒过期。
- 授权码只保存哈希。
- 使用唯一约束或锁保证只交换一次。
- callback 与 `return_to` 分别维护白名单。
- `state` 必须绑定浏览器登录意图。
- 公共客户端或纯 SPA 使用 PKCE；有可信后端的站点仍建议支持 PKCE。
- URL 不传长期 Token。
- localStorage 不保存长期 JWT。

## 旧 final-review 用户

- 不直接改现有 `users.id`。
- 新建 `account_links`：`platform_user_id + service + local_user_id`。
- 首次登录按已验证邮箱提出绑定候选，但必须经过一次性确认流程。
- 冲突账号进入人工合并队列，不能以邮箱自动覆盖。
- 下载记录、资料投稿、积分、会员和订单继续关联旧本地 ID，直到数据 Owner 决策完成。

## 验收

- 真实学生邮箱验证码送达。
- 授权码重复交换返回稳定错误。
- callback 非法被拒绝。
- state 不匹配被拒绝。
- 日志中无完整邮箱、验证码、Token 和 Cookie。
- 测试业务站可切回旧登录。

---

# M4：事件、通知和邮件

## 目标

把“业务直接写通知/发邮件”改为标准事件驱动流程。

## 首批事件

- `submission.received`
- `submission.approved`
- `submission.rejected`
- `correction.resolved`
- `points.credited`
- `school_notice.created`
- `school_notice.updated`

## 事务边界

业务事件接收时，在一个 PostgreSQL 事务中：

1. 根据 `service_id + idempotency_key` 去重。
2. 写 `events`。
3. 写 `outbox_events`。
4. 返回已接受状态。

Outbox relay 再提交 Redis Stream。Redis 暂时不可用不能导致已提交业务事件丢失。

## 邮件优先级

- `critical`：验证码、安全提醒。
- `transactional`：投稿、审核、纠错、积分。
- `digest`：校园通知摘要。

三个优先级使用不同 stream 或独立 consumer policy。摘要堆积不能阻塞验证码。

## 验收

- 重复事件不重复建通知、不重复发积分、不重复发邮件。
- Worker 重启后可继续处理。
- DirectMail 超时进入重试。
- 拒收和重试耗尽进入死信。
- 死信有管理员重新投递入口和审计记录。

---

# M5：QuizCraft 统一身份接入

## 目标

让 QuizCraft 接受可信平台用户身份，同时不丢匿名用户历史统计。

## 当前风险

当前作答请求允许客户端传 `user_id`。统一身份接入后，服务端不得信任浏览器任意提交的统一用户 ID。

## 新模型

在 QuizCraft 数据库增加：

```text
quiz_sessions
quiz_account_links
quiz_anonymous_identities
```

- 匿名用户由服务端签发不可猜测匿名 ID 和 HttpOnly Cookie。
- 登录后由 QuizCraft 后端根据平台交换结果写绑定。
- 作答 API 从可信会话解析 user，不读取请求体中的平台 `user_id`。
- 原有 `users.user_id` 和统计表先保留。

## 绑定流程

- 未登录用户继续使用匿名会话。
- 登录后请求绑定当前匿名身份。
- 事务内锁定匿名身份和目标本地用户。
- 已绑定其他平台用户时返回冲突，不自动改绑。
- 聚合统计先关联原本地 user ID，不做大范围重写。
- 建立可重复执行的数据核对报表。

## 验收

- 旧匿名数据保留。
- 并发绑定只有一次成功。
- 不能通过伪造请求体归属到其他用户。
- 解绑/合并有人工审计入口。
- 可通过功能开关回到原匿名方式。

---

# M6：资料库产品收敛

## 目标

让 `study.superhuazai.me` 只展示资料库。

## 前台处理顺序

1. 从主导航移除刷题、错题、排行榜、泛社区、动态和 AI 独立入口。
2. 课程页刷题按钮改为 QuizCraft 跳转。
3. 首页删除第二套刷题产品文案。
4. 保留资料检索、详情、预览、下载、我的下载、投稿和纠错。
5. 保留必要 Wiki/说明内容，不发展泛社区信息流。
6. “打开资料册”改为轻量一次性动画。

## 后端处理顺序

1. 保留旧路由但标记 deprecated。
2. 禁止新前端继续调用旧刷题路由。
3. 记录 30 天调用量和调用方。
4. QuizCraft 替代路径稳定后返回迁移提示。
5. 经过备份和回滚窗口后，再删除实现。

## 账号、通知、积分等

- 账号：兼容旧会话，逐步切统一账户。
- 通知：逐类改为平台事件。
- 积分：先确认投稿、AI 和会员依赖，暂不删除。
- 支付/会员：冻结扩展，不与平台重构同时上线重大变化。
- Blog/论坛/动态：前台隐藏和冻结；数据只读保留。
- AI：只保留资料处理必要流程，其余按 Owner 拆分。

## 验收

- 学生前台看不到第二套刷题产品。
- 原资料下载、审核和审计 smoke 全部通过。
- 旧 URL 有兼容或明确迁移提示。
- 可通过前端版本回滚恢复原入口。

---

# M7：QuizCraft API 渐进 Go 化

## 迁移批次

### Q1：兼容网关和观测

- Go 入口只代理 FastAPI。
- 统一 request ID。
- 记录 route provider、延迟和错误率。
- 功能开关按 API、用户百分比和环境控制。

### Q2：题库只读

- 题库列表。
- 题库详情。
- 章节与题目读取。
- 隐藏答案字段。

验证：双读 JSON 规范化后完全一致。

### Q3：反馈

- 创建反馈。
- 反馈看板只读。
- 管理状态变更。

验证：唯一键、状态流和管理鉴权一致。

### Q4：练习开始

- 随机、章节、难题模式。
- 随机性使用统计分布而不是逐次 JSON 完全相等。

### Q5：作答与统计

- 先引入 append-only `answer_events`。
- Go 与 FastAPI 双算，不双写同一聚合行。
- 比较正确性、题目统计、用户统计和排行榜快照。
- 一致性达到门槛并连续观察后切写流量。

### Q6：题库工坊

最后迁移文件解析、WebSocket 进度、AI 解析和题库保存。

## 切流门槛

- 契约测试通过。
- 数据一致性报表通过。
- 影子流量无敏感数据泄露。
- P95、错误率和资源使用不劣于基线。
- 回滚开关经过演练。
- 测试负责人批准。

## FastAPI 下线

- 连续至少两周零生产流量。
- 无仅 FastAPI 支持的管理脚本。
- 备份、恢复和回滚演练完成。
- 归档至少保留六个月，不立即删除。

---

# M8：目录物理重排

## 目标路径

| 当前路径 | 目标路径 |
|---|---|
| `apps/web` | `apps/study-web` |
| `apps/admin` | `apps/study-admin` |
| `services/api` | `services/study-api`（公共模块迁出后） |
| `services/worker` | `services/study-worker` |
| `products/quizcraft/web-app` | `apps/quiz-web` |
| `products/quizcraft` 后端文件 | `services/quiz-api-legacy` |

## 每次移动步骤

1. CI 同时识别新旧路径。
2. Docker 和部署脚本增加新路径参数。
3. 使用 `git mv` 完成单一部署单元移动。
4. 修复 import、workspace、build context 和文档。
5. 新旧构建结果比对。
6. 测试环境部署。
7. 合并后观察一个发布周期。
8. 删除旧路径兼容逻辑。

不在一个 PR 同时移动 Web、Admin、API、Worker 和 QuizCraft。

---

# M9：仓库改名与旧仓库归档

## 改名前检查

- 默认分支所有必需检查通过。
- GitHub Actions、Secrets、Environments、Deploy Keys、Webhook 已盘点。
- 容器镜像、部署脚本和状态页不硬编码旧仓库名。
- 文档和贡献入口只指向新仓库。
- 旧 `HENU-Kit` 和 `quizcraft-cn` 仓库准备归档 README。

## 改名后

- 仓库改名为 `HENU-Kit`。
- 验证 GitHub redirect。
- 更新本地 remote、CI badge、镜像仓库和部署自动化。
- 旧仓库设置 archived，只保留迁移说明。
- 不删除历史 release、issue 或许可证信息。

---

# 10. 关键风险与控制

| 风险 | 控制 |
|---|---|
| Monorepo 导入后 CI 时间激增 | 路径过滤、共享缓存、并行任务、夜间全量回归 |
| 当前 `services/api` 同时承担公共和业务能力 | 先建模块边界和契约，不直接改名为 core |
| 资料库隐藏入口但旧 API 仍被调用 | 调用量观测、deprecated header、分阶段停用 |
| QuizCraft 用户 ID 可由客户端提交 | 先引入可信会话，再迁移作答 |
| 排行榜迁移不一致 | 双算、快照 diff、固定数据集、回滚开关 |
| 目录移动破坏自动部署 | 新旧路径兼容、测试环境发布、单部署单元 PR |
| 旧仓库继续产生新提交 | 导入后冻结，旧 README 指向 monorepo |
| 被误认为河南大学官方项目 | 固定非官方声明，不使用校徽和官方色描述 |

# 11. 里程碑退出条件

## R0：Monorepo Foundation

- 外部仓库已导入。
- 根文档已切换为 HENU Kit。
- 现有学习平台构建不受影响。

## R1：Portal Preview

- 主站预览可访问。
- 统一导航和 Kit 墨绿 token 可被三个前端消费。

## R2：Unified Account Pilot

- 一个测试站完成授权码登录。
- 真实验证码可送达。
- 旧登录可回滚。

## R3：Study Boundary

- 学习平台学生前台只展示资料库。
- QuizCraft 是唯一刷题入口。

## R4：Quiz Identity

- 匿名用户和统一用户安全绑定。
- 作答不信任客户端 user ID。

## R5：Repository Rename

- 新仓库结构、CI/CD、文档和部署稳定。
- 旧仓库已冻结。

# 12. 第一批实施任务

1. 导入 QuizCraft 和旧 HENU-Kit 规划仓库。
2. 建立 `apps/portal` 骨架。
3. 建立 `packages/design-tokens`。
4. 建立 OpenAPI 3.1 基线。
5. 建立 `services/platform-core` 空骨架与 CI。
6. 给现有学习平台刷题、社区、动态和 AI 路由加功能清单与调用观测。
7. 为 QuizCraft 作答链路补可信身份威胁模型。
8. 设计课程 ID ↔ 题库 ID 映射文件和 API。
9. 盘点自动部署中所有硬编码旧仓库名和路径。
10. 在测试环境演练单服务回滚。

本计划只定义迁移顺序。任何生产切换仍必须在对应 Issue 中写明负责人、测试结果、灰度比例、监控指标和回滚命令。