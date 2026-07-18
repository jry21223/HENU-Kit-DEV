# 工程协作、CI/CD 与发布规范

> 来源：`HENUKitDev-Monorepo-重构与渐进迁移开发计划-V2.1`。本文件是面向 1–2 名开发、1–2 名测试的执行抽取版。  
> 原计划保留为审计与证据文档；本文件用于日常开发、评审、测试和发布。  
> 固定原则：`expand -> migrate -> contract`；业务变化、数据库迁移、目录移动、域名切换和仓库改名不得合并为一次大改动。


## 1. 分支与 PR

- `main` 为受保护稳定分支，禁止直接 Push。
- 小团队使用短生命周期分支：`feature/<area>/<issue-id>`、`fix/<area>/<issue-id>`。
- 每个 Issue 0.5–2 个工作日；超过 2 日必须拆分。
- 一个 PR 只做一类变化：业务行为、Migration、目录移动、部署变更和仓库改名必须分开。
- Draft PR 可跳过生产批准，不能跳过 Repo Governance 和受影响模块 CI。
- 安全关键和数据迁移 PR 必须由非作者开发 + 测试人员 Review。

## 2. 代码归属

CODEOWNERS 至少覆盖：

```text
/apps/portal/                 area:portal
/apps/study-*/                area:study
/apps/quiz-web/               area:quiz
/services/platform-*/        area:platform
/services/study-*/           area:study
/services/quiz-api-legacy/   area:quiz
/packages/api-contracts/     area:contracts
/packages/event-schemas/     area:contracts
/infra/                      area:infra
```

公共文件可修改，但 PR 必须说明影响的所有消费者，并触发 Fan-out CI。

## 3. PR 必填内容

- 关联 Issue、目标、明确不做。
- 允许修改路径和实际修改路径。
- API/Event/Migration 变化。
- 正常、失败、重复、并发和权限测试结果。
- Feature Flag、发布顺序、监控和回滚命令。
- 截图或 E2E 证据（前端变化）。
- 数据对账结果（Migration/双写/影子流量）。

## 4. 路径过滤 CI

| Job | 触发路径 | 必须执行 |
|---|---|---|
| `repo-governance` | 全部 PR | 模块清单、License、Secret、大文件、导入来源 |
| `contracts` | `packages/api-contracts/**`、`event-schemas/**` | OpenAPI/JSON Schema Lint、生成代码一致性 |
| `portal` | `apps/portal/**`、Design Tokens | Install、Type、Lint、Build、Playwright Smoke |
| `study-web-admin` | Study Web/Admin | Next/Vue Build、Type、Lint、关键 E2E |
| `study-go` | Study API/Worker | gofmt、Vet、Staticcheck、Test、Race、Integration |
| `quiz-python` | Quiz FastAPI | Format/Lint、Pytest、Smoke、PostgreSQL Consistency |
| `quiz-web` | Quiz Web | Lint、TypeScript、Syntax、Build、Playwright |
| `platform-core` | Platform Core/Worker | Go 全量 CI、Migration、PG/Redis Integration、Docker/Scan |
| `compose` | Infra、Docker、Env Example | Compose Config、Health/Readiness Smoke |

共享包改动必须触发所有消费方，不能只运行 Contracts Job。

## 5. 强制检查

### Go

`gofmt`、`go vet`、`staticcheck`、`go test ./...`、`go test -race ./...`、`govulncheck`、Migration Up/Down、PostgreSQL/Redis Integration、Docker Build、镜像扫描。

### Study

保留现有 API/Worker Tests、Next/Vue Build、Compose Config，以及资料下载、水印、审核、支付安全 Smoke。前台隐藏不等于删除安全测试。

### QuizCraft

Python 格式/静态检查、Pytest、FastAPI Smoke、React Lint/Type/Build、现有 Syntax Tests、Playwright、PostgreSQL Consistency、Adapter Contract。

