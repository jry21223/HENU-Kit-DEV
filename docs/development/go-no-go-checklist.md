# 启动决策与停止条件

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 启动前必须拍板

| 决策 | 推荐默认值 | 最晚时间 |
|---|---|---|
| 开发仓最终名称 | `final-review-platform -> HENUKitDev`；公开 `HENU-Kit` 不改名 | Phase 6 前 |
| Foundation 合并门槛 | Study/Quiz 全量基线 + 导入 Hash/License + 不触发旧部署 | W1 |
| Quiz 旧仓冻结点 | 统一身份和 Monorepo Deploy 完成后只读 | Phase 4 末 |
| 公开仓同步 | 公开仓只同步项目入口和公开路线；实现/内部文档留开发仓 | Phase 1 |
| Turborepo/Nx | 首期不引入；Workspace + Script + Path Filter | Phase 1 |
| 正式开始日和每日投入 | 启动会上写入项目看板 | 启动前 |
| 学生邮箱域名 | 核实允许域名，不凭记忆上线 | Phase 1 末 |
| DirectMail | 独立发信域和最小权限凭据 | Phase 1 末 |
| 旧 Quiz 用户认领 | 无签名持有证明不允许自助认领 | Phase 4 前 |
| 社区/支付/泛 AI | 首期前台隐藏冻结，数据保留 | Phase 0 |
| Free/VIP 权益 | 只区分内容、统计和额度，不绑定管理权限 | Phase 1 末 |
| Scope 层级 | Platform/Product/Course/Bank | Phase 1 末 |
| 匿名数据保留 | 建议 90 天，最终需治理确认 | Phase 2 前 |
| Course ID Owner | Study Course UUID；Quiz 保存显式映射 | Phase 0 |
| RPO/RTO | 建议 RPO≤24h、RTO≤2h | CD 设计前 |

## 2. 第一周行动清单

### 周一

- 开发 A：主仓 Main、线上 SHA、数据库只读盘点。
- 开发 B：Foundation 文件、导入来源和 Deploy Trigger 盘点。
- 测试 A：Quiz 导入 Hash、License 和测试清单。
- 测试 B：Actions、Secrets 引用、服务器 Remote 和仓名硬编码清单。
- 日终：单仓迁移 Inventory。

### 周二

- 开发 A：Study 模块、表、数据量和公共能力依赖图。
- 开发 B：修正 Path Filter，准备全量 Build。
- 测试 A：Study Go/Node/Compose 基线。
- 测试 B：Quiz Pytest/Smoke/Lint/Build 基线。
- 日终：Foundation 合并报告 v1。

### 周三

- 开发 A：Platform Core 边界、OpenAPI 和 Migration ADR。
- 开发 B：Portal Workspace 方案。
- 测试 A：账号/事件契约测试清单。
- 测试 B：Portal/Study/Quiz 浏览器基线。
- 日终：架构与测试评审材料。

### 周四

- 全员：数据 Owner Workshop。
- 开发 B：Quiz Freeze/热修同步和公开仓同步 Runbook。
- 测试：审查失败、补偿、回滚和改名引用扫描方案。
- 日终：签字版模块边界与同步策略。

### 周五

- 拆分 Platform/Auth/Event、Portal/CI/Study/Quiz Issues。
- 测试完成 Phase 0 验收。
- 输出 Foundation PR 发布/回滚演练方案。
- 决定 PR #10 是否可合并。

## 3. 第一周禁止事项

- 移动 Study 目录。
- 修改生产数据库或执行破坏性 Migration。
- 修改开发仓名称、归档公开 `HENU-Kit` 或切换 DNS。
- 重写 FastAPI 或删除历史模块。
- 让纯 Foundation 变更自动部署生产。
- 在未盘点数据前承诺删除社区、支付、积分、AI 或刷题表。

## 4. 停止条件

出现以下任一情况，立即停止评审、合并或发布：

- 提议一次 PR 完成目录移动、数据库迁移和流量切换，且无分阶段测试和回滚。
- 未完成生产数据盘点就删除旧表、路由或账户数据。
- 未完成版本化 Migration 和恢复演练就做破坏性 Schema 变更。
- 未完成验证码重放、限流、Callback、State 和 PKCE 测试就开放生产账号流量。
- 未完成 Quiz 判分和统计一致性验证就切写流量。
- 日志、监控或错误响应出现完整邮箱、验证码、Token、Cookie 或密钥。
- Quiz 旧仓冻结、导入 Hash、Actions/Secrets/Deploy Key/Remote 清单未完成就改名。
- 新系统无法在 Feature Flag 下回到旧 Provider。

## 5. 每阶段 Go/No-Go 会议

会议只回答四个问题：

1. 当前退出条件是否有证据，而不是口头“应该没问题”？
2. 数据和安全风险是否由非作者验收？
3. 回滚是否演练过，预计多久恢复？
4. 失败时是否会损坏旧系统或阻断用户？

任一答案不明确，结论为 No-Go。
