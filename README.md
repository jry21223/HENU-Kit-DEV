# 一站式期末复习平台

面向高校学生的课程级期末复习平台。当前项目聚焦河南大学软件学院的 MVP：学生邮箱登录、课程资料浏览、资料下载权限、基础题库、错题本、课程包授权、投稿审核、PDF 水印、运营统计，以及支付和 AI 任务的基础模拟流程。

## 当前真实状态

当前进入 `Phase 16：MVP 硬化与上线前审查`。这不是生产可直接上线版本，而是一个已经具备端到端数据流和核心安全边界的 MVP。

已验证：
- Next.js App Router 页面和 API 可本地运行。
- Prisma schema、migration、seed 已建立。
- `free / login_required / paid` 下载权限在服务端判断。
- `paid` 资料下载需要登录、邮箱验证和有效 entitlement。
- admin API 和 reviewer API 使用服务端角色校验。
- 题目列表 API 不返回 `answer` / `explanation`。
- Email 验证码哈希存储、过期、一次性消费，API 不向前端返回验证码。
- 上传限制 PDF/TXT、10MB、后缀与 MIME 必须匹配。
- 下载只允许读取本地 `uploads/` 内文件，拒绝路径穿越。
- PDF 水印生成新 Buffer，不覆盖原文件。
- EasyPay 兼容流程校验签名、商户号、金额、订单状态，并使用 upsert 保持授权幂等。
- AI job 只生成 `draft` material，不会自动发布。
- mock fallback 仅允许非生产环境；生产环境缺少 `DATABASE_URL` 时不会回退到 mock 数据。

仍未生产化：
- 未接真实邮件服务，开发环境使用 mock 邮件输出。
- 未完成真实支付商户联调，EasyPay 目前是兼容基础版。
- 未接真实 AI 模型、文本提取、异步队列和人工审核队列。
- admin 后台表单仍是基础维护能力，不是成熟运营后台。
- 本地文件存储尚未迁移到对象存储。
- 缺少完整 E2E、压力测试、审计日志、限流、防刷验证码和生产监控。

## 技术栈

- Frontend：Next.js、React、TypeScript、Tailwind CSS
- Backend：Next.js Route Handlers / Server Components
- Database：PostgreSQL
- ORM：Prisma
- Auth：学生邮箱验证码登录，HTTP-only session cookie
- Storage：本地 `uploads/`
- Payment：EasyPay 兼容基础流程
- PDF：`pdf-lib` + `@pdf-lib/fontkit`

## 本地启动

安装依赖：

```bash
npm install
```

启动开发服务：

```bash
npm run dev
```

访问：

```text
http://localhost:3000
```

## 环境变量

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

注意：
- 非生产环境且没有 `DATABASE_URL` 时，课程和资料读取可以 fallback 到 mock 数据。
- `NODE_ENV=production` 时不会使用 mock fallback。
- 真实上线必须设置强随机 `AUTH_SECRET`、真实 `DATABASE_URL` 和真实邮件/支付配置。
- `.env` 不应提交到 Git；仓库只保留 `.env.example`。

## 数据库

启动 PostgreSQL：

```bash
docker compose up -d postgres
```

生成 Prisma Client、迁移并写入 seed：

```bash
npm run db:generate
npm run db:migrate
npm run db:seed
```

Seed 包含：
- 河南大学 / 软件学院
- 网络工程、软件工程
- 2023级、2024级
- 离散数学、概率论与数理统计A、大学物理、高等数学A、软件工程、移动开发
- 示例资料、题目、课程包、开发环境 admin

开发环境 admin：

```text
admin@example.com
```

仅用于本地开发和演示，不代表生产账号方案。

## 测试与验证

推荐上线前至少运行：

```bash
npm run db:generate
npm run typecheck
npm run lint
npm test
npm run build
npm audit
```

当前单元测试覆盖：
- 邮箱域名、验证码哈希、验证码可用性、session 防篡改和过期
- 下载权限：free、login_required、paid、未发布资料拒绝
- 下载路径穿越防护和响应 Content-Type
- 上传类型、大小、后缀/MIME 绕过尝试
- EasyPay 签名、商户号、sign_type、金额篡改
- 题目公开字段不泄露答案，答案提交规则
- PDF 水印不覆盖原文件
- 投稿、AI job、错题、analytics 基础规则

## 目录结构

```text
src/
  app/          页面与 API Route Handlers
  components/   UI 组件
  constants/    开发 mock 数据
  lib/          鉴权、权限、上传、支付、水印等基础库
  services/     数据访问和业务服务
  types/        公共类型
prisma/
  schema.prisma
  seed.ts
  migrations/
docs/
tests/
uploads/
```

## 安全边界

当前已做：
- admin/reviewer 权限在服务端检查。
- paid 下载在服务端检查 entitlement。
- draft、pending_review、archived 资料不会对普通用户展示或下载。
- 题目列表不返回答案。
- 上传文件名会清洗，文件类型和大小受限。
- 本地下载路径限定在 `uploads/` 内。
- 支付回调校验签名、商户号、金额和订单状态。
- AI 生成结果默认 draft。
- 学生投稿默认 pending，必须 reviewer/admin 审核。

仍需上线前补强：
- 验证码发送限流和失败次数限制。
- CSRF 策略和更细的 session 安全策略。
- 文件内容嗅探和病毒扫描。
- 对象存储私有桶与临时签名 URL。
- 支付商户真实回调字段复核、订单对账和退款流程。
- 操作审计日志和告警。
- E2E 覆盖登录、下载、支付回调、admin 审核。

## 功能成熟度

可作为 MVP 继续迭代：
- 学校/学院/专业/课程/资料结构
- 学生邮箱登录基础流程
- 资料权限下载
- 题库和错题本基础版
- 课程包和手动授权
- 投稿审核基础版
- PDF 水印基础版
- admin analytics 基础指标

Demo 级或待联调：
- EasyPay 真实商户支付
- AI 内容生成
- admin 后台深度运营
- 文件存储和防盗链
- 移动端完整产品级体验

## 仓库卫生

`.gitignore` 已排除：
- `.env*` 真实环境文件，保留 `.env.example`
- `node_modules/`
- `.next/`、`dist/`、`out/`
- 日志、测试报告、缓存
- `uploads/` 运行时文件，保留 `uploads/.gitkeep`
- 本地数据库、dump、私钥和证书

GitHub 仓库当前为 private。

## 下一步建议

1. 做真实 E2E 测试：登录、下载、投稿审核、支付回调。
2. 增加验证码限流和登录失败保护。
3. 将本地上传迁移到私有对象存储。
4. 做 EasyPay 真实商户沙箱/小额联调。
5. 拆出 AI 任务队列和人工审核工作台。
6. 补生产部署文档、备份策略、监控和审计日志。
