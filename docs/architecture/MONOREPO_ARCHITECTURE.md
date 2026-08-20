# HENU Kit Monorepo 架构

> 状态：目标架构 v1.1  
> 原则：开发代码集中到 `HENUKitDev`，公开入口 `HENU-Kit` 保持不变；Monorepo 不等于单体运行时。

## 1. 仓库决策

当前 `final-review-platform` 将作为 HENU Kit 最终开发仓库，并在迁移完成后改名为 `HENUKitDev`。

现有 `jry21223/HENU-Kit` 保持原名和公开职责：

- 公开项目介绍与索引。
- 对外路线图和社区信息。
- 面向贡献者的公开说明。

`HENUKitDev` 负责：

- Portal、Study、QuizCraft 和 Platform Core 实现代码。
- 内部架构、OpenAPI、事件 schema、测试和部署配置。
- 单仓 CI/CD 和发布单元管理。

公开仓不保存生产配置和内部部署细节；开发仓中需要公开的内容经评审后同步到公开仓。导入到 `archive/henukit-planning` 的内容只是导入时快照，不替代公开仓库。

## 2. 为什么使用 Monorepo

目标不是让所有代码变成一个进程，而是解决：

- 产品规范分散。
- 账户、角色、会员、通知和 API 契约重复定义。
- 资料库和刷题边界容易再次重叠。
- 跨产品变更难以一次评审和回归。
- CI、部署、设计 token 和安全基线不一致。

Monorepo 仍允许：

- Next.js、Vue、React/Vite 并存。
- Go 与 Python 并存。
- 子产品独立构建、独立镜像和独立部署。
- PostgreSQL 按服务独立数据库或 schema。
- 按路径触发 CI/CD。

## 3. 迁移期间的真实结构

```text
final-review-platform/             # 迁移完成后改名 HENUKitDev
├── apps/
│   ├── web/                       # 当前 Next.js，迁移期作为资料库 Web
│   ├── console/                   # HENUKit Console 独立应用边界
│   ├── study-legacy-admin/        # 物理隔离的旧 Vue Admin
│   └── portal/                    # 新 HENU Kit 主站
├── services/
│   ├── api/                       # 当前混合 Go API，迁移期兼容层
│   └── worker/                    # 当前 Go Worker
├── products/
│   └── quizcraft/                 # 导入的完整 QuizCraft
├── packages/
│   ├── design-tokens/
│   └── api-contracts/
├── docs/
│   ├── product/
│   ├── architecture/
│   ├── migrations/
│   └── adr/
├── infra/
├── scripts/
├── legacy/
└── archive/
    └── henukit-planning/          # 公开仓导入时快照，不替代公开仓
```

`products/quizcraft` 是迁移缓冲区。它先保持原仓库目录和运行方式，避免导入当天同时修改 FastAPI、React、部署脚本和数据库。

## 4. 目标结构

```text
HENUKitDev/
├── apps/
│   ├── portal/                    # henukit.cn
│   ├── console/                   # HENUKit Console，Vue/Vite
│   ├── study-web/                 # study.henukit.cn，Next.js
│   ├── study-legacy-admin/        # 物理隔离的旧 Study 后台，迁移期保留回滚
│   └── quiz-web/                  # quiz.henukit.cn，React/Vite
├── services/
│   ├── platform-core/             # Go 模块化单体 API；worker 在 core 内（cmd/mail-worker，即 platform-mail-worker）
│   ├── study-api/                 # 资料库业务 API
│   ├── study-worker/              # 资料处理任务
│   └── quiz-api-legacy/           # FastAPI，逐接口迁移期间保留
├── packages/
│   ├── design-tokens/
│   ├── api-contracts/
│   ├── event-schemas/
│   └── test-fixtures/
├── data/
│   └── course-quiz-mapping/
├── infra/
├── docs/
├── legacy/
└── archive/
```

## 5. 运行时架构

```text
Browser
├── henukit.cn               Portal
├── account.superhuazai.me   Platform Core（当前备案域名）
├── study.henukit.cn         Study Web -> Study API -> Study DB
└── quiz.henukit.cn          Quiz Web -> Adapter -> FastAPI/Go -> Quiz DB

Platform Core API -> Core DB
Platform Worker -> Redis Streams -> DirectMail
Business services -> signed events -> Platform Core
```

