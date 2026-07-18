# 统一管理后台决策记录

> 本文件只记录已接受、会影响后续实现方向的决策。变更必须说明原因、影响、迁移和回滚，不允许静默覆盖。

## ADR-ADMIN-001：继续使用 Vue 3

- 状态：Accepted
- 日期：2026-07-18
- 决策：在现有 `apps/admin` 上渐进建设统一后台，不重写为 React。
- 原因：已有 Vue 路由、认证、用户、资料、审核和统计页面；重写不会直接增加业务价值。
- 影响：新页面与新 Shell 使用 Vue 3 + TypeScript。
- 回滚：不适用；若未来改框架需新 ADR 和完整迁移计划。

## ADR-ADMIN-002：新 UI 使用 shadcn-vue

- 状态：Accepted
- 日期：2026-07-18
- 决策：新 Admin Shell 和新页面统一使用 shadcn-vue（Reka UI base）+ Tailwind CSS v4。
- 原因：开放源码、组合式组件和统一界面规范更适合长期由开发者和 AI 共同维护。
- 约束：新页面禁止引入 Element Plus；旧页面仅在迁移期保留。
- 回滚：Feature Flag 回旧 Shell；删除 shadcn 基础设施必须单独 PR。

## ADR-ADMIN-003：Platform Core Admin Module 作为 BFF

- 状态：Accepted
- 日期：2026-07-18
- 决策：第一阶段不新建独立 Admin Gateway 服务，由 Platform Core 的 Admin Module 聚合数据。
- 原因：减少部署单元和早期运维成本，同时保持服务间 API/HMAC 边界。
- 约束：Admin Module 不获得 Study/Quiz/Food/Notice 数据库凭据。
- 复审：当后台流量、发布节奏或团队边界要求独立扩缩容时再拆分。

## ADR-ADMIN-004：数据 Owner 不因统一后台改变

- 状态：Accepted
- 日期：2026-07-18
- 决策：Platform Core 拥有用户、订阅、邮件、平台反馈、统一待办和审计；业务正文仍归各服务。
- 约束：统一待办只保存 `source_service + source_type + source_id` 和运营摘要。

## ADR-ADMIN-005：学院归属与订阅分离

- 状态：Accepted
- 日期：2026-07-18
- 决策：用户首次登录可填写学院、专业、年级；通知邮件仍需用户主动订阅。
- 约束：自报学院只用于推荐和匹配，不自动产生管理角色或受保护内容权限。

## ADR-ADMIN-006：自建邮件通过 Provider Adapter

- 状态：Accepted
- 日期：2026-07-18
- 决策：允许部署自建发信服务，但验证码、通知和业务代码只调用 Mail Provider Interface。
- 约束：Critical、Transactional、Digest 分队列；accepted 不等于 delivered；可切 Fake/备用 Provider。

## ADR-ADMIN-007：两级反馈

- 状态：Accepted
- 日期：2026-07-18
- 决策：平台反馈与 QuizCraft 题目反馈分表、分服务、分状态流程；后台统一导航和待办。
- 约束：题目反馈只有完成 JSON、PostgreSQL 和运行时验证后才能 resolved。

## ADR-ADMIN-008：美食榜单采用“定档 + 校准”

- 状态：Accepted
- 日期：2026-07-18
- 决策：投稿人选择五档中的建议初始定位，社区投票只回答被低估/差不多/被高估。
- 约束：不实现五星评分；不把票数解释为“喜欢”；档位顺序来自定义表；前端不硬编码。

## ADR-ADMIN-009：美食调档按轮次

- 状态：Accepted
- 日期：2026-07-18
- 决策：每次调档关闭旧轮并创建新轮，旧票不再推动新位置。
- 首发：系统推荐 + 管理员确认，每次最多一档。
- 约束：存在异常未处理时 `blocked_by_risk`，禁止调档。

## ADR-ADMIN-010：校园通知保留不可变版本

- 状态：Accepted
- 日期：2026-07-18
- 决策：官网内容更新创建新版本，禁止覆盖旧正文。
- 约束：前台和后台均保留原来源、原发布时间和原链接。

## ADR-ADMIN-011：开发文档 03 为服务通信基线

- 状态：Accepted
- 日期：2026-07-18
- 决策：所有管理后台 API、HMAC、事件、幂等、Envelope 和兼容策略严格遵守 `docs/development/api-communication-spec.md`。
- 影响：必须修复现有早期 OpenAPI 中与该文档冲突的路由、HMAC Header 和 Event Envelope。

## ADR-ADMIN-012：六张业务卡固定保留并渐进接入

- 状态：Accepted
- 日期：2026-07-18
- 决策：总览固定展示用户、校园通知、邮件、反馈、美食、系统六张卡。
- 约束：未接入域必须显示 `not_integrated` 与 `—`，不得用示例值或 0 冒充真实数据；正式 V1 验收时六域全部接入。
- 回滚：运行时 Feature Flag 可立即切回旧 Shell。

## ADR-ADMIN-013：校园通知 V1 仅人工录入与 JSONL 导入

- 状态：Accepted
- 日期：2026-07-18
- 决策：V1 支持后台单条表单和 UTF-8 `campus-notice-import/1.0` JSONL 导入，每任务最多 1,000 条或 10 MB。
- 约束：表单与 JSONL 共用 Upsert；`(source_id, external_id)` 唯一；相同内容幂等，内容变化创建不可变新版本。
- 明确不做：自动抓取、QQ 空间同步、网页解析器、OCR、来源抓取失败指标和解析器测试。
- 存储：通知附件使用 S3 兼容对象存储，本地与 CI 使用 MinIO。

## ADR-ADMIN-014：统一待办采用两档 SLA

- 状态：Accepted
- 日期：2026-07-18
- 决策：`urgent` 创建后 24 小时到期，`normal` 创建后 72 小时到期。
- 约束：只有未解决且超过 `due_at` 才计算 overdue；受限管理员可调整到期时间，调整必须审计。

## ADR-ADMIN-015：美食试运营 Policy 使用 10 人、70%、7 天

- 状态：Accepted
- 日期：2026-07-18
- 决策：有效参与者少于 10 为 `insufficient_votes`；任一方向达到 70% 才形成对应候选；最近调档不足 7 天为 `cooldown`。
- 约束：suspected/invalidated 票不进入分母；存在未处理阻断异常为 `blocked_by_risk`；管理员每次最多升降一档。

## ADR-ADMIN-016：S3 兼容存储与通用 SMTP

- 状态：Accepted
- 日期：2026-07-18
- 决策：通知附件和美食图片使用 S3 兼容存储；邮件使用 Fake 与通用 SMTP Adapter。
- 约束：生产凭据只由 Secret/环境配置注入；SMTP 接受只记 `accepted`，没有 DSN 或回调证据不得记 `delivered`。

## ADR-ADMIN-017：新认证不进入本 Epic

- 状态：Accepted
- 日期：2026-07-18
- 决策：本 Epic 保留现有认证流程，通过旧认证适配器验证 RS256 管理员 Token，并从内部身份接口读取角色与 Scope。
- 约束：适配器不得读取 Study 数据库；未来新认证上线只替换验证器，不改变 Admin API 与前端。

## 变更模板

```markdown
## ADR-ADMIN-XXX：标题

- 状态：Proposed / Accepted / Superseded / Rejected
- 日期：YYYY-MM-DD
- 背景：
- 决策：
- 备选方案：
- 影响：
- 迁移：
- 回滚：
- 替代/被替代：
```
