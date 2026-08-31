# Architecture Decision Records

本目录记录 HENUKitDev 当前有效的仓库级架构决策。实现 HENUKit Console、Platform Core、Console Gateway 或 QuizCraft 重构前，应先阅读根目录 [`CONTEXT-MAP.md`](../../CONTEXT-MAP.md)、相关 Context，再阅读下列 Accepted ADR。

## 当前决策

1. [`0001-henukit-monorepo.md`](./0001-henukit-monorepo.md) — 开发 Monorepo 与独立部署边界。
2. [`0002-separate-henukit-console-from-study-legacy-admin.md`](./0002-separate-henukit-console-from-study-legacy-admin.md) — HENUKit Console 与 Study Legacy Admin 硬隔离。
3. [`0003-vue-vite-for-henukit-console.md`](./0003-vue-vite-for-henukit-console.md) — Console 使用 Vue 3、TypeScript 和 Vite。
4. [`0004-shadcn-vue-for-henukit-console.md`](./0004-shadcn-vue-for-henukit-console.md) — Console 组件、样式、图表和前端状态方案。
5. [`0005-go-console-gateway.md`](./0005-go-console-gateway.md) — 独立 Go Console Gateway。
6. [`0006-go-platform-core.md`](./0006-go-platform-core.md) — 独立 Go Platform Core。
7. [`0007-postgres-source-of-truth-and-redis-coordination.md`](./0007-postgres-source-of-truth-and-redis-coordination.md) — PostgreSQL 持久化与 Redis 短期协调。
8. [`0008-explicit-sql-with-pgx-and-sqlc.md`](./0008-explicit-sql-with-pgx-and-sqlc.md) — 显式 SQL、pgx、sqlc 和版本化 Migration。
9. [`0009-openapi-first-service-contracts.md`](./0009-openapi-first-service-contracts.md) — OpenAPI-first 服务集成。
10. [`0010-console-testing-stack.md`](./0010-console-testing-stack.md) — Console 分层测试与真实依赖测试。
11. [`0011-pnpm-for-frontend-workspaces.md`](./0011-pnpm-for-frontend-workspaces.md) — pnpm 前端 Workspace。
12. [`0012-quizcraft-react-frontend-go-backend.md`](./0012-quizcraft-react-frontend-go-backend.md) — QuizCraft React 前端与 Go 后端重构。
13. [`0013-quizcraft-maintenance-window-full-cutover.md`](./0013-quizcraft-maintenance-window-full-cutover.md) — 无人使用维护期内执行技术停写和一次性全量切换。
14. [`0014-account-center-registration-and-password-credentials.md`](./0014-account-center-registration-and-password-credentials.md) — Account Center 注册、密码凭证与首次自动登录边界。
15. [`0015-authentication-cookie-and-trusted-proxy-boundary.md`](./0015-authentication-cookie-and-trusted-proxy-boundary.md) — 认证 Cookie 与可信 HTTPS 代理边界。
16. [`0016-account-portfolio-owner.md`](./0016-account-portfolio-owner.md) — Account Portfolio 持久化账户事实边界。
17. [`0017-account-portfolio-user-commands-through-portal-gateway.md`](./0017-account-portfolio-user-commands-through-portal-gateway.md) — Portal Gateway 的受限账户自助写入例外。
18. [`0018-portal-quizcraft-practice-command-boundary.md`](./0018-portal-quizcraft-practice-command-boundary.md) — Portal Gateway 的两条默认关闭 QuizCraft Practice 命令边界。
19. [`0019-membership-order-purchase-surface.md`](./0019-membership-order-purchase-surface.md) — 用户自建会员订单的 Portal 写入例外与不泄漏商户订单号的收银台句柄。
20. [`0020-library-owner-production-onboard.md`](./0020-library-owner-production-onboard.md) — Library 独立数据所有者服务接入 Console 的生产上线边界。
21. [`0021-portal-favorites-write-boundary.md`](./0021-portal-favorites-write-boundary.md) — Portal Gateway 的三条默认关闭收藏写命令边界。
22. [`0022-materials-candidate-preparation.md`](./0022-materials-candidate-preparation.md) — 固定 SHA、非 root 的资料候选准备与派生目录预检边界。
23. [`0023-materials-latest-arrival-queue.md`](./0023-materials-latest-arrival-queue.md) — 资料候选准备的实例隔离、最近验收事件队列与同一无特权身份边界。
24. [`0024-materials-sealed-release-boundary.md`](./0024-materials-sealed-release-boundary.md) — root-owned、非激活的资料来源封存与可审计 release identity 边界。
25. [`0025-materials-atomic-activation.md`](./0025-materials-atomic-activation.md) — 单一 webhook、特权 runner、维护围栏、恢复 journal 与显式回滚边界。
26. [`0026-materials-oss-canary-publication.md`](./0026-materials-oss-canary-publication.md) — 单份封存审核资料到私有 OSS、可重放且不激活的 canary 发布边界。
27. [`0027-library-public-download-owner.md`](./0027-library-public-download-owner.md) — Library 对活动公开免费资料、短期下载 grant、下载开始账本与聚合的独立 owner 边界。
28. [`0028-materials-complete-oss-release.md`](./0028-materials-complete-oss-release.md) — 完整审核资料的 OSS exact-version commit、跨静态/派生/Library owner 激活与单调恢复边界。
29. [`0029-library-oss-owner-atomic-activation.md`](./0029-library-oss-owner-atomic-activation.md) — Library 对完整 OSS owner catalog 的验证、原子激活、聚合统计与前向回滚边界。
30. [`0030-degraded-baseline-production-recovery.md`](./0030-degraded-baseline-production-recovery.md) — 无健康回滚基线时的显式、精确 SHA 生产恢复边界。
31. [`0031-materials-oss-download-only.md`](./0031-materials-oss-download-only.md) — 资料原文件只经 Library owner 的短期 OSS grant 下载，在线预览及派生转换保持关闭。
32. [`0032-food-owns-post-creation-and-reads.md`](./0032-food-owns-post-creation-and-reads.md) — Food 服务独立拥有 Food Post 创建与公开读，Portal Gateway 第三条默认只读代理例外与立即公开语义。
33. [`0033-food-posts-mcp.md`](./0033-food-posts-mcp.md) — Food Post 投稿能力封装为远程 Streamable HTTP MCP 服务，调用方自报 actor，Food 仍是唯一数据与策略所有者。
34. [`0034-career-resume-mcp.md`](./0034-career-resume-mcp.md) — 简历上传 AI 提取能力封装为远程 Streamable HTTP MCP 服务，6 行 actor 绑定签名，调用方自报 actor 且不校验会员，Career 仍是唯一数据与策略所有者。
35. [`0036-portal-practice-read-path-owner-go-core.md`](./0036-portal-practice-read-path-owner-go-core.md) — Portal `/practice` 读路径收敛为 Gateway 精确路由 → QuizCraft Go core 契约读，portal-api 直读降级并移除（amends 0013/0018）。
36. [`0037-library-legacy-adapter-removed.md`](./0037-library-legacy-adapter-removed.md) — Library 移除 Study Legacy API 适配层与 fail-closed 启动依赖，命令路由诚实 503，数据迁移（T1）后恢复自有目录数据（supersedes 0020）。
37. [`0038-ranking-reuses-platform-identity.md`](./0038-ranking-reuses-platform-identity.md) — 排行身份复用平台 users.display_name：废除 Ranking Profile 机制，Gateway 经 platform-core display-names 批量接口实时解析，无显示名/游客渲染「游客x」稳定编号，system_avatar 哈希派生，对外永不出现 user_id（amends 0036）。
38. [`0039-library-canonical-material-types-and-owner-only-activation.md`](./0039-library-canonical-material-types-and-owner-only-activation.md) — 对齐公开资料 canonical 类型、教材授权、OSS 证据与 Library-only 激活边界。
39. [`0040-career-resume-suification-command.md`](./0040-career-resume-suification-command.md) — Portal Gateway 的单条可撤销 Career Resume Suification 命令、幂等重放与独立明文传输门禁。
40. [`0041-root-owned-actions-release-extraction.md`](./0041-root-owned-actions-release-extraction.md) — Actions runtime 解包统一 root 所有权，并以显式历史 UID 审计接管一次健康回滚基线（不放宽 ADR-0030）。
41. [`0042-getwork-mcp-as-career-job-source.md`](./0042-getwork-mcp-as-career-job-source.md) — 直接运行已授权且锁定版本的 getWork MCP，Career 仅通过官方 SDK 调用岗位工具，继续自有匹配、持久化和邮件链。
42. [`0043-getwork-mcp-remote-execution-over-ssh.md`](./0043-getwork-mcp-remote-execution-over-ssh.md) — 浏览器型 getWork MCP 仅在常久在线 WSL2 执行，通过受限 SSH 隧道接入生产内网 relay。
43. [`0044-actions-degraded-baseline-recovery.md`](./0044-actions-degraded-baseline-recovery.md) — 允许经显式双 SHA 授权的最新成功 current-main Actions 制品复用 ADR-0030 降级基线恢复契约。

## 使用规则

- ADR 的状态为 Accepted 时，实现不得静默偏离；如需变更，新增或修订 ADR 并明确替代关系。
- ADR 描述目标决策，不代表对应能力已经部署。实际状态仍以经过验证的代码、契约、Migration 和运行证据为准。
- Context 负责统一领域语言，ADR 负责解释架构选择，执行规格负责定义可验收行为；三者发生表述差异时应先消除差异再实现。
