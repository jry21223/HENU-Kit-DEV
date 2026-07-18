# HENUKitDev 文档中心

> 这是仓库文档的唯一入口。开发人员先从“开发实施”选择任务，再按需查看长期参考、运行维护或历史归档。

## 1. 开发实施

面向 1–2 名开发、1–2 名测试，按编号阅读和执行：

| 顺序 | 模块 | 什么时候看 |
|---:|---|---|
| 01 | [`开发实施计划`](./development/01-开发实施计划.md) | 确认范围、人员、阶段、工期和关键路径 |
| 02 | [`架构与模块边界`](./development/02-架构与模块边界.md) | 确认模块职责、数据 Owner、依赖方向和迁移顺序 |
| 03 | [`API 与服务通信`](./development/03-API与服务通信.md) | 设计或评审 REST、OpenAPI、幂等、签名、事件和接口 |
| 04 | [`数据库与数据迁移`](./development/04-数据库与数据迁移.md) | 设计 Schema、Migration、事务、锁和旧数据迁移 |
| 05 | [`身份权限与安全`](./development/05-身份权限与安全.md) | 处理登录、会话、角色、会员、服务凭据和日志脱敏 |
| 06 | [`工程协作与发布`](./development/06-工程协作与发布.md) | 开分支、提 PR、配置 CI、发布、灰度或回滚 |
| 07 | [`测试与验收`](./development/07-测试与验收.md) | 编写测试、执行阶段门禁和形成验收结论 |
| 08 | [`GitHub 任务清单`](./development/08-GitHub任务清单.md) | 认领任务、拆 Issue、确认依赖和验收条件 |
| 09 | [`启动与停止条件`](./development/09-启动与停止条件.md) | 项目启动、阶段 Go/No-Go 和风险止损 |

来源与原计划章节对应关系见 [`来源映射`](./development/来源映射.md)。

推荐阅读顺序：

```text
01 实施计划
→ 02 架构边界
→ 按任务阅读 03 API / 04 数据库 / 05 安全
→ 06 工程发布 + 07 测试验收
→ 08 建立任务
→ 09 做阶段决策
```

## 2. 长期参考

完整索引见 [`reference/README.md`](./reference/README.md)。这些文档定义长期产品和架构约束，不作为每日任务清单：

- [`reference/architecture/MONOREPO_ARCHITECTURE.md`](./reference/architecture/MONOREPO_ARCHITECTURE.md)：Monorepo 与运行时架构。
- [`reference/architecture/ACCESS_CONTROL.md`](./reference/architecture/ACCESS_CONTROL.md)：角色、会员和 entitlement 模型。
- [`reference/product/PRODUCT_BOUNDARIES.md`](./reference/product/PRODUCT_BOUNDARIES.md)：产品职责与数据边界。
- [`reference/product/DESIGN_SYSTEM.md`](./reference/product/DESIGN_SYSTEM.md)：设计系统与交互约束。
- [`reference/product/ROADMAP.md`](./reference/product/ROADMAP.md)：当前产品路线图。
- [`reference/product/material-library-format.md`](./reference/product/material-library-format.md)：资料库文件格式。
- [`reference/migrations/FINAL_REVIEW_TO_HENUKIT.md`](./reference/migrations/FINAL_REVIEW_TO_HENUKIT.md)：迁移基线。
- [`reference/adr/`](./reference/adr/)：已批准的架构决策。

## 3. 运行维护

- [`operations/README.md`](./operations/README.md)：运行维护入口。
- [`operations/deployment.md`](./operations/deployment.md)：部署与上线检查。
- [`operations/internal-smoke.md`](./operations/internal-smoke.md)：内部 Smoke Runbook。
- [`operations/wechat-pay-native.md`](./operations/wechat-pay-native.md)：微信支付 Native 联调记录。

## 4. 历史归档

完整索引见 [`archive/README.md`](./archive/README.md)。

- [`archive/foundation/`](./archive/foundation/)：已被模块化实施文档取代的 Foundation 开发总规范。
- [`archive/legacy-platform/`](./archive/legacy-platform/)：旧 Study V2 的产品、技术和 UI 资料。
- [`archive/status/`](./archive/status/)：带日期的阶段总结和当时结论。
- [`../archive/henukit-planning/`](../archive/henukit-planning/)：公开 HENU-Kit 规划仓的导入快照。

归档仅用于追溯，不用于直接启动新任务。

## 5. 文档优先级

发生冲突时依次采用：

1. 已运行验证的代码、OpenAPI、Migration 和 CI；
2. `docs/development/` 下编号实施文档；
3. `docs/reference/` 下长期规范与 ADR；
4. 与当前运行单元匹配的 `docs/operations/` Runbook；
5. `docs/archive/` 与根目录 `archive/` 中的历史材料。

## 6. 维护规则

- 新开发任务必须落到 01–09 中对应模块，避免再创建平行总规范。
- 长期架构、产品或迁移约束进入 `reference/`。
- 部署、Smoke 和故障处理进入 `operations/`。
- 已替代内容进入 `archive/`，并在归档入口说明替代关系。
- `docs/` 根目录只保留本入口文件。
