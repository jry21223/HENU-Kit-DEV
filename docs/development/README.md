# HENUKitDev 开发实施文档包 V1.0

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。

## 1. 这套文档解决什么问题

原 V2.1 计划同时包含仓库审计、产品边界、架构、API、数据模型、时序、测试矩阵、甘特图和 60 余个 Issue，信息完整，但不适合作为开发人员每天直接执行的手册。本包将其拆为九个互相引用、职责明确的文档，并把重复任务合并为 56 个可执行任务，其中 3 个校园通知任务为可选项。

## 2. 文档目录

| 文件 | 使用者 | 用途 |
|---|---|---|
| [`implementation-plan.md`](./implementation-plan.md) | 全员 | 18–32 周排期、人员分工、阶段门槛、交付物 |
| [`architecture-boundaries.md`](./architecture-boundaries.md) | 开发、架构评审 | 仓库目录、产品边界、数据 Owner、依赖禁区 |
| [`api-communication-spec.md`](./api-communication-spec.md) | 前后端、测试 | REST/OpenAPI、响应、幂等、签名、事件和接口清单 |
| [`database-migration-spec.md`](./database-migration-spec.md) | 后端、测试、运维 | 数据库隔离、Migration、事务、旧用户与题库数据迁移 |
| [`identity-security-spec.md`](./identity-security-spec.md) | 后端、前端、测试 | Guest/Free/VIP/角色、登录、会话、密钥和脱敏要求 |
| [`engineering-release-spec.md`](./engineering-release-spec.md) | 开发、测试、运维 | 分支、PR、CI、镜像、灰度、回滚、目录移动和仓库改名 |
| [`testing-acceptance-spec.md`](./testing-acceptance-spec.md) | 测试、开发 | 分层测试、关键用例、发布门禁和验收报告 |
| [`github-issues.md`](./github-issues.md) | 项目负责人、开发、测试 | 可直接录入项目看板的任务、依赖、工时和验收条件 |
| [`go-no-go-checklist.md`](./go-no-go-checklist.md) | 项目负责人 | 启动前决策、第一周行动、禁止事项和停止线 |

仓库以拆分文件为准，便于按模块评审、认领和持续维护。

## 3. 默认执行口径

- 当前开发主仓：`jry21223/final-review-platform`；全部迁移验收后改名为 `jry21223/HENUKitDev`。
- 公开仓：`jry21223/HENU-Kit` 保持原名，只承担公开介绍、项目索引、路线图和社区信息。
- Quiz 旧仓：`jry21223/quizcraft-cn` 在冻结点前保留生产来源，之后只读并归档。
- 推荐团队：2 名开发 + 1 名测试，约 21 周；增加第 2 名测试可压缩为约 18 周。
- 第一版范围：Monorepo 基础、Portal、Platform Core、统一账号、事件/邮件、QuizCraft 接入、Study 收敛、独立部署和回滚。
- 非首发范围：校园通知抓取、社区复活、支付重构、泛 AI 功能、一次性 Go 重写 QuizCraft。

## 4. 对原计划的执行性修订

1. **修正仓库改名冲突。** 原计划主体已明确公开 `HENU-Kit` 不改名、开发主仓最终改为 `HENUKitDev`；甘特图中残留的“旧 HENU-Kit 改名归档 / 主仓改名为 HENU-Kit”不执行。
2. **合并重复任务。** `MONO-006` 与 `CORE-002` 合并为 Platform Core/Worker 骨架；`MONO-004`、`CI-001`、`CI-002` 合并为路径过滤 PR CI 工作流。
3. **测试不后置。** 每个阶段从第一天建立测试数据、Mock、契约测试和回滚证据；测试人员不只是最终验收。
4. **可选功能退出首发关键路径。** 校园通知属于可选 Epic，不阻塞统一账号、Study/Quiz 和主站首发。
5. **目录移动与仓库改名后置。** 业务行为、数据迁移和部署兼容稳定后才移动目录；改名必须使用单独发布窗口。

## 5. 文档优先级

发生冲突时依次采用：

1. 已合并并运行验证的 OpenAPI、Migration、CI 和代码；
2. 本文档包中的执行规范；
3. 原 V2.1 计划的详细审计和证据；
4. 旧 README 或历史规划声明。

## 6. 来源映射

见 [`source-mapping.md`](./source-mapping.md)。
