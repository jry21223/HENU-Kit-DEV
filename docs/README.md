# HENUKitDev 文档中心

> 本目录按“当前执行规范、长期架构规范、运行维护文档、历史归档”分层。新开发者从本页进入，不再从散落在 `docs/` 根目录的旧文件开始阅读。

## 1. 新开发者入口

1. [`../CONTEXT-MAP.md`](../CONTEXT-MAP.md)：先确定当前领域词汇、所有权和新旧系统边界。
2. [`development/henukit-console-executable-spec.md`](./development/henukit-console-executable-spec.md)：HENUKit Console 与 QuizCraft 重构的可执行规格。
3. [`development/henukit-console-replacement-plan.md`](./development/henukit-console-replacement-plan.md)：已接受的替代实施计划、迁移顺序和 Gate。
4. [`adr/README.md`](./adr/README.md)：支撑当前替代方案的架构决策索引。
5. [`development/README.md`](./development/README.md)：其余开发实施文档索引。
6. [`development/go-no-go-checklist.md`](./development/go-no-go-checklist.md)：启动决策与停止条件。
7. [`DEVELOPMENT.md`](./DEVELOPMENT.md)：Monorepo Foundation 阶段形成的仓库级开发约束。

## 2. 当前有效规范

### 开发实施

- [`development/henukit-console-executable-spec.md`](./development/henukit-console-executable-spec.md)：当前 HENUKit Console 与 QuizCraft 重构规格。
- [`development/henukit-console-replacement-plan.md`](./development/henukit-console-replacement-plan.md)：对应的替代计划和 PR 拆分顺序。
- [`development/`](./development/)：未被替代部分的任务、API、数据迁移、安全、测试、发布和 GitHub Issues。

### 架构与决策

- [`architecture/MONOREPO_ARCHITECTURE.md`](./architecture/MONOREPO_ARCHITECTURE.md)
- [`architecture/ACCESS_CONTROL.md`](./architecture/ACCESS_CONTROL.md)
- [`adr/README.md`](./adr/README.md)

### 产品规范

- [`product/PRODUCT_BOUNDARIES.md`](./product/PRODUCT_BOUNDARIES.md)
- [`product/DESIGN_SYSTEM.md`](./product/DESIGN_SYSTEM.md)
- [`product/ROADMAP.md`](./product/ROADMAP.md)
- [`product/material-library-format.md`](./product/material-library-format.md)

### 迁移

- [`migrations/FINAL_REVIEW_TO_HENUKIT.md`](./migrations/FINAL_REVIEW_TO_HENUKIT.md)

### 运行维护

- [`operations/README.md`](./operations/README.md)
- [`operations/deployment.md`](./operations/deployment.md)
- [`operations/internal-smoke.md`](./operations/internal-smoke.md)
- [`operations/wechat-pay-native.md`](./operations/wechat-pay-native.md)

## 3. 历史文档

- [`archive/legacy-platform/`](./archive/legacy-platform/)：旧 Study V2 的产品语境、开发、架构、API、数据库、管理后台、安全、UI 设计记录和路线图。
- [`archive/status/`](./archive/status/)：带日期的阶段总结和当时的验收结论。

这些文件用于追溯历史，不作为新功能实现依据。

公开 HENU-Kit 规划仓的导入快照位于仓库根目录 [`archive/henukit-planning/`](../archive/henukit-planning/)，不与当前开发规范混用。

## 4. 文档优先级

发生冲突时按以下顺序处理：

1. 已运行验证的代码、OpenAPI、Migration 和 CI；
2. 已接受且明确标为当前的执行规格与替代计划；
3. Accepted ADR 与当前 Context；
4. `docs/development/` 下未被替代的执行规范；
5. `docs/architecture/`、`docs/product/` 中未被 ADR 替代的长期规范；
6. `docs/operations/` 中与当前运行单元匹配的 Runbook；
7. `docs/archive/` 与根目录 `archive/` 中的历史材料。

## 5. 维护规则

- 新执行文档进入 `docs/development/`。
- 长期架构约束进入 `docs/architecture/` 或 `docs/adr/`。
- 产品边界、设计系统和资料格式进入 `docs/product/`。
- 部署、Smoke、支付接入等操作说明进入 `docs/operations/`。
- 阶段总结和已替代文档进入 `docs/archive/`。
- 不在 `docs/` 根目录新增无分类文件。
