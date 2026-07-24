# HENUKitDev 文档中心

> 本目录按“当前执行规范、长期架构规范、运行维护文档、历史定位说明”分层。新开发者从本页进入，不再从散落或已移除的旧文件开始阅读。

## 1. 新开发者入口

1. [`../CONTEXT-MAP.md`](../CONTEXT-MAP.md)：先确定当前领域词汇、所有权和新旧系统边界。
2. [`operations/PRODUCTION_RELEASE_CHECKLIST.md`](./operations/PRODUCTION_RELEASE_CHECKLIST.md)：上线前唯一的 Go/No-Go 汇总检查表。
3. [`development/henukit-console-executable-spec.md`](./development/henukit-console-executable-spec.md)：HENUKit Console 与 QuizCraft 重构的可执行规格。
4. [`development/henukit-console-replacement-plan.md`](./development/henukit-console-replacement-plan.md)：已接受的替代实施计划、迁移顺序和 Gate。
5. [`adr/README.md`](./adr/README.md)：支撑当前替代方案的架构决策索引。
6. [`development/README.md`](./development/README.md)：其余开发实施文档索引。
7. [`development/go-no-go-checklist.md`](./development/go-no-go-checklist.md)：启动决策与停止条件。
8. [`DEVELOPMENT.md`](./DEVELOPMENT.md)：Monorepo Foundation 阶段形成的仓库级开发约束。

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

- [`operations/PRODUCTION_RELEASE_CHECKLIST.md`](./operations/PRODUCTION_RELEASE_CHECKLIST.md)
- [`operations/README.md`](./operations/README.md)
- [`operations/deployment.md`](./operations/deployment.md)
- [`operations/internal-smoke.md`](./operations/internal-smoke.md)
- [`operations/wechat-pay-native.md`](./operations/wechat-pay-native.md)

## 3. 历史材料策略

- [`archive/legacy-platform/`](./archive/legacy-platform/)：旧 Study V2 文档正文已从默认分支移除，只保留定位说明。
- [`archive/status/`](./archive/status/)：过期阶段总结正文已移除，只保留定位说明。
- 仓库根目录 [`archive/henukit-planning/`](../archive/henukit-planning/)：公开 `HENU-Kit` 重复快照已移除，只保留来源说明。
- 仓库根目录 [`legacy/v1-next-prisma/`](../legacy/v1-next-prisma/)：旧 V1 可执行源码已移除，只保留定位说明。

完整历史仍保存在 Git 提交历史中，可按清理提交父 SHA 检出；历史材料不作为新功能实现、发布完成或生产状态的依据。

## 4. 文档优先级

发生冲突时按以下顺序处理：

1. 安全、隐私和法律要求；
2. Accepted ADR；
3. 当前 Context 与 `docs/product/PRODUCT_BOUNDARIES.md`；
4. 明确标为当前的执行规格、替代计划和生产发布总检查表；
5. `docs/product/DESIGN_SYSTEM.md`；
6. `docs/architecture/MONOREPO_ARCHITECTURE.md`；
7. `docs/DEVELOPMENT.md` 与 `docs/development/` 下未被替代的规范；
8. 各应用、服务 README 和与当前运行单元匹配的 Runbook；
9. Git 历史中的已归档材料。

代码、OpenAPI、Migration、CI 或运行证据与上述决策不一致时，先记录真实状态并创建差异记录；不能用现有实现自动覆盖安全要求或 Accepted ADR，也不能把计划能力写成已部署能力。

## 5. 维护规则

- 新执行文档进入 `docs/development/`。
- 长期架构约束进入 `docs/architecture/` 或 `docs/adr/`。
- 产品边界、设计系统和资料格式进入 `docs/product/`。
- 部署、Smoke、支付接入和发布总门禁进入 `docs/operations/`。
- 不再向 `docs/archive/` 或根目录 `archive/` 提交重复的可执行源码、公开仓镜像或过期阶段结论正文。
- 不在 `docs/` 根目录新增无分类文件。