# final-review-platform

面向河南大学软件学院学生的一站式期末复习平台。当前项目已具备 MVP 主流程和多个扩展模块的基础实现，但状态仍是“内测准备 / 上线前硬化”，不是正式生产上线版本。

## 当前真实状态

- 已实现基础页面、课程资料浏览、学生邮箱验证码登录、下载权限检查、后台基础维护、题库/错题本、课程包和 entitlement 基础模型。
- 当前支付方向已调整为 **微信支付 Native 扫码支付**。
- EasyPay / 易支付不再是目标支付方案，也不应作为默认支付路径继续推进。
- 已实现微信 Native mock 下单骨架、订单状态查询和生产禁止 mock 的配置校验。
- 真实微信支付 live API 仍未接入，需要部署时配置商户号、AppID、API v3 Key、商户私钥、证书序列号和公网 `notify_url` 后继续联调。
- 开发/测试环境可以使用 mock 支付；生产环境必须禁止 mock。
- 核心课程资料由人工准备，通过 manifest 或后台导入后绑定课程和课程包。
- AI 生成资料不是当前主流程，相关草稿能力不能替代人工准备和审核。

## 技术栈

- Next.js App Router
- React
- TypeScript
- Tailwind CSS
- Prisma
- PostgreSQL
- Vitest

## 本地启动

```bash
npm install
npm run prisma:generate
npm run db:push
npm run db:seed
npm run dev
```

默认访问：

```text
http://localhost:3000
```

## 环境变量

复制 `.env.example` 为 `.env`，再按本地情况填写。

核心变量：

```env
DATABASE_URL=""
AUTH_SECRET=""
NEXT_PUBLIC_APP_URL="http://localhost:3000"
EMAIL_PROVIDER="mock"
EMAIL_FROM="Final Review Platform <noreply@example.com>"
STORAGE_DRIVER="local"
LOCAL_UPLOAD_DIR="uploads"
PDF_WATERMARK_FONT_PATH=""
```

微信支付 Native 变量：

```env
WECHAT_PAY_MODE="mock"
WECHAT_PAY_APPID=""
WECHAT_PAY_MCH_ID=""
WECHAT_PAY_API_V3_KEY=""
WECHAT_PAY_MERCHANT_SERIAL_NO=""
WECHAT_PAY_MERCHANT_PRIVATE_KEY=""
WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH=""
WECHAT_PAY_PLATFORM_CERTS_DIR=""
WECHAT_PAY_NOTIFY_URL=""
WECHAT_PAY_NATIVE_EXPIRE_MINUTES="15"
```

注意：

- `WECHAT_PAY_MODE=mock` 只允许 `development` / `test`。
- `NODE_ENV=production` 时必须使用 `WECHAT_PAY_MODE=live`，且不能缺少微信支付必要参数。
- 不要提交真实私钥、证书、API v3 Key 或课程资料 PDF。

## 数据库

```bash
npm run db:push
npm run db:seed
```

当前 seed 数据用于本地演示和内测准备，不等同于生产数据。

## 测试与检查

```bash
npm run typecheck
npm run lint
npm test
npm run build
```

当前测试覆盖认证、权限、上传、题目字段隐藏、PDF 水印、下载路径、微信 Native mock 下单、生产禁止 mock、live 缺配置报错和订单支付权限。下一轮仍需补微信回调验签、解密、金额校验和幂等授权测试。

## 目录结构

```text
src/
  app/
  components/
  constants/
  lib/
  services/
  types/
prisma/
docs/
tests/
uploads/
```

`uploads/` 仅保留 `.gitkeep`。真实课程资料应放在部署环境挂载目录或被 `.gitignore` 忽略的 `uploads/materials/` 下。

## 资料交付方向

当前核心课程资料已经由人工准备，平台目标是：

1. 通过 manifest 或后台导入资料。
2. 将资料绑定到学校、学院、专业、年级、课程。
3. 将 paid 资料绑定到课程包。
4. 支付成功后由服务端回调发放 entitlement。
5. paid 资料下载必须经过服务端 entitlement 检查。

计划中的 manifest 示例文件：

```text
data/material-manifest.example.json
```

真实资料文件不要提交到仓库。

## 支付方向

目标支付方案：微信支付 Native 扫码支付。

目标链路：

1. 用户购买课程包。
2. 服务端创建 pending 订单。
3. 服务端调用微信 Native 下单并返回 `code_url`。
4. 前端渲染二维码并轮询订单状态。
5. 微信服务器回调项目 notify API。
6. 服务端验签、解密、校验订单号、校验金额、校验商户信息。
7. 服务端将订单更新为 paid，并且只发放一次 entitlement。

当前已完成微信 Native mock 下单骨架和订单字段迁移。真实微信 API 接入、回调验签/解密和二维码渲染仍在后续。

## 文档

- `docs/PROJECT_PLAN.md`
- `docs/ROADMAP.md`
- `docs/API_DESIGN.md`
- `docs/DATABASE_DESIGN.md`
- `docs/SELF_CHECK.md`
- `docs/WECHAT_PAY_NATIVE.md`

## 上线前仍需完成

- 删除 EasyPay 遗留工具文件和历史路由。
- 增加微信 Native live 支付服务端实现。
- 增加微信回调验签、解密、金额校验和幂等授权测试。
- 增加 material manifest 导入脚本和路径穿越保护。
- 确认生产环境禁止 mock 数据和 mock 支付。
- 使用真实商户参数完成微信支付联调。