每个服务独立：

- 进程。
- 容器镜像。
- Health/Readiness。
- 数据库访问凭据。
- 发布和回滚开关。

业务模块不得因为同仓而直接读取其他服务数据库。

## 6. 产品边界

### Portal

统一品牌、入口、导航、账户状态、状态页和非官方声明。不复制资料和刷题业务。

### Study

只展示资料库：课程、搜索、预览、下载、投稿、审核和纠错。历史刷题、社区、动态、支付和泛 AI 代码先隐藏冻结，再按数据盘点迁移或删除。

### QuizCraft

唯一刷题产品，负责题库、题目、练习、作答、错题、进度、排行榜、反馈和题库工坊。第一阶段继续 React/Vite + FastAPI。

### Platform Core

负责统一用户、学生邮箱验证、会话、跨主域授权、角色、会员、entitlement、Account Links、事件、通知、邮件、指标和服务认证。

## 7. 身份和访问模型

平台区分：

1. 主体类型：`guest`、`user`、`service`。
2. 权限角色：`student`、`creator`、`reviewer`、`operator`、`admin`、`super_admin`。
3. 会员档位：`free`、`vip`。
4. 具体 entitlement 和资源授权。

VIP 是权益，不是权限角色。一个用户可以同时拥有多个有作用域的角色。完整规则见 [`ACCESS_CONTROL.md`](ACCESS_CONTROL.md)。

## 8. 数据 Owner

| 数据 | 唯一 Owner |
|---|---|
| 用户、邮箱身份、会话 | Platform Core |
| 角色、会员、entitlement | Platform Core |
| Account Links | Platform Core |
| 事件、通知、邮件、用户指标 | Platform Core |
| 课程、资料文件、下载记录 | Study API |
| 题库、题目、作答、错题、排行 | QuizCraft |
| 餐厅和榜单 | food（services/food） |
| 工具目录 | Portal |

平台核心不保存题目、作答、资料文件、餐厅或工具正文。

## 9. 目录迁移规则

目录移动、业务语义修改、数据库迁移和流量切换必须拆成不同 PR。

推荐顺序：

1. 先建立契约、测试和目标目录。
2. 保持旧目录可运行。
3. 增加兼容入口或代理。
4. 复制或移动单个部署单元。
5. 修正 CI、Compose 和部署路径。
6. 双跑验证。
7. 切流量。
8. 观察并演练回滚。
9. 最后删除旧路径。

## 10. 依赖规则

- `apps/*` 可以依赖 `packages/*`，不得 import 另一个 app 的内部源码。
- `services/*` 通过 API 或事件通信，不直接 import 其他服务的业务实现。
- `packages/*` 只放至少两个真实消费方需要的共享内容。
- `products/*` 只作迁移缓冲区，不长期新增跨产品公共能力。
- `archive/*` 和 `legacy/*` 不作为新代码运行依赖。

## 11. CI/CD

PR CI 按路径触发：

- Portal 改动：Portal lint/type/build/E2E。
- Study 改动：Next.js、Vue、Go API/Worker、资料和支付 smoke。
- Quiz 改动：Python、FastAPI smoke、React/Vite、Playwright、数据一致性。
- Platform 改动：Go、OpenAPI、Migration、PostgreSQL/Redis、服务认证和安全测试。
- Shared package 改动：所有真实消费方验证。

夜间和发布前运行全仓回归。

## 12. 仓库改名

仓库改名仅在以下条件全部满足后执行：

- Foundation、CI 和部署路径已稳定。
- Portal、Platform Core、Study、Quiz 均可按单元构建。
- GitHub Actions、Deploy Key、服务器 remote、脚本和文档已完成名称清单。
- 旧名称回滚和 Git remote 更新 Runbook 已演练。

执行动作仅为：

```text
jry21223/final-review-platform -> jry21223/HENUKitDev
```

现有：

```text
jry21223/HENU-Kit
```

保持不变。

## 13. 非目标

- 不把所有服务合成一个进程。
- 不统一第一阶段全部前端框架。
- 不一次性重写 FastAPI。
- 不让公开仓替代开发仓。
- 不把 VIP 设计成管理角色。
- 不因目录整齐扩大第一阶段业务范围。
