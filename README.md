# HENU Kit Monorepo

> 当前实际开发仓库为 `jry21223/HENU-Kit-DEV`，它由原 `final-review-platform` 演进而来；迁移完成并通过发布验收后，计划规范为 `jry21223/HENUKitDev`。现有公开仓库 `jry21223/HENU-Kit` 保持原名，不被替换。

HENU Kit 是由河南大学学生自主发起并维护的统一校园工具系统。它不是简单的网站导航集合，也不是河南大学官方产品，不代表学校官方立场。

> 学生自主运营 · 非河南大学官方项目

## 文档入口

- [`docs/README.md`](docs/README.md)：文档中心，区分当前规范、运行维护和历史归档。
- [`CONTEXT-MAP.md`](CONTEXT-MAP.md)：当前领域上下文、规范词汇及各实现上下文的归属。
- [`docs/development/henukit-console-executable-spec.md`](docs/development/henukit-console-executable-spec.md)：HENUKit Console 与 QuizCraft 重构的当前执行规格。
- [`docs/development/implementation-plan.md`](docs/development/implementation-plan.md)：面向 1–2 名开发、1–2 名测试的实施计划。
- [`docs/development/go-no-go-checklist.md`](docs/development/go-no-go-checklist.md)：启动决策、第一周行动和停止条件。

新开发者应从文档中心进入，不再直接使用散落的旧 V2 文档作为当前规范。

## 仓库分工

- `HENUKitDev`：实际 Monorepo 开发仓库，承载产品代码、平台核心、契约、测试和部署配置。
- `HENU-Kit`：继续作为公开项目入口，承载公开介绍、项目索引、路线图和社区信息。
- 两个仓库不并行维护同一份实现代码；公开内容经评审后从开发仓同步。

## 产品结构

HENU Kit 对用户提供统一品牌、入口、导航、账户状态和跨产品跳转体验。各子产品可以独立部署、使用不同技术栈，但必须遵守统一的产品边界和平台契约。

- **主站**：`henukit.cn`，负责统一品牌、入口、导航、状态和公告。
- **资料库**：当前为 `study.superhuazai.me`，后续可迁移到 `study.henukit.cn`；只负责课程、资料检索、预览、下载、投稿、审核和纠错。
- **刷题**：当前独立 QuizCraft，后续可迁移到 `quiz.henukit.cn`；它是唯一刷题产品，负责题库、练习、作答、错题、进度、排行榜、反馈和题库工坊。
- **校园生活**：美食榜单等轻量校园工具。
- **平台核心**：统一账户、邮件、通知、事件、用户统计、服务间认证和 API 契约，不作为首页一级入口。

资料库不再开发或展示第二套刷题流程。资料页中的“去刷题”只负责携带课程上下文跳转到 QuizCraft。

## 身份与权益基线

平台从第一阶段开始区分：

- 主体类型：游客、登录用户、服务账号。
- 权限角色：学生、创作者、审核员、运营、管理员、超级管理员。
- 会员权益：免费学生、VIP 学生。VIP 是权益档位，不是管理角色。

游客不写入 `users` 表，可使用业务站本地的不可猜测匿名会话。一个用户可以同时拥有多个可限定作用域的角色。详细规则见 [`docs/architecture/ACCESS_CONTROL.md`](docs/architecture/ACCESS_CONTROL.md)。

## 当前迁移状态

本仓库当前仍包含原“一站式学习平台 V2”的全部代码：

- `apps/web`：Next.js 学习平台 Web，迁移期间收敛为资料库 Web。
- `apps/study-legacy-admin`：物理隔离的旧 Study Vue 管理后台，保留原路由、行为、部署入口与回滚能力。
- `apps/console`：独立 HENUKit Console 应用边界；HC-03 仅提供无旧依赖的最小 Bundle，六模块产品壳由 HC-04 交付。
- `services/api`：Go Gin/GORM API，当前同时包含账号、资料、刷题、社区、支付、AI 等能力。
- `services/worker`：Go + Redis Streams Worker。
- `legacy/v1-next-prisma`：V1 归档。

Monorepo Foundation 新增或导入：

