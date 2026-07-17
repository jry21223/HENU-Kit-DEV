# ADR-0001：将 final-review-platform 重构为 HENU Kit Monorepo

- 状态：Accepted
- 日期：2026-07-17
- 决策人：HENU Kit 维护团队

## 背景

当前存在三个事实来源：

1. `jry21223/HENU-Kit`：品牌、路线图和设计规范。
2. `jry21223/final-review-platform`：大型学习平台 V2，包含 Next.js、Vue、Go API、Go Worker、PostgreSQL、Redis 及大量业务能力。
3. `jry21223/quizcraft-cn`：独立 QuizCraft 刷题产品，使用 React/Vite、FastAPI 和 PostgreSQL。

跨仓库维护导致产品边界、账户、API、设计系统、CI 和发布流程容易分叉。用户已明确最终开发仓库落在当前 `final-review-platform`，并将其重构为包含所有产品代码的 HENU Kit monorepo。

## 决策

- 当前 `final-review-platform` 成为最终 HENU Kit 开发仓库。
- 完成基础迁移后，仓库改名为 `HENU-Kit`。
- `quizcraft-cn` 以 subtree 方式导入 `products/quizcraft`，第一阶段保持原技术栈和部署方式。
- 原 `HENU-Kit` 规划仓库以 subtree 方式归档到 `archive/henukit-planning`，其规范迁入根 `docs/` 并成为唯一事实来源。
- 当前学习平台代码在迁移期保持原路径和可部署性，逐步收敛为资料库。
- 新平台核心使用 Go 模块化单体，但不把当前混合业务 `services/api` 直接改名为平台核心。
- 子产品独立部署，monorepo 不等于单体运行时。

## 产品边界

- HENU Kit 主站：统一品牌、入口、导航、账户状态和跨产品体验。
- 资料库：只负责资料，不再开发第二套刷题。
- QuizCraft：唯一刷题产品。
- 平台核心：统一账户、邮件、通知、事件、统计、服务间认证和 API 契约。

## 方案比较

### 保持多仓库

优点：改动最小。  
缺点：共享规范和跨产品变更持续分叉，不满足最终仓库要求。  
结论：不采用。

### 一次性把所有代码移动到最终目录并统一技术栈

优点：表面结构立即整齐。  
缺点：同时改变路径、构建、部署、认证、数据库和运行时，风险不可控。  
结论：不采用。

### Monorepo + 渐进迁移

优点：先统一事实来源和评审，再按部署单元迁移；可保留旧系统和回滚。  
缺点：迁移期存在临时目录和兼容层。  
结论：采用。

## 后果

### 正面

- 产品和架构规范只有一份。
- 跨产品变更可在一个 PR 中评审。
- 设计 token、OpenAPI 和测试 fixture 可统一维护。
- CI/CD 可按路径触发并运行全量夜间回归。
- 旧学习平台和 QuizCraft 可继续独立部署。

### 代价

- 仓库体积和 CI 复杂度增加。
- 迁移期同时存在旧路径和目标路径。
- 需要严格 CODEOWNERS、路径过滤和发布单元边界。
- subtree 更新需要固定流程和来源记录。

## 约束

- 不在导入 PR 中切生产流量。
- 不在目录移动 PR 中同时修改业务语义。
- 不因 Go 统一目标一次性重写 FastAPI。
- 不删除现有支付、资料下载和安全测试。
- 不把业务站数据库合并为平台核心数据库。
- 不通过跨主域共享 Cookie 实现统一登录。

## 复审条件

以下情况需要更新 ADR：

- 决定不再使用当前仓库作为最终仓库。
- 采用不同的 QuizCraft 导入或历史保留方式。
- 平台核心从模块化单体改为微服务。
- 子产品部署方式不再独立。
- 数据 Owner 发生重大变化。