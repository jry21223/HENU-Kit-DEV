# 数据库与数据迁移规范

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 数据库隔离

第一阶段至少维护三个逻辑数据库：

| 数据库 | Owner | 主要内容 |
|---|---|---|
| `henukit_core` | Platform Core | 用户、身份、会话、授权、事件、通知、邮件和指标 |
| `final_review_v2` | Study | 课程、资料、下载、投稿、审核及历史业务数据 |
| `quizcraft` | QuizCraft | 题库、练习、作答、统计、排行榜和题库工坊 |

可以部署在同一 PostgreSQL 集群，但必须使用不同数据库用户、权限、Migration 和备份。任何服务不得持有其他数据库的写权限。

## 2. Migration 规则

- 新服务必须使用版本化 SQL Migration；生产关闭 AutoMigrate 和运行时建表。
- Migration 文件一经进入共享环境不得修改，只能新增后续版本。
- 每个 Migration 必须提供：前置检查、Up、可行的 Down 或明确不可逆说明、数据量估计、锁影响、验证 SQL 和回滚策略。
- Schema 改动与业务切流分开 PR；破坏性 `contract` 只在兼容期结束后执行。
- CI 必须验证空库 Up、已有 Baseline Up、必要的 Down、重复执行和并发启动。

## 3. `expand -> migrate -> contract`

### Expand

- 新增表、字段、索引、唯一约束和兼容读写路径。
- 不删除旧列，不改变旧调用方语义。
- 发布后确认旧服务仍可工作。

### Migrate

- Dry-run 输出 create/link/conflict/skip。
- 小批量回填，记录批次、来源、Hash、耗时和异常。
- 双读或影子读对账；写流量通过 Feature Flag 灰度。
- 所有异常进入人工复核，不自动覆盖数据。

### Contract

- 确认旧流量为零、数据对账通过、回滚窗口结束。
- 再删除旧字段、旧表、兼容代码和 Alias。
- Contract 与仓库改名、域名切换不得在同一窗口执行。

## 4. Platform Core 最小数据模型

| 表 | 关键约束 | 敏感与保留 | 访问范围 |
|---|---|---|---|
| `users` | UUIDv4；状态索引 | 不直接保存邮箱；软删后匿名化 | Core；业务通过 API |
| `user_roles` | 有效 `(user_id, role_code, scope_type, scope_id)` 唯一 | 撤销保留 180 天审计 | Core AuthZ |
| `memberships` | 来源引用与 `idempotency_key` 唯一 | 财务/权益引用脱敏，长期保留 | Core Membership |
| `entitlement_grants` | 用户、Code、资源、来源唯一 | 撤销保留审计 | Core AuthZ/Rewards |
| `email_identities` | `normalized_email` 唯一 | 邮箱加密可选，日志必须掩码 | Identity |
| `verification_codes` | `request_key` 唯一；只存 HMAC | 原码不落库；24h 清理 | Identity/Mail Worker 最小读取 |
| `oauth_clients` | `client_id` 唯一；Callback 精确匹配 | Secret 只存 Hash | Identity/Admin |
| `authorization_codes` | `code_hash` 唯一 | 60–120s 单次；消费/过期后 24h 清理 | Identity |
| `sessions` | `session_token_hash` 唯一 | IP/UA 限长脱敏；30–90 天审计 | Identity；业务只 Introspect |
| `account_links` | `(service_id, external_user_id)` 唯一 | 证据 Metadata 脱敏；不级联删业务数据 | Identity/对应业务服务 |
| `service_credentials` | `(service_id, key_id)` 唯一 | Secret AES-GCM；主密钥外置 | ServiceAuth Only |
| `events` | `(service_id, event_id)` 唯一 | Payload 不含凭据和正文；默认 180 天 | Events/Projectors |
| `outbox_events` | `dedupe_key` 唯一 | 成功 30 天；失败转 DLQ | Dispatcher |
| `notifications` | `(event_id, user_id, type)` 唯一 | 默认 1 年；用户可归档/删除 | 用户本人 |
| `notification_preferences` | `(user_id, channel, topic)` 唯一 | 账号生命周期 | 用户本人 |
| `email_deliveries` | `delivery_key` 唯一 | 收件人加密/引用；正文不保存或 7 天清理 | Mail/Ops |
| `dead_letters` | `(source_type, source_id)` 唯一 | 错误栈脱敏；365 天 | Ops/Admin |
| `daily_user_metrics` | 日期、指标、Scope 复合唯一 | 只存聚合 | 只读报表 |