## 6. Artifact 与版本

同一 Commit SHA 可构建：

```text
henukit-portal:<sha>
henukit-platform-core:<sha>
henukit-platform-worker:<sha>
henukit-study-web:<sha>
henukit-study-admin:<sha>
henukit-study-api:<sha>
henukit-study-worker:<sha>
henukit-quiz-web:<sha>
henukit-quiz-api-legacy:<sha>
```

发布清单必须记录每个 Deploy Unit 的实际 SHA；未变化单元可复用旧镜像，但不能用模糊 `latest` 代替可追踪版本。

## 7. 发布流程

```text
PR CI 全通过
→ 合并 main
→ 按路径构建 SHA Artifact
→ 推送镜像
→ 部署 Staging 受影响单元
→ 执行向前兼容 Migration
→ Readiness / Smoke / Contract / E2E
→ 测试签字
→ 人工批准
→ 单 Deploy Unit 灰度
→ 生产 Smoke 和指标观察
→ 完成发布
```

任何写流量切换必须有 Feature Flag 和可回切旧 Provider 的开关。

## 8. 回滚触发

| 指标 | 动作 |
|---|---|
| Readiness 失败或 5xx >1% 持续 5 分钟 | 停止放量，回滚对应 Deploy Unit |
| 登录成功率比基线下降 >5 个百分点 | 回切旧 Auth Adapter |
| Quiz 任一判分差异、重复计数或写失败率 >0.5% | 回切 Legacy Provider |
| Study 下载失败/权限异常明显上升 | 回切旧 Study Artifact，不回滚兼容 Migration |
| Critical Mail 最老任务 >5 分钟或连续凭据错误 | 停止相关流量，修复 Mail Worker/凭据 |
| 仓库改名后 Actions、Deploy Key、Remote 任一不可用 | 暂停全部发布，按 Runbook 恢复或修复 |

## 9. 目录移动规则

1. 先增加逻辑模块名和旧/新路径 Alias。
2. 每个目录一个独立 PR，只移动文件和修正 Import/Build/Deploy Path。
3. PR 不改 API、业务行为和数据库 Schema。
4. CI 同时运行旧生产 Smoke 和新路径构建。
5. 合并后观察至少一个发布周期，再删除 Alias。

移动顺序：Study Web → Study Admin → Study API → Study Worker → Quiz Web → Quiz FastAPI Legacy。

## 10. 仓库改名 Runbook

1. 冻结 Quiz 旧仓新功能，确认 P0 热修同步流程。
2. 扫描并更新 README、Actions、Secrets 引用、Deploy Script、服务器 Remote、Webhook 和文档 URL。
3. 确认公开 `jry21223/HENU-Kit` 保持原名和可访问。
4. 单独窗口把 `final-review-platform` 改为 `HENUKitDev`。
5. 验证 Redirect、Clone/Fetch、Actions、Deploy Key、Branch Protection、Webhook 和 Issue/PR 链接。
6. 旧 Quiz 仓发布归档公告，稳定后 Archive。

改名不能与数据库 Migration、目录移动或 DNS 切换同窗执行。

## 11. 环境与配置

- 环境至少分为 Local、CI、Staging、Production。
- 每个 Deploy Unit 有独立数据库凭据和最小权限。
- 健康检查不泄露版本、Secret、连接串或内部拓扑。
- Staging 使用可重复 Fixtures 和 Fake DirectMail；生产送达测试使用专用测试邮箱。
- 备份、恢复、RPO/RTO 在 CD 设计前冻结；推荐目标为 RPO ≤24h、RTO ≤2h。

## 12. 发布签字

发布必须同时具备：

- 开发 Owner：代码、配置和回滚命令确认。
- 测试 Owner：门禁用例、回归和数据对账确认。
- 项目负责人：范围、窗口、风险和生产批准。
- 观察负责人：上线后指标、日志和回切决策。
