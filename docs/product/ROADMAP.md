# HENU Kit Monorepo Roadmap

> 当前阶段：R0 Monorepo Foundation  
> 原则：一个仓库、多个独立部署单元；先统一事实来源，再迁移业务和流量。

## R0：Monorepo Foundation

### 范围

- 当前 `final-review-platform` 作为最终仓库。
- 根 README、产品边界、设计系统、开发规范、架构和 ADR 转为 HENU Kit。
- 导入 QuizCraft 至 `products/quizcraft`。
- 导入旧 HENU-Kit 规划仓库至 `archive/henukit-planning`。
- 建立 design tokens、API contracts 和 module manifest。
- 保持现有学习平台路径和部署不变。

### 退出条件

- 外部仓库代码已进入迁移分支。
- 导入来源 SHA 和许可证可追溯。
- 当前学习平台仍可构建。
- 没有生产流量、数据库或 DNS 变更。

## R1：Portal Preview

### 范围

- 建立 `apps/portal` 可运行工程。
- 首页展示美食榜单、工具箱、学习三个一级入口。
- 学习页展示资料库、刷题、接毕设（二期）。
- 接入统一导航、Kit 墨绿和非官方声明。
- 增加产品状态与维护信息。

### 退出条件

- 360px 和桌面端均可完成核心入口发现。
- 新用户 30 秒内找到资料或进入 QuizCraft。
- 主站不复制资料和刷题正文。
- 可独立部署到测试域名。

## R2：Platform Core Skeleton

### 范围

- `services/platform-core` 和 `services/platform-worker`。
- Go 1.26.x 固定补丁版本。
- PostgreSQL、Redis、OpenAPI 3.1。
- 版本化 Migration。
- Health、Readiness、JSON 日志和 request ID。
- 验证码、OAuth client、authorization code、session、account link 基础表。

### 退出条件

- OpenAPI lint、生成一致性、单元、race 和集成测试通过。
- 空库和升级 Migration 通过。
- 测试人员可基于 Mock 编写契约测试。
- 现有业务站尚未强制切换。

## R3：Unified Account Pilot

### 范围

- 河南大学学生邮箱验证码。
- DirectMail critical 队列。
- Authorization Code Flow。
- callback/return_to 白名单。
- state、单次授权码、服务端交换和业务站本地会话。
- 一个测试站点接入。

### 退出条件

- 真实学生邮箱送达。
- 授权码重复交换失败。
- 非法 callback/state 被拒绝。
- 日志无完整邮箱、验证码、Token、Cookie。
- 可切回旧登录。

## R4：Event / Notification / Email

### 范围

- events + outbox_events 同事务。
- Redis Streams relay。
- 站内通知、通知偏好、邮件投递和死信。
- submission、correction、points 和 school notice 首批事件。
- critical / transactional / digest 隔离。

### 退出条件

- 重复事件不重复通知、发信或发积分。
- Redis 暂时不可用不丢已提交事件。
- Worker 重启可恢复。
- 重试耗尽进入死信并可人工重投。

## R5：QuizCraft Unified Identity

### 范围

- 可信匿名会话。
- 平台统一用户绑定。
- 作答不信任请求体 user_id。
- 账号冲突和并发绑定处理。
- 课程 ID 与题库 ID 映射。

### 退出条件

- 匿名历史数据保留。
- 并发绑定只有一次成功。
- 用户无法伪造其他统一用户身份。
- 资料库课程可跳转对应题库。
- 可回到旧匿名认证。

## R6：Study Product Boundary

### 范围

- `study.superhuazai.me` 学生前台只展示资料库。
- 刷题入口统一跳转 QuizCraft。
- Blog、论坛、动态、泛 AI 和第二套刷题从主导航隐藏/冻结。
- 保留课程、资料、投稿、审核、纠错、下载权限和审计。
- “打开资料册”改为轻量一次性动画。

### 退出条件

- 学习平台前台无第二套刷题流程。
- 资料下载、审核和安全 smoke 全通过。
- 旧路由有观测、兼容和回滚。
- 非资料能力均有保留/隐藏/冻结/迁移/替代/删除决策。

## R7：QuizCraft Go Migration

### 批次

1. Go 兼容网关与 FastAPI fallback。
2. 题库只读。
3. 反馈。
4. 练习开始。
5. append-only 作答事件、双算统计与排行榜。
6. 作答写流量。
7. 题库工坊。
8. FastAPI 零流量观察和归档。

### 退出条件

- 每批接口有契约测试和 provider 开关。
- 题库、作答和排行一致性达到团队门槛。
- 回滚演练通过。
- FastAPI 下线前至少两周零生产流量。

## R8：Physical Layout Migration

### 范围

- `apps/web` → `apps/study-web`
- `apps/admin` → `apps/study-legacy-admin`（已完成物理拆分）
- `services/api` → `services/study-api`
- `services/worker` → `services/study-worker`
- `products/quizcraft/web-app` → `apps/quiz-web`
- QuizCraft 后端 → `services/quiz-api-legacy`

### 退出条件

- 每次只移动一个部署单元。
- 新旧 CI、Docker 和部署路径有兼容窗口。
- 测试环境和生产 smoke 通过。
- 旧路径兼容在一个发布周期后清理。

## R9：Repository Rename

### 范围

- 仓库改名为 `HENU-Kit`。
- 更新 CI badge、镜像、Webhook、Deploy Key、GitHub App 和文档。
- 旧 HENU-Kit 与 QuizCraft 仓库归档并指向 monorepo。

### 退出条件

- `main` 保护和必需检查启用。
- 所有部署不硬编码旧仓库名。
- GitHub redirect 和本地 remote 验证完成。
- 旧仓库没有继续开发的分支或自动化。

## 不进入近期范围

- 适配其他学校。
- 大规模微服务拆分。
- 统一重写所有前端框架。
- 一次性重写 QuizCraft FastAPI。
- 复杂推荐、私信和增长系统。
- 在需求未验证前扩展“接毕设”。
- 以平台重构为理由同步重做支付和会员业务。

## 里程碑管理

每个里程碑必须：

- 有 Owner 和绑定当前 SHA 的 Review 证据；存在协作者时优先安排外部 Reviewer。
- 有可演示结果。
- 有自动化与人工测试。
- 有灰度和回滚。
- 有明确退出条件。
- 预留至少 20% 联调、评审和修复缓冲。
