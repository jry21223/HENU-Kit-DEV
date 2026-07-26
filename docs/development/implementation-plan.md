# HENUKitDev 开发实施计划

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 目标与首发范围

项目目标不是把所有代码搬到一个目录，而是建立一个可独立构建、部署、扩缩容和回滚的 Monorepo：

- Portal：统一品牌、入口、账户状态和跨产品导航。
- Study：只保留课程、资料、预览、下载、投稿、审核和纠错。
- QuizCraft：唯一题库、练习、作答、错题、进度和排行榜 Owner。
- Platform Core：统一用户、邮箱验证、会话、授权码、账户映射、服务认证、事件、通知、邮件和用户指标。
- 共享包：Design Tokens、OpenAPI、事件 Schema 和测试 Fixtures。

首发不处理社区复活、支付系统重构、泛 AI 能力扩展和校园通知；这些功能保留数据和旧实现，但从前台隐藏或进入后续 Epic。

## 2. 团队规模与工期

| 模式 | 日投入假设 | 建议周期 | 适用条件 |
|---|---|---:|---|
| 2 开发 + 2 测试 | 开发各 4h；测试各 3h | 18 周 | 推荐；测试并行覆盖契约/安全与浏览器/发布 |
| 2 开发 + 1 测试 | 开发各 4h；测试 3h | 21 周 | 可执行；发布和目录移动阶段需串行验收 |
| 1 开发 + 1 测试 | 开发 4h；测试 3h | 28–32 周 | 仅在范围严格冻结时采用；校园通知和物理目录移动可后置 |
| 基础版 | 1–2 开发 + 1 测试 | 8–10 周 | 只完成 Foundation、CI、Portal Shell、Core、统一账号和一个业务站登录闭环 |

一名开发不得同时承担两个安全关键链路的实现和最终验收。测试资源不足时，优先延后目录移动、排行榜切流和可选功能，不降低账号、数据和回滚门槛。

按本包合并后的 Backlog 估算，核心范围约为 **92.5 个开发工作日 + 54.5 个测试工作日**；校园通知可选 Epic 另增加约 **6 个开发工作日 + 3 个测试工作日**。这里的工作日沿用源计划的兼职口径，实际日历周期还包含依赖等待、评审、影子流量、灰度和不少于 20% 缓冲。

| 阶段 | 开发工作日 | 测试工作日 |
|---|---:|---:|
| Phase 0 | 4.5 | 3.5 |
| Phase 1 | 13.5 | 6.0 |
| Phase 2–3 | 36.0 | 20.5 |
| Phase 4 | 15.5 | 8.0 |
| Phase 5 | 7.5 | 4.0 |
| Phase 6 | 15.5 | 8.5 |
| 持续测试自动化 | 0 | 4.0 |

## 3. 人员职责

| 角色 | 主责 | 不得独立验收 |
|---|---|---|
| 开发 A | Platform Core、OpenAPI、身份、权限、事件、数据迁移 | 自己实现的验证码、授权码、服务签名、Migration |
| 开发 B | Portal、Study/Quiz 接入、Worker、CI/CD、目录迁移 | 自己实现的账号绑定、部署切换、回滚脚本 |
| 测试 A | API 契约、数据库、并发、幂等、安全、数据一致性 | 生产最终批准仍需项目负责人 |
| 测试 B（可选） | 浏览器 E2E、移动端、邮件送达、灰度、改名和恢复演练 | 不代替测试 A 的数据库与安全验收 |

## 4. 阶段计划（2 开发 + 2 测试基线）

### Phase 0：真实盘点与 Foundation（W1）

**必须完成**

- 归档三个仓库、本地分支、生产 SHA、服务器 remote、Actions、Secrets 引用和部署路径。
- 对 Study/Quiz 做只读数据量和 30/90 天活跃盘点。
- 验证 PR #10 导入 hash、License、关键文件和 Study/Quiz 基线构建。
- 阻止纯 `docs/products/archive` 变更自动部署 Study。
- 固定数据 Owner、Quiz 旧仓冻结流程和公开仓同步流程。

**退出条件**

- PR #10 不会因纯结构变更触发生产部署。
- 生产数据与活跃能力有书面结论。
- 旧仓、开发仓和公开仓不存在双向长期开发路径。
- 全员确认第一版范围和停止条件。

### Phase 1：工程基础、Portal Shell 与 Core 骨架（W2–W3）

**开发 A**：Platform Core/Worker 骨架、`request_id`、OpenAPI 3.1、Core 数据库 Migration。  
**开发 B**：Go 工具链、路径过滤 PR CI、分支保护、Portal Shell。  
**测试**：临时 PostgreSQL/Redis、契约 Mock、空库/已有库 Migration、所有现有产品构建回归。

**交付物**

- `services/platform-core`、`services/platform-worker` 可独立启动。
- `/api/v1/healthz`、`/api/v1/readyz` 可测试。
- `packages/api-contracts` 成为新 Core 契约唯一来源。
- 共享包改动能触发所有消费方 CI。
- Portal 在 360px 和桌面端可访问三个一级入口并显示非官方声明。

### Phase 2：统一账号与访问模型（W4–W6）

**实现顺序**

1. `users`、`email_identities`、`user_roles`、`memberships`、`entitlement_grants`、`sessions`。
2. 邮箱/IP/设备限流；验证码请求事务与 Outbox。
3. critical Mail Worker 与 DirectMail Fake/真实测试。
4. 验证码单次消费事务。
5. OAuth Client 精确 callback 白名单。
6. Authorization Code + PKCE + Token Exchange。
7. 本地业务 Session、退出和全局撤销。
8. Portal + 一个业务站完成登录/退出。

