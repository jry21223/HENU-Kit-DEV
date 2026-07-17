# 任务清单与 GitHub Issues

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 使用规则

- 以下任务已经合并原计划中的重复 Issue，适合小团队直接录入 GitHub Projects。
- 一个 Issue 目标工期 0.5–2 个工作日；表中超过 2 日的任务必须在开发前继续拆分。
- `Owner` 编写实现与单元测试；`Reviewer` 负责独立评审和验收，不代表只有一人可以参与。
- 阶段任务全部满足退出条件后才进入下一阶段；允许准备后续 Mock，但不得提前切生产流量。
- `OPT-*` 不进入首发关键路径。

## 2. 可执行 Backlog

| ID | 阶段 | 标题 | Owner | Reviewer | 开发/测试 | 依赖 | 验收条件 |
|---|---|---|---|---|---:|---|---|
| P0-01 | Phase 0 | 仓库、分支、生产 SHA 与部署 Inventory | 开发A | 测试A | 1d / 0.5d | - | 报告包含三仓、本地/线上版本、Remote、Actions、Secrets 引用且已脱敏 |
| P0-02 | Phase 0 | 生产数据与 30/90 天活跃盘点 | 开发A | 测试A | 1.5d / 0.5d | P0-01 | 每个重复能力有保留/迁移/冻结结论 |
| P0-03 | Phase 0 | Foundation 导入 Hash、License 与基线验证 | 测试A | 开发B | 0.5d / 1.5d | PR #10 | Study/Quiz 基线全过，无缺失、Secret 或超限文件 |
| P0-04 | Phase 0 | 阻止纯 Foundation 变更自动部署 Study | 开发B | 测试B | 0.5d / 0.5d | - | Study 路径变化才触发部署；Docs/Archive 不触发 |
| P0-05 | Phase 0 | 固定数据 Owner、Quiz 冻结与公开仓同步 Runbook | 开发B | 开发A+测试A | 1d / 0.5d | P0-01 | 不存在同一功能在两仓长期独立开发 |
| P1-01 | Phase 1 | 固定 Go 工具链并完成全量回归 | 开发B | 测试B | 1d / 0.5d | P0 | Go/Docker/CI 版本一致，业务行为不变 |
| P1-02 | Phase 1 | Platform Core/Worker 骨架与 request_id | 开发A | 测试A | 3d / 1d | P1-01 | Core/Worker 独立启动，Health/Ready 与 JSON Log 通过 |
| P1-03 | Phase 1 | 版本化 SQL Migration Baseline | 开发B | 测试A | 2d / 1d | P1-01 | 空库/已有库 Up 通过，生产不再运行破坏性 AutoMigrate |
| P1-04 | Phase 1 | OpenAPI 3.1 与 Event Envelope 骨架 | 开发A | 测试A | 2d / 1d | P1-02 | Lint、Mock、生成代码和最小接口契约可消费 |
| P1-05 | Phase 1 | 路径过滤 PR CI、Branch Protection、CODEOWNERS | 开发B | 测试B | 3.5d / 1.5d | P0-04 | 至少 8 组路径矩阵；共享包触发全部消费者 |
| P1-06 | Phase 1 | Portal Shell | 开发B | 测试B | 2d / 1d | P1-05 | 三个一级入口、状态占位、360px 和非官方声明通过 |
| P2-01 | Phase 2 | Users/Email Identity/Role/Membership/Entitlement Schema | 开发A | 测试A+非作者开发 | 4d / 2.5d | P1-03 | 多角色和 VIP 独立；唯一约束、Migration 和回滚通过 |
| P2-02 | Phase 2 | 邮箱/IP/设备限流 | 开发A | 测试A | 1.5d / 1d | P2-01 | 并发阈值、Redis 故障 Fail Closed、日志脱敏通过 |
| P2-03 | Phase 2 | Events/Outbox 幂等接收基础 | 开发A | 测试A | 2d / 1d | P1-03,P1-04 | 重复 Event 一条；事务故障无部分提交 |
| P2-04 | Phase 2 | 验证码请求事务与 Outbox | 开发A | 测试A | 2d / 1d | P2-02,P2-03 | 202、不暴露账号；Code Hash/Event/Outbox 同事务 |
| P2-05 | Phase 2 | Critical Mail Worker 与 DirectMail Adapter | 开发B | 测试B | 2d / 1d | P2-04 | Fake Timeout/Reject 和真实测试邮箱送达通过 |
| P2-06 | Phase 2 | 验证码单次验证事务 | 开发A | 测试A | 2d / 1d | P2-01,P2-04 | 20 并发仅一次成功；User/Identity/Session 同事务 |
| P2-07 | Phase 2 | OAuth Client 与精确 Callback 白名单 | 开发A | 测试A | 1d / 0.5d | P1-04 | 非法 Callback 不重定向；Client 可禁用 |
| P2-08 | Phase 2 | Authorization Code、PKCE 与 Token Exchange | 开发A | 测试A | 3.5d / 2d | P2-06,P2-07 | State/PKCE/Code 重放阻断；身份响应最小化 |
| P2-09 | Phase 2 | Sessions、撤销与全局退出 | 开发A | 测试B | 2d / 1d | P2-08 | 本地/Core Session 撤销，跨站最终失效 |
| P2-10 | Phase 2 | Portal + 测试业务站账号接入 | 开发B | 测试B | 2d / 1d | P2-08,P2-09 | 跨站登录、回跳、退出三浏览器通过 |
| P2-11 | Phase 2 | 身份响应返回 Roles/Membership/Entitlements | 开发A | 测试A | 1d / 1d | P2-01,P2-08 | Portal/Study/Quiz 状态展示一致，客户端不能自报 |
| P2-12 | Phase 2/3 | Study 旧角色与会员幂等回填 | 开发A | 测试A+测试B | 2d / 2d | P2-01 | 全量计数一致、零自动提权、异常进入人工复核 |
| P3-01 | Phase 3 | 服务间 HMAC 签名与 Nonce 防重放 | 开发A | 测试A | 2d / 1d | P1-04 | 正确签名通过；重放/过期/错误 Key 被拒绝 |
| P3-02 | Phase 3 | 服务密钥双 Key 轮换 | 开发A | 测试A | 1.5d / 1d | P3-01 | Pending→Active，旧 Key 宽限后撤销，Secret 只展示一次 |
| P3-03 | Phase 3 | Redis Streams Consumer Group 与 Reclaim | 开发B | 测试A | 2d / 1d | P2-03 | Kill -9 后可重领且只处理一次 |
| P3-04 | Phase 3 | 站内通知、偏好和已读 | 开发A | 测试A | 2d / 1d | P2-03 | 当前用户隔离、重复事件幂等、退订即时生效 |
| P3-05 | Phase 3 | 邮件重试、优先级和 DLQ | 开发B | 测试B | 2d / 1d | P2-05,P3-03 | Critical/Transactional/Digest 隔离，人工重放有审计 |
| P3-06 | Phase 3 | Activity 与 Daily Metrics | 开发A | 测试A | 1.5d / 0.5d | P2-03 | UTC 聚合可重算，不保存业务正文 |
| P4-01 | Phase 4 | Quiz Go Adapter 与 Legacy Fallback | 开发B | 测试A | 2d / 1d | P1-05 | 新 Envelope、Legacy 回切和 Contract 通过 |
| P4-02 | Phase 4 | Quiz 服务端签名匿名 Session | 开发B | 测试A | 2d / 1d | P4-01 | 匿名 ID 后端生成；Body/LocalStorage 自报无效 |
| P4-03 | Phase 4 | Quiz User Links 与绑定合并事务 | 开发B | 测试A | 2d / 1d | P4-02,P3-01 | 唯一 Link、冲突不覆盖、统计合并一次 |
| P4-04 | Phase 4 | 题库读取 Shadow | 开发A | 测试A | 2d / 1d | P4-01 | 字段一致率 ≥99.99%，未知差异为 0 |
| P4-05 | Phase 4 | Practice Sessions/Answers Append-only | 开发B | 测试A | 2d / 1d | P4-02 | Answer 可追溯、可重算、重复提交幂等 |
| P4-06 | Phase 4 | 作答影子判分与双统计 | 开发B | 测试A | 2d / 1.5d | P4-04,P4-05 | 判分 100% 一致，统计差异 ≤0.1% |
| P4-07 | Phase 4 | 排行榜双算与切流开关 | 开发B | 测试A | 2d / 1d | P4-06 | Top100 连续 14 天一致，可一键回 Legacy |
| P4-08 | Phase 4 | Course ID 到 Bank Key 显式映射 | 开发A | 测试B | 1.5d / 0.5d | P4-01 | 未映射明确提示，不按名称猜测 |
| P5-01 | Phase 5 | Library User Links 与 Dry-run | 开发A | 测试A | 2d / 1d | P2-08,P0-02 | Create/Link/Conflict/Skip 报告；旧 FK 不改 |
| P5-02 | Phase 5 | Study 统一登录兼容层 | 开发A | 测试A | 2d / 1d | P5-01,P2-09 | 新 Session 可访问历史数据，旧 JWT 可受控回退 |
| P5-03 | Phase 5 | 隐藏/冻结 Study 重复刷题与非首发入口 | 开发B | 测试B | 1.5d / 1d | P4-08 | 学生端只展示资料库，旧数据和后台只读保留 |
| P5-04 | Phase 5 | Portal MVP 与跨站状态 | 开发B | 测试B | 2d / 1d | P1-06,P2-10 | 账户状态、模块状态和 Portal/Study/Quiz E2E 通过 |
| P6-01 | Phase 6 | Study 路径移动兼容层 | 开发B | 测试B | 1d / 0.5d | P5 | 旧命令和逻辑模块命令生成等价 Artifact |
| P6-02a | Phase 6 | 移动 Study Web | 开发B | 测试B | 1.5d / 0.75d | P6-01 | 只改路径/构建引用，旧 Smoke 与新 Build 通过 |
| P6-02b | Phase 6 | 移动 Study Admin | 开发B | 测试B | 1d / 0.5d | P6-02a | 同上 |
| P6-02c | Phase 6 | 移动 Study API | 开发B | 测试B | 2d / 1d | P6-02b | 同上；API 与 Schema 无变化 |
| P6-02d | Phase 6 | 移动 Study Worker | 开发B | 测试B | 1.5d / 0.75d | P6-02c | 同上；队列和任务行为无变化 |
| P6-03 | Phase 6 | 拆分 Quiz Web 与 FastAPI Legacy 路径 | 开发A | 测试A | 2d / 1d | P4 | 旧/新路径产物一致，不重写 FastAPI |
| P6-04 | Phase 6 | Commit SHA 镜像与 Staging Pipeline | 开发B | 测试B | 2d / 1d | P6-02d,P6-03 | 每个 Deploy Unit 记录实际 SHA，Staging 自动 Smoke |
| P6-05 | Phase 6 | Blue/Green、人工批准和一键回滚 | 开发B | 测试B | 2d / 1.5d | P6-04 | Readiness 故障停止放量并回旧 |
| P6-06 | Phase 6 | 备份恢复和 RPO/RTO 演练 | 测试B | 开发A | 1.5d / 0.5d | P6-04 | 空环境恢复后计数/Hash 与 Smoke 通过 |
| P6-07 | Phase 6 | HENUKitDev 改名演练与切换 | 测试B | 开发B | 1d / 1d | P6-05,P6-06 | 公开 HENU-Kit 不变；Actions/Key/Remote/Webhook 全过 |
| T-01 | 持续测试 | Auth/Event/Mail 契约与故障自动化 | 测试A | 开发A | 0d / 2d | P2-P3 | 关键接口、重放、事务和 Worker 故障进入 Required Checks |
| T-02 | 持续测试 | 跨浏览器、移动端与邮件送达 E2E | 测试B | 开发B | 0d / 2d | P2-P6 | 三浏览器、360/390px、真实测试邮箱和回滚路径自动化 |
| OPT-01 | 可选 | 学校通知源白名单与抓取审计 | 开发A | 测试A | 2d / 1d | 核心首发后 | 只抓公开白名单，记录来源、原发布时间和非官方声明 |
| OPT-02 | 可选 | Notice 版本、去重与事件 | 开发A | 测试A | 2d / 1d | OPT-01 | 正文不覆盖旧版本；Source+URL 唯一 |
| OPT-03 | 可选 | 主动订阅、摘要和退订 | 开发B | 测试B | 2d / 1d | OPT-02,P3-05 | 默认关闭，退订立即且幂等，不占 Critical Worker |

## 3. GitHub Issue 模板

```markdown
## 背景

## 目标

## 范围

## 明确不做

## 允许修改路径

## API / Event / Migration

## 前置依赖

## 实现步骤
1.
2.
3.

## 测试
- [ ] Unit
- [ ] Contract
- [ ] Integration
- [ ] Failure/Concurrency/Idempotency
- [ ] E2E（如适用）
- [ ] Security/Redaction（如适用）

## 验收条件
- [ ]

## 发布与回滚

## Owner / Reviewer

## 预计工时
```

## 4. 标签

```text
area:repo
area:portal
area:platform
area:study
area:quiz
area:contracts
area:infra
area:test
risk:security
risk:data
risk:release
priority:p0
priority:p1
priority:p2
status:blocked
status:shadow
status:ready-for-release
```

## 5. 看板列

```text
Backlog
Ready
In Development
In Review
Ready for Test
Testing
Blocked
Ready for Release
Released / Observing
Done
```

`Done` 只用于观察期结束、回滚证据和文档均完成的任务；刚上线的任务放在 `Released / Observing`。