## 5. 事务、锁与幂等

- 用户可见的业务写入与 Outbox 必须同事务。
- 验证码、授权码、Account Link、积分 Ledger、作答提交使用条件更新或行锁。
- 多行合并采用固定锁顺序，避免死锁；死锁只允许有限自动重试。
- 幂等由数据库唯一约束兜底，不得只依赖缓存。
- 统计必须可由 Append-only 明细重算；禁止只更新聚合而无明细来源。
- 积分必须使用 Ledger + Balance 同事务，禁止只改余额。

## 6. 旧 Study 用户迁移

1. 只读审计用户总数、Verified、重复规范化邮箱、异常邮箱、冻结用户和关联订单/积分/资料/投稿数量。
2. 新建 `library_user_links(legacy_user_id, platform_user_id, status, evidence, linked_at)`。
3. 只对“已验证 + 唯一邮箱”批量预创建；每批先 Dry-run。
4. 同一邮箱多个旧账号进入 `manual_review`，不自动合并。
5. 旧业务表继续引用 Legacy UUID；运行时通过 Link 解析统一用户。
6. 新登录走 Core；旧 JWT 只在兼容期双读，观察后关闭。

禁止用统一邮箱直接更新旧表主键或级联修改历史业务 FK。

## 7. QuizCraft 匿名用户与作答迁移

- 新匿名 ID 必须由后端生成并放入签名 HttpOnly Cookie。
- 旧 LocalStorage `user_id` 只是候选，不是所有权证明。
- 新增 `quiz_user_links`；不修改 Legacy Text 主键。
- 当前聚合统计只能合并计数，不能重建历史答案。
- 先新增 `practice_sessions` 与 Append-only `practice_answers`，未来作答才支持完整迁移。
- 合并匿名和登录账号时锁定两侧统计、写 Merge Ledger，原匿名账号置 `linked/retired`，不删除。

## 8. Study 刷题数据到 QuizCraft

| 阶段 | 操作 | 验证 | 回滚 |
|---|---|---|---|
| 盘点 | 统计 Question/Attempt/Answer/WrongQuestion 数据量和活跃度 | 只读 SQL + 30/90 天日志 | 无变更 |
| 映射 | Course UUID → Bank Key；题型与答案格式映射 | Schema Validator + 样本 | 不写生产 |
| 影子导入 | 导入可证明的数据并保留 Source ID | 数量、Hash、判分回放 | 删除影子 Schema |
| 内部可见 | 内部账号只读展示历史 | 用户隔离 E2E | 关闭 Flag |
| 切入口 | Study 链接统一跳 Quiz | 映射和点击回归 | 恢复旧入口 |
| 归档 | 旧 Quiz API 只读至少 90 天 | 无旧流量、快照和 Checksum | 保持只读，不删表 |

## 9. 保留、删除与脱敏

- 验证码明文永不保存；Hash 记录 24 小时后清理。
- 授权码消费或过期后 24 小时清理。
- Session Token 只存 Hash；过期后删除 Hash，保留必要审计。
- 事件和邮件投递默认保留 180 天；失败 Dead Letter 保留 365 天。
- 完整邮箱、IP、UA、支付引用和外部身份均按最小化原则记录。
- 用户删除请求不得级联删除业务记录；先撤销 Link，再按各业务治理策略匿名化。

## 10. Migration Review Checklist

- 是否明确 Owner、数据量、锁范围和预计时间。
- 是否与业务行为、目录移动和仓库改名分离。
- 是否有 Dry-run、批次记录、异常清单和对账 SQL。
- 是否支持旧版本服务继续运行。
- 是否在 staging 做过真实规模抽样和恢复演练。
- 是否有 Feature Flag、回切路径和 Contract 最早日期。
