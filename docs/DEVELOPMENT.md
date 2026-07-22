# HENU Kit Monorepo 开发规范

> 本文件是当前仓库的开发基线。影响品牌、账户、导航、隐私、数据 Owner、API 契约或模块边界的变更，必须先更新对应文档或 ADR，再修改实现。

## 1. 项目身份

HENU Kit 是由河南大学学生自主发起、运营和维护的校园工具系统，非河南大学官方产品，不代表学校官方立场。

所有正式页面、README、登录、支付和宣传材料必须出现完整或短版声明：

> 学生自主运营 · 非河南大学官方项目

禁止使用“河南大学官方工具箱”“学校指定平台”“河大官方标准色”等未经证实的表述。

## 2. 产品原则

- HENU Kit 是一个系统，不是简单的网站导航集合。
- 主站统一品牌、入口、导航、账户状态和跨产品体验。
- 资料库只负责资料，不开发第二套刷题。
- QuizCraft 是唯一刷题产品。
- 子产品可以独立部署和使用不同技术栈。
- 统一的是设计 token、导航行为、账户、API 契约、状态语义、日志和安全基线。
- 平台核心是隐藏基础设施，不作为首页一级入口。

完整边界见 [`product/PRODUCT_BOUNDARIES.md`](product/PRODUCT_BOUNDARIES.md)。

## 3. 仓库结构规则

### `apps/`

只存放直接面向浏览器的应用：

- Portal
- Study Web
- Study Admin
- Quiz Web

应用可以依赖 `packages/`，不得直接 import 另一个应用内部源码。

### `services/`

存放独立运行进程：

- Platform Core API/Worker
- Study API/Worker
- Quiz API Legacy

服务不得直接读取其他服务数据库。

### `packages/`

只放至少有两个真实消费方的共享内容：

- 设计 token
- OpenAPI/事件 schema
- 生成客户端
- 测试 fixture

不要因为“以后可能复用”提前抽共享包。

### `products/`

迁移缓冲区。外部仓库先整体导入，保持原运行方式；完成契约和部署兼容后再拆到 `apps/` 与 `services/`。

### `legacy/` 与 `archive/`

只读历史参考，不是运行依赖。新增业务代码不得放入这些目录。

## 4. 分支与 PR

- 禁止直接 push `main`。
- 一个 PR 解决一个可说明的问题。
- 目录移动、业务行为变化、数据库迁移和流量切换原则上拆成不同 PR。
- PR 必须写：背景、范围、明确不做、验证、风险、发布、回滚。
- 账户、权限、支付、邮件、服务认证和 Migration 需要非作者评审。
- Owner 不能独立验收自己的安全关键实现。
- 大型迁移先开 Draft PR，先评审结构和风险，再填充实现。

规范分支：

```text
feature/<area>/<issue-id>
fix/<area>/<issue-id>
```

## 5. Commit

使用可检索的约定式前缀：

```text
feat(portal): add learning entry
fix(auth): reject reused authorization code
refactor(study): route quiz entry to QuizCraft
test(quiz): add ranking consistency check
docs(architecture): record monorepo decision
chore(ci): add path-filtered checks
```

禁止把大量无关改动压在一个提交中。导入外部仓库使用独立 squash commit 并记录来源 SHA。

## 6. Issue 准入

Issue 必须包含：

- 用户问题和目标人群。
- 当前代码证据与文件路径。
- 目标和可验收结果。
- 范围与明确不做。
- API、数据表和事件。
- 前置依赖。
- 安全、隐私和合规影响。
- 测试设计。
- 发布与回滚。
- Owner、Reviewer、开发/测试工时。

“优化一下”“完善系统”“重构代码”不是可执行 Issue 标题。

## 7. 前端规范

### 技术栈

第一阶段尊重现有技术栈：

- Portal：根据仓库实现确定，优先与现有 Node 工具链兼容。
- Study Web：Next.js。
- Study Admin：Vue。
- Quiz Web：React + Vite。

不得为了框架统一重写成熟页面。

### 设计系统

- 主色：Kit 墨绿 `#0C6B45`。
- 背景：纸白 `#F5F1E7`。
- 强调：Kit 麦金 `#F0BE44`，使用比例不超过 10%。
- 同一视区原则上只有一个 Primary 动作。
- 可点击区域最小 `44 × 44px`。
- 正文不小于 `16px`。
- 支持键盘焦点和 WCAG AA 对比度。
- 360px 宽度下核心流程必须可完成。
- Loading、Empty、Error、Success、Disabled、Login required 状态必须完整。

### 资料库动画

“打开资料册”只允许轻量一次性动画：

