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

## 使用规则

- ADR 的状态为 Accepted 时，实现不得静默偏离；如需变更，新增或修订 ADR 并明确替代关系。
- ADR 描述目标决策，不代表对应能力已经部署。实际状态仍以经过验证的代码、契约、Migration 和运行证据为准。
- Context 负责统一领域语言，ADR 负责解释架构选择，执行规格负责定义可验收行为；三者发生表述差异时应先消除差异再实现。
