---
status: accepted
---

# HENUKit Console 使用分层测试与真实依赖集成测试

Vue 组件与页面逻辑使用 Vitest 和 Vue Test Utils，关键浏览器流程使用 Playwright 覆盖桌面与 390px 移动端；Go 使用标准库 `testing`。PostgreSQL/Redis 集成测试通过 Testcontainers 启动真实临时依赖，不以 SQLite 替代 PostgreSQL；CI 同时执行 OpenAPI lint、正反例、生成一致性、breaking-change、版本化 Migration 和“Console Bundle 不含 Element Plus/旧路由”的边界测试。
