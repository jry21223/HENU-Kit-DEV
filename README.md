# 一站式期末复习平台

面向高校学生的课程级期末复习平台。平台目标是通过学生邮箱认证，按学校、学院、年级、专业和课程精准匹配知识点讲义、模拟卷、答案解析、考前速背版、历年真题、在线刷题和错题本。

当前第一阶段聚焦：

- 河南大学
- 软件学院
- 2023级 / 2024级
- 网络工程 / 软件工程
- 离散数学、概率论与数理统计A、大学物理、高等数学A、软件工程、移动开发

## 当前状态

当前 Phase 0 到 Phase 15 已完成基础验收。后续进入按真实运营需求细化的迭代阶段。

已完成：

- Phase 1 静态页面：首页、登录页、学校选择页、课程列表页、课程详情页、资料详情页、管理后台首页。
- Phase 2 基础模型：Prisma PostgreSQL schema、seed 脚本、Docker Compose、DB-first service。
- Phase 3 学生邮箱验证码登录、当前用户和退出登录。
- Phase 4 学校、学院、专业、课程和资料只读 API。
- Phase 5 资料下载 API 和 free/login_required/paid 基础权限。
- Phase 6 admin 服务端权限检查、课程/资料写 API 和上传限制。
- Phase 7 单选/判断题刷题、提交判题、解析展示和基础错题保存。
- Phase 8 错题本、课程/知识点筛选、移除错题和薄弱知识点统计。
- Phase 9 课程复习包、手动授权和 paid 内容 entitlement 校验。
- Phase 10 易支付兼容订单创建、签名校验、支付回调幂等结算和自动授权。
- Phase 11 学生资料投稿、我的投稿、reviewer/admin 审核、驳回原因和审核通过生成资料。
- Phase 12 admin AI 任务、本地草稿生成、失败状态记录和 draft 不自动发布约束。
- Phase 13 PDF 下载动态水印、原文件保护和非 PDF 正常下载。
- Phase 14 admin 数据统计、热门课程、高下载资料和错题知识点分布。
- Phase 15 关键页面 390px 移动端巡检与触控/文案优化。
- 无 `DATABASE_URL` 时 fallback 到 `src/constants/mock-data.ts`，保证本地仍可运行。
- 已通过 `npm install`、`npm run db:generate`、`npm run typecheck`、`npm run lint`、`npm test`、`npm run build` 和 `npm audit`。

尚未完成：

- Windows 主机 Prisma schema engine 写库失败；已使用 Linux Node Docker 容器成功执行 Prisma migrate。
- 尚未绑定真实支付商户和生产支付网关；上线前必须按目标易支付服务商文档复核字段、网关和回调细节。
- 尚未接入真实 AI 服务。
- Computer Use 插件当前初始化失败，最终 Computer Use 自检尚未通过。

## 技术栈

- Frontend：Next.js、React、TypeScript、Tailwind CSS
- Backend：Next.js Route Handlers / Server Actions
- Database：PostgreSQL
- ORM：Prisma
- Auth：学生邮箱验证码登录，Phase 3 实现
- Storage：本地 `uploads/`，后续预留对象存储
- Payment：Phase 10 已实现易支付兼容基础版；真实商户配置前不会展示外部支付跳转
- AI：Phase 12 已实现本地草稿生成流程，真实 AI 服务后续接入

## 本地启动方式

无数据库 fallback 启动：

```bash
npm install
npm run dev
```

本地地址：

```text
http://localhost:3000
```

## 环境变量说明

复制 `.env.example` 为 `.env` 后按需修改：

```env
DATABASE_URL="postgresql://final_review:final_review_dev@127.0.0.1:5432/final_review?schema=public"
AUTH_SECRET="replace-with-local-random-secret"
EMAIL_PROVIDER="mock"
EMAIL_FROM="noreply@example.com"
STORAGE_DRIVER="local"
LOCAL_UPLOAD_DIR="./uploads"
PDF_WATERMARK_FONT_PATH=""
NEXT_PUBLIC_APP_URL="http://localhost:3000"
EASYPAY_PID="1001"
EASYPAY_KEY="replace-with-easypay-key"
EASYPAY_GATEWAY_URL="https://pay.example.com/submit.php"
EASYPAY_TYPE="alipay"
```

Phase 2 中，如果没有 `DATABASE_URL`，页面会 fallback 到 mock 数据。
Phase 10 中，如果没有真实 `EASYPAY_*` 配置，开发环境仍可用本地签名和回调验收订单结算，但不会在前端展示真实支付跳转。
Phase 13 中，PDF 水印默认尝试使用 Windows 中文字体；非 Windows 部署建议通过 `PDF_WATERMARK_FONT_PATH` 指向可用中文 TTF/TTC 字体。

## 数据库迁移方式

如使用本仓库提供的 PostgreSQL Docker Compose：

```bash
docker compose up -d postgres
```

数据库可用后执行：