**退出条件**

- 真实测试邮箱送达，验证码不进入日志、数据库正文或响应。
- 同一码并发验证仅一个请求成功；同一授权码只可交换一次。
- callback、state、PKCE、限流、Cookie 和日志脱敏全部通过。
- Free/VIP 与 Creator/Reviewer/Operator/Admin 权限可独立计算。

### Phase 3：服务认证、事件、通知和邮件可靠性（W7–W8）

**实现内容**

- 服务间 HMAC 签名、nonce 防重放和双 Key 轮换。
- Events/Outbox 接收、Redis Streams Consumer Group、PEL/XAUTOCLAIM。
- 站内通知、偏好、已读。
- critical/transactional/digest 队列隔离、重试、DLQ、人工重放。
- 最小 activity 上报和 UTC 日聚合。
- Study 旧角色/会员幂等回填和双读兼容。

**退出条件**

- 重复事件不重复通知、积分或统计。
- Worker 被 `kill -9` 后任务可被安全重领且只完成一次。
- 服务 Key 轮换期间新旧 Key 同时可用，宽限后旧 Key 失效。
- Redis/邮件供应商故障有明确降级、重试和告警证据。

### Phase 4：QuizCraft 可信身份与影子迁移（W9–W12）

**实现顺序**

- Go Adapter + FastAPI legacy fallback。
- 服务端签名匿名 Session，拒绝请求体自报 `user_id`。
- `quiz_user_links` 与匿名/统一用户绑定事务。
- 题库读取 shadow 比较。
- `practice_sessions`、`practice_answers` append-only 模型。
- 作答影子判分与双统计。
- 排行榜双算和 Feature Flag。
- Study `course_id` 到 Quiz `bank_key` 显式映射。

**退出条件**

- Quiz 在 Monorepo 内可独立构建和部署。
- 题库读取未知差异不超过 0.01%；判分差异为 0。
- 统计可重算，重复提交不重复计数。
- 排行榜 Top100 连续 14 天一致或所有差异都有签字解释。
- 任一阶段可回切 legacy provider。

### Phase 5：Study 收敛和 Portal MVP（W13–W14）

- 建立 `library_user_links` 和旧账号 dry-run 报告。
- Study 接入统一登录兼容层，旧业务 FK 不改。
- 学生端隐藏/冻结 Study 中重复刷题、Wiki、Blog、论坛、动态、支付和泛 AI 入口。
- 所有“去刷题”通过课程映射跳转 QuizCraft。
- 完成 Portal 账户状态、模块状态和跨站导航。

**退出条件**

- Study 学生端只展示资料库能力。
- 下载、投稿和审核历史映射抽样 100% 正确。
- Portal/Study/Quiz 跨站登录、退出、移动端和回滚 E2E 通过。

### Phase 6：目录迁移、独立发布与仓库改名（W15–W18）

1. 建立旧路径到逻辑模块的 Build Alias。
2. 使用独立 PR 移动 Study Web、Admin、API、Worker；PR 不改业务和 Schema。
3. 拆分 Quiz Web 与 FastAPI legacy 路径，不改写为 Go。
4. 一个 Commit SHA 构建多个独立镜像，部署 staging。
5. 执行 Migration、readiness、smoke、契约、E2E、备份恢复和 blue/green 回滚。
6. 单独窗口演练并执行 `final-review-platform -> HENUKitDev`；公开 `HENU-Kit` 不改名。
7. 观察一周后再删除路径 Alias 或归档 Quiz 旧仓。

**退出条件**

- 新旧目录产物等价，旧生产 smoke 全通过。
- 每个 Deploy Unit 可独立发布和回滚。
- GitHub redirect、Actions、Deploy Key、remote、webhook、Branch Protection 全部验证。
- 生产观察期无阻断指标。

## 5. 关键路径

```text
真实盘点
→ Foundation 安全合并
→ 路径过滤 CI / Migration / Core Skeleton
→ 统一账号
→ 事件与邮件
→ Quiz 可信身份和影子迁移
→ Study 收敛
→ 目录移动与独立 CD
→ 仓库改名
→ 生产观察
```

以下事项不进入关键路径：校园通知、支付重构、社区恢复、泛 AI、Quiz 全量 Go 重写。

## 6. 每周管理节奏

- 周一：确认本周 Issue、依赖、允许修改路径和测试数据。
- 每个 Issue：0.5–2 个工作日；超过 2 日必须拆分。
- 每日：开发提交可运行增量；测试同步更新用例和自动化。
- 周三：契约/Migration/风险评审，不等待功能完成。
- 周五：阶段 Demo、回归、风险登记和下周冻结范围。
- 发布窗口：测试签字、负责人批准、回滚命令和观察指标必须齐全。

## 7. Definition of Done

一个 Issue 只有同时满足以下条件才可关闭：

- 功能代码与单元测试已提交。
- OpenAPI、事件 Schema 或 Migration 与代码一致。
- 正常、失败、重复、并发和权限路径已测试。
- 日志含 `request_id` 且无敏感信息。
- Feature Flag、回滚方式和数据清理方式明确。
- 当前 SHA 完成 Standards / Spec 双轴 Review；安全关键项具备独立的失败路径测试和回滚证据。
- CI 全绿，staging smoke 通过，文档和运行命令已更新。
