# HENUKitDev 文档中心

> 本目录按“当前执行规范、长期架构规范、运行维护文档、历史归档”分层。新开发者从本页进入，不再从散落在 `docs/` 根目录的旧文件开始阅读。

## 1. 新开发者入口

1. [`development/README.md`](./development/README.md)：开发实施文档索引。
2. [`development/implementation-plan.md`](./development/implementation-plan.md)：1–2 名开发、1–2 名测试的执行计划。
3. [`development/go-no-go-checklist.md`](./development/go-no-go-checklist.md)：启动决策与停止条件。
4. [`DEVELOPMENT.md`](./DEVELOPMENT.md)：Monorepo 基础开发规则与仓库约束。

## 2. 当前有效规范

### 开发实施

- [`development/`](./development/)：任务、API、数据迁移、安全、测试、发布和 GitHub Issues。

### 架构与决策

- [`architecture/MONOREPO_ARCHITECTURE.md`](./architecture/MONOREPO_ARCHITECTURE.md)
- [`architecture/ACCESS_CONTROL.md`](./architecture/ACCESS_CONTROL.md)
- [`adr/`](./adr/)

### 产品规范

- [`product/PRODUCT_BOUNDARIES.md`](./product/PRODUCT_BOUNDARIES.md)
- [`product/DESIGN_SYSTEM.md`](./product/DESIGN_SYSTEM.md)
- [`product/ROADMAP.md`](./product/ROADMAP.md)

### 迁移

- [`migrations/FINAL_REVIEW_TO_HENUKIT.md`](./migrations/FINAL_REVIEW_TO_HENUKIT.md)

### 运行维护

- [`operations/internal-smoke.md`](./operations/internal-smoke.md)
- [`operations/wechat-pay-native.md`](./operations/wechat-pay-native.md)

## 3. 历史文档

旧学习平台的架构、API、安全、开发说明、路线图和阶段总结统一放入 [`archive/legacy-platform/`](./archive/legacy-platform/)。这些文件用于追溯历史，不作为新功能实现依据。

公开 HENU-Kit 规划仓的导入快照位于仓库根目录 [`archive/henukit-planning/`](../archive/henukit-planning/)，不与当前开发规范混用。

## 4. 文档优先级

发生冲突时按以下顺序处理：

1. 已运行验证的代码、OpenAPI、Migration 和 CI；
2. `docs/development/` 下的当前执行规范；
3. `docs/architecture/`、`docs/product/`、`docs/adr/` 中的长期规范；
4. `docs/archive/` 与根目录 `archive/` 中的历史材料。

## 5. 维护规则

- 新执行文档进入 `docs/development/`。
- 长期架构约束进入 `docs/architecture/` 或 `docs/adr/`。
- 部署、Smoke、支付接入等操作说明进入 `docs/operations/`。
- 阶段总结和已替代文档进入 `docs/archive/`。
- 不在 `docs/` 根目录新增无分类文件。