- `apps/portal`：HENU Kit 主站入口。
- `products/quizcraft`：从 `jry21223/quizcraft-cn` 导入的完整 QuizCraft 产品代码，第一阶段保持 FastAPI + React/Vite 原样运行。
- `services/platform-core`：新平台核心的目标位置；在代码迁移前先通过模块边界和 OpenAPI 固化契约。
- `packages/design-tokens`：Kit 墨绿、纸白、Kit 麦金等跨前端框架设计变量。
- `packages/api-contracts`：OpenAPI 3.1、错误码、事件 schema 和生成产物。

完整结构、迁移顺序和兼容策略见：

- [`docs/architecture/MONOREPO_ARCHITECTURE.md`](docs/architecture/MONOREPO_ARCHITECTURE.md)
- [`docs/architecture/ACCESS_CONTROL.md`](docs/architecture/ACCESS_CONTROL.md)
- [`docs/migrations/FINAL_REVIEW_TO_HENUKIT.md`](docs/migrations/FINAL_REVIEW_TO_HENUKIT.md)
- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)
- [`docs/product/PRODUCT_BOUNDARIES.md`](docs/product/PRODUCT_BOUNDARIES.md)
- [`docs/product/DESIGN_SYSTEM.md`](docs/product/DESIGN_SYSTEM.md)

## 迁移原则

1. **不在第一步删除现有能力。** 先识别真实依赖、数据规模和线上使用情况，再决定保留、隐藏、冻结、迁移、替代或最终删除。
2. **保持线上学习平台可回滚。** 目录移动、域名迁移、数据库变更和认证切换分别进行，不能绑成一次发布。
3. **QuizCraft 逐接口迁移。** 第一阶段继续使用 FastAPI；新 Go 平台核心只接管公共能力。题库读取、作答、排行榜和题库工坊按契约测试、影子流量、双读/双算和功能开关逐批迁移。
4. **数据 Owner 唯一。** 用户归平台核心，资料归资料库，题目和作答归 QuizCraft，业务模块不得直接读取平台核心数据库。
5. **OpenAPI 驱动。** 新平台接口以 `/api/v1`、`snake_case`、UTC ISO 8601 和统一响应包络为标准；旧接口通过兼容层渐进接入。
6. **角色与权益分离。** 角色决定管理行为，会员和 entitlement 决定内容、额度和增强功能；VIP 不获得审核或管理权限。
7. **品牌不冒充官方。** 主色称为“Kit 墨绿”，不得称为河南大学官方标准色；所有公开产品固定展示非官方声明。

## 资料库体验约束

资料库保留“打开资料册”的创意，但调整为一次性轻量动画：

- `600–900ms`，只使用 `transform` 与 `opacity`。
- 标题、搜索框和主要按钮从首帧可操作。
- 移动端和 `prefers-reduced-motion` 使用静态或简单淡入。
- 不保留长距离 sticky、连续翻页、多层视差或持续滚动计算。

## 开发入口

迁移期间原学习平台命令继续有效：

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up --build
```

常用检查：

```bash
pnpm install --frozen-lockfile
pnpm run lint
pnpm run test
pnpm run build
cd services/api && go test ./...
cd ../worker && go test ./...
```

新增产品或共享包不得绕过 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) 和 [`docs/development/`](docs/development/) 中的模块边界、测试、迁移和发布要求。

## 生产安全

- 不提交邮箱验证码、Token、Cookie、JWT 私钥、支付密钥、LLM Key 或真实课程资料。
- 不允许业务模块直接连接平台核心数据库。
- 不通过跨主域共享 Cookie 实现统一登录。
- 不信任浏览器请求体中的 `user_id`、`role`、`membership_tier` 或 `entitlements`。
- 生产变更必须有灰度、监控、备份验证和应用回滚方案。
- 破坏性 Migration 不自动执行，采用 expand / migrate / contract。

## License

项目现有开源部分沿用仓库内许可证；导入的子产品保留各自许可证和来源说明。`HENUKitDev` 的可见性、许可证边界和向公开 `HENU-Kit` 同步的内容需在迁移里程碑中单独评审。