- `600–900ms`。
- `transform` + `opacity`。
- 主操作首帧可用。
- 移动端和减少动态模式不依赖动画。
- 禁止持续滚动计算、多层视差和连续翻页进入关键路径。

### 跨产品导航

- HENU Kit Logo 返回 `henukit.cn`。
- 一级入口命名一致。
- 账户入口位置一致。
- 内部产品默认当前标签页打开。
- 外部项目明确标识并可新标签页打开。
- 资料库“去刷题”必须进入 QuizCraft，并携带白名单课程上下文。

## 8. API 规范

新平台核心以 OpenAPI 3.1 为唯一接口契约来源。

### 统一要求

- 前缀：`/api/v1`
- JSON：`snake_case`
- 时间：UTC ISO 8601
- 公开 ID：不可猜测
- 响应包含 `request_id`
- 写接口按场景支持 `Idempotency-Key`
- 统一分页、错误码和废弃流程
- 内部 API 与浏览器 API 分离
- 日志脱敏

成功：

```json
{
  "data": {},
  "request_id": "req_xxx"
}
```

失败：

```json
{
  "error": {
    "code": "AUTHORIZATION_CODE_USED",
    "message": "授权码已使用，请重新登录",
    "details": null
  },
  "request_id": "req_xxx"
}
```

### 兼容旧接口

- 不要求 final-review 与 QuizCraft 旧接口同时改格式。
- 新入口通过 adapter 转换。
- 废弃接口先加文档、指标和响应头，再移除消费方，最后删除实现。
- 公开 API 不返回答案、存储路径、审核内部字段或敏感身份字段。

## 9. 身份与会话

- 不通过跨主域共享 Cookie 统一登录。
- 业务站跳转 `account.henukit.cn` 完成 Authorization Code Flow。
- callback、`return_to` 和 client 必须预登记。
- `state` 防 CSRF。
- 授权码短期、单次使用，只保存哈希。
- 有后端的业务站使用服务端交换。
- 公共客户端使用 PKCE。
- 业务站建立自己的 HttpOnly、Secure 会话。
- URL 不传长期 Token。
- localStorage 不保存长期 JWT。
- 业务站不得读取平台核心数据库。
- 不以邮箱覆盖旧用户数据。

验证码：

- 5–10 分钟过期。
- 单次使用。
- 至少 60 秒重发等待。
- 邮箱、IP、设备小时/日级限流。
- 原始验证码、完整邮箱、Token 和 Cookie 不得进入日志。

## 10. 数据库与 Migration

### Owner

每类数据只能有一个 Owner。跨服务关联使用不可猜测 `user_id`，不使用邮箱作为长期键。

### Migration

- 生产禁止依赖 GORM AutoMigrate 管理破坏性变更。
- 新平台核心使用版本化 SQL migration。
- 采用 expand / migrate / contract。
- 新字段先可空或有兼容默认值。
- 数据回填可暂停、可续跑、可重复执行。
- 删除列和表必须在消费方停止使用后单独发布。
- CI 执行空库升级、从上一个版本升级和允许范围内的回滚测试。
- 生产发布前验证备份和恢复流程。

### 事务与幂等

- 业务状态和 Outbox 在同一数据库事务中写入。
- 幂等依赖唯一约束，不只依赖应用层查询。
- 并发绑定、授权码交换、积分到账、支付回调必须使用锁或条件更新。
- 聚合统计迁移使用 append-only 事件或可重放源数据。

## 11. 事件、通知和邮件

业务模块提交标准事件，不直接调用通用发信接口。

事件必须包含：

- `event_id`
- `event_type`
- `occurred_at`
- `producer`
- `subject_user_id`
- `idempotency_key`
- `schema_version`
- 最小必要 payload

邮件优先级：

- critical
- transactional
- digest

验证码不能被摘要队列阻塞。Worker 必须支持重试、退避、死信、重新投递和审计。

校园通知必须：

- 来源于公开白名单页面。
- 保留原文链接和发布时间。
- 标注非官方声明。
- 用户主动订阅，默认不发邮件。
- 支持退订。

## 12. QuizCraft 迁移规则

- 导入后先保持原目录和部署方式。
- 浏览器提交的 `user_id` 不是可信身份。
- 先增加服务端匿名会话和平台账号绑定。
- 先迁移读取接口，最后迁移作答写接口。
- 对题库、作答统计和排行榜做双读/双算一致性比较。
- 每个 API 有 provider 功能开关。
- 没有测试证据不得切生产流量。
- FastAPI 下线前保持至少两周零流量观察，并归档至少六个月。

## 13. 测试要求

开发必须编写单元测试。测试人员独立设计契约、集成、E2E、异常、安全、并发和发布验收。

### 必测