```bash
npm run db:generate
npm run db:migrate
npm run db:seed
```

如果 Windows 主机直接执行 `npm run db:migrate` 遇到 schema engine 空错误，可使用 Linux Node 容器执行迁移：

```bash
docker run --rm --network codex-codex-demo-2023-2024-a_default -v "%cd%:/app" -v final-review-node-modules:/app/node_modules -w /app -e DATABASE_URL="postgresql://final_review:final_review_dev@postgres:5432/final_review?schema=public" mirror.gcr.io/library/node:22-alpine sh -lc "npm ci && npm run db:migrate -- --name init"
```

当前 Phase 2 已完成迁移和 seed 验收。

## Seed 数据方式

`prisma/seed.ts` 提供河南大学软件学院基础 seed：

- 学校：河南大学
- 学院：软件学院
- 专业：网络工程、软件工程
- 年级：2023级、2024级
- 课程：离散数学、概率论与数理统计A、大学物理、高等数学A、软件工程、移动开发
- 资料：讲义、模拟卷、答案解析、考前速背版、免费样例资料
- 题库：离散数学、概率论与数理统计A、大学物理基础单选/判断题
- 课程包：离散数学期末复习包，包含 paid 模拟卷和答案解析
- 开发环境 admin：`admin@example.com`，仅开发使用
- 资料共建：学生可提交 PDF/TXT 资料，进入人工审核后才可能发布

## 测试方式

当前可运行检查：

```bash
npm run db:generate
npm run typecheck
npm run lint
npm test
npm run build
npm audit
```

后续建议逐步加入：

- Unit：邮箱域名校验、验证码过期、权限判断、课程筛选。
- Integration：登录、课程列表、资料详情、下载权限。
- E2E：用户登录、查看课程、下载资料、admin 上传资料、刷题、错题保存。

## 目录结构

当前核心结构：

```text
src/
  app/
  components/
  constants/
  lib/
  services/
  types/
prisma/
  schema.prisma
  seed.ts
docs/
uploads/
docker-compose.yml
package.json
```

## 当前功能状态

- 项目文档：已初始化。
- 页面：Phase 1 静态页面已完成。
- 数据库：schema、migration、seed、DB-first service 已完成。
- API：auth API 和课程/资料只读 API 已完成。
- 登录：学生邮箱验证码登录已完成。
- 下载权限：free/login_required/paid 规则已完成；paid entitlement 已接入课程包授权。
- 管理后台写操作：基础 API 已完成，表单 UI 待细化。
- 题库：基础单选/判断题、提交判题和解析展示已完成。
- 错题本：错题页、筛选、移除、重新练习入口和薄弱知识点统计已完成。
- 课程复习包：公开列表/详情、admin API、手动授权和 paid 下载校验已完成。
- 支付：订单创建、易支付兼容签名、异步通知、同步返回、重复回调幂等处理和 paid 自动授权已完成基础版。
- 资料共建：学生投稿、我的投稿、reviewer/admin 审核、通过发布资料和驳回原因已完成基础版。
- AI：admin AI 任务、本地生成草稿、失败状态和 draft 不自动发布已完成基础版；尚未接真实模型。
- PDF 水印：PDF 下载时动态生成浅灰斜向水印，包含用户、下载时间和用途声明；原文件不被覆盖。
- 数据统计：admin 可查看用户数、认证数、资料下载、热门课程、高下载资料和高错题知识点；不返回用户邮箱明细。
- 移动端：关键页面 390px 视口无横向溢出，筛选控件和刷题选项已做基础触控优化。

## 后续路线图

近期阶段：

- Phase 2：数据库与基础数据模型，已完成。
- Phase 3：学生邮箱登录。
- Phase 4：课程与资料真实 API 数据流。
- Phase 5：资料下载权限控制。
- Phase 6：管理员后台。
- Phase 7：在线题库基础版，已完成。
- Phase 8：错题本与知识点关联，已完成。
- Phase 9：课程复习包与权限体系，已完成。
- Phase 10：支付接入，已完成基础版。
- Phase 11：资料共建与审核，已完成基础版。
- Phase 12：AI 辅助内容生成，已完成基础版。
- Phase 13：PDF 水印与防盗传，已完成基础版。
- Phase 14：数据统计与运营后台，已完成基础版。
- Phase 15：移动端优化，已完成基础版。

后续建议：

- 增加课程访问日志和趋势统计。
- 完善 admin 表单 UI。
- 引入真实 AI 模型和异步任务队列。
- 真实支付商户联调和退款/对账。
- 大 PDF 水印性能优化。

完整路线见 `docs/ROADMAP.md`。

## 开发约束

- 不跳过当前 Phase。
- 不一次性重写整个项目。
- 不提前接 AI。
- 不把 mock 数据当成真实完成。
- 不把 admin 权限只做成前端隐藏按钮。
- 不把 paid 权限只做成前端判断。
- 每个阶段结束后必须更新 `docs/SELF_CHECK.md`。