- OpenAPI 契约。
- 验证码过期、重放、限流。
- callback 和 `return_to` 白名单。
- state/PKCE。
- 授权码重复交换。
- 会话撤销。
- 服务间认证和密钥轮换。
- 事件重复投递。
- Outbox 一致性。
- Worker 重启和 Redis 暂时不可用。
- DirectMail 超时、拒收、重试和死信。
- 旧用户冲突绑定。
- QuizCraft 匿名用户绑定。
- 通知用户隔离。
- 积分幂等。
- 移动端、跨浏览器和键盘。
- 日志脱敏和密钥泄漏。
- 备份恢复和发布回滚。

### 测试数据

- 使用专用测试邮箱和 fixture。
- 不复制生产邮箱、Token、支付密钥或真实课程资料到 CI。
- 变更统计逻辑时保留固定快照和可重放输入。

## 14. CI

### Go

- gofmt
- go vet
- staticcheck
- go test
- go test -race
- govulncheck
- PostgreSQL/Redis integration
- migration upgrade/rollback
- Docker build
- 镜像漏洞扫描
- 密钥泄漏扫描

### Study

保留现有：

- API tests
- Worker tests
- Next.js build
- Vue build
- Compose config
- 资料、下载、支付和审核 smoke

不得为了平台重构删除已有安全测试。

### QuizCraft

- Python formatter/static checks（新增时明确标记）
- FastAPI tests/smoke
- frontend lint
- TypeScript
- build
- syntax tests
- Playwright
- PostgreSQL consistency
- Go adapter contract tests

### 路径过滤

只运行受影响部署单元，但共享 token、契约、基础设施和 Migration 变更触发所有消费方相关测试。每天或每晚运行一次全量回归。

## 15. CD

```text
CI 全通过
→ 构建 commit SHA 镜像
→ 推送镜像仓库
→ 测试环境
→ 向前兼容 Migration
→ Readiness
→ Smoke
→ 契约测试
→ E2E
→ 测试验收
→ 人工批准
→ 灰度生产
→ 生产 Smoke
→ 监控观察
→ 完成发布
```

- `main` 分支保护。
- 禁止直接 push。
- 生产人工批准。
- 破坏性 Migration 不自动执行。
- 部署失败停止继续放量。
- 回滚应用时不自动回滚已兼容的 expand migration。
- 发布前验证备份。

## 16. 结构图与文档导出

- 仓库 Markdown 中可使用 Mermaid。
- 导出 DOCX/PDF 时必须先渲染 Mermaid 为 SVG/PNG。
- 不允许把 `flowchart`、`sequenceDiagram` 或 `gantt` 源码作为最终图片替代品。
- 图中必须使用真实模块名称，不能写“平台处理”黑盒。
- 文档中的架构事实、目标方案和待验证推断必须分开。

## 17. 安全与日志

- JSON 结构化日志。
- 每个请求携带 request ID。
- 邮箱默认掩码。
- 不记录验证码、Authorization、Cookie、私钥、支付原始密文和完整第三方回调敏感体。
- 管理操作、账号绑定、服务密钥轮换、死信重投和支付处理写审计日志。
- 错误消息面向用户可读，但不暴露内部堆栈和路径。

## 18. 发布验收清单

合并前：

- [ ] 模块边界未被破坏。
- [ ] 没有新增第二套资料库或刷题能力。
- [ ] 非官方声明正确。
- [ ] Kit 墨绿命名和 token 正确。
- [ ] 移动端关键流程通过。
- [ ] Loading/Empty/Error/权限状态完整。
- [ ] API 契约和 Migration 已评审。
- [ ] 日志脱敏。
- [ ] 自动化和人工测试结果附在 PR。
- [ ] 发布和回滚步骤可执行。
- [ ] 安全关键变更由非作者验收。

## 19. 团队节奏

- 每周一次 30 分钟里程碑会，只讨论进度、风险和决策。
- 每个工作日异步更新：完成、下一步、阻塞。
- 每两周一个可演示增量。
- 每阶段最多一个主目标。
- 线上事故先恢复服务，再补 Issue、复盘和防复发措施。

## 20. 文档优先级

冲突时按以下顺序处理：

1. 安全、隐私和法律要求。
2. ADR 中已批准的架构决策。
3. `docs/product/PRODUCT_BOUNDARIES.md`。
4. `docs/product/DESIGN_SYSTEM.md`。
5. `docs/architecture/MONOREPO_ARCHITECTURE.md`。
6. 本开发规范。
7. 各应用和服务 README。

发现代码与文档不一致时，不以 README 自动判定代码错误；应创建 Issue，写明真实代码、运行数据和需要团队决策的差异。