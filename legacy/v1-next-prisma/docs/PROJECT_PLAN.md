# 一站式期末复习平台项目计划

## Phase 16 支付方向纠偏

当前支付方向已从 EasyPay / 易支付调整为微信支付 Native 扫码支付。EasyPay 不再作为目标方案或默认路径。当前资料交付方向是人工准备核心课程资料，通过资料导入、课程包绑定、支付成功后 entitlement 解锁和服务端 paid 下载鉴权完成交付；AI 生成资料不是当前主流程。

## 项目定位

本项目是面向高校学生的课程级期末复习平台。平台通过学生邮箱认证，按学校、学院、年级、专业、课程、任课老师或考试范围组织复习资料，帮助学生集中获取知识点讲义、模拟卷、答案解析、考前速背版、历年真题、在线刷题和错题本能力。

第一阶段只服务一个明确场景：

- 学校：河南大学
- 学院：软件学院
- 年级：2023级、2024级
- 专业：网络工程、软件工程
- 课程：离散数学、概率论与数理统计A、大学物理、高等数学A、软件工程、移动开发

本项目不是全国高校资料站，也不是一次性 Demo。所有阶段必须保持可运行、可验收、可回归。

## 当前仓库现状

审查时间：2026-06-20

审查结论：

- 当前目录为空。
- 当前目录不是 Git 仓库。
- 未发现 `package.json`、`next.config.*`、`tsconfig.json`、`pom.xml`、`build.gradle`、`prisma/schema.prisma` 或其他技术栈入口文件。
- 未发现已有页面、API、数据库、登录模块、测试或文档。
- 未发现可复用业务模块。

因此，后续适合按 MVP 方式新建 Next.js 项目，而不是改造既有项目。

## 推荐技术栈

前端：

- Next.js App Router
- React
- TypeScript
- Tailwind CSS
- shadcn/ui 可选，只有在确实提升开发效率时再引入

后端：

- Next.js Route Handlers / Server Actions
- 第一阶段不拆微服务

数据库：

- PostgreSQL
- Prisma ORM

鉴权：

- 学生邮箱验证码登录
- 开发环境 mock 邮件发送，在服务端日志输出验证码
- 生产环境预留真实邮件发送接口

文件存储：

- 本地开发使用 `uploads/`
- 后续预留 Cloudflare R2、阿里 OSS 或腾讯 COS

支付与 AI：

- Phase 10 前不接支付流程
- Phase 16 开始支付目标调整为微信支付 Native 扫码支付，旧易支付兼容基础版仅作为遗留待替换代码
- 绑定真实商户、支付查询、退款和对账留到支付阶段后续迭代
- Phase 12 前不接真实 AI 生成
- 保留权限、订单、AI job 等数据结构和文档设计

## 长期开发原则

- 先可运行，再完善。
- 先单学校单学院，再扩展。
- 先资料平台，再题库，再 AI。
- 先学生邮箱认证，再复杂身份系统。
- 先权限字段预留，再接支付。
- 先人工审核，再自动生成。
- 每次改动必须能本地启动。
- 每个阶段结束后必须更新自检文档。
- 不一次性生成巨大不可控改动。
- 不为了“大平台”破坏 MVP 可运行性。

## MVP 明确不做

- 不做全国高校库。
- 不做复杂推荐系统。
- 不在 Phase 10 前接支付。
- 不在未核对真实服务商文档和商户配置前开放生产支付。
- 不接真实 AI 生成。
- 不做社交系统、论坛、积分商城。
- 不做校园合伙人系统。
- 不做微服务。
- 不做复杂权限中心。
- 不做过度 UI 动效。

## Phase 0 目标

Phase 0 只做仓库审查和项目初始化文档：

- 审查当前仓库结构。
- 判断技术栈。
- 判断已有可复用模块。
- 判断是否适合直接做 Next.js MVP。
- 输出项目现状报告。
- 建立长期开发文档。
- 给出 Phase 1 文件级 TODO。

Phase 0 不写业务代码，不创建页面，不创建 API，不初始化数据库。

## Phase 1 文件级 TODO

Phase 1 目标：MVP 信息架构与静态页面，数据先用 mock。

建议新增文件：

- `package.json`：Next.js 项目依赖和脚本。
- `next.config.ts`：Next.js 配置。
- `tsconfig.json`：TypeScript 配置。
- `tailwind.config.ts`：Tailwind 配置。
- `postcss.config.mjs`：PostCSS 配置。
- `src/app/layout.tsx`：应用根布局。
- `src/app/page.tsx`：首页。
- `src/app/login/page.tsx`：登录页静态壳。
- `src/app/schools/page.tsx`：学校、年级、专业选择页。
- `src/app/courses/page.tsx`：课程列表页。
- `src/app/courses/[id]/page.tsx`：课程详情页。
- `src/app/materials/[id]/page.tsx`：资料详情页。
- `src/app/admin/page.tsx`：管理后台首页静态壳。
- `src/components/layout/`：导航、页面容器、移动端布局。
- `src/components/course/`：课程卡片、课程筛选器。
- `src/components/material/`：资料卡片、权限标签。
- `src/constants/mock-data.ts`：河南大学软件学院 mock 数据。
- `src/constants/enums.ts`：课程状态、资料类型、访问等级枚举。
- `src/types/index.ts`：School、College、Major、Course、Material 等类型。

Phase 1 页面验收：

- `/` 可访问。
- `/login` 可访问。
- `/schools` 可访问。
- `/courses` 可访问。
- `/courses/[id]` 可访问。
- `/materials/[id]` 可访问。
- `/admin` 可访问。
- 移动端没有明显横向溢出。
- 所有数据明确标注为 mock，不宣称数据库已完成。

## 每次开发前检查

- 当前处于哪个 Phase？
- 本次任务是否属于当前 Phase？
- 是否存在需求不明确？
- 是否需要先读文件？
- 是否可能破坏已有功能？
- 是否需要更新文档？
- 是否需要新增测试？

## 每次开发后检查

- 实际完成了什么？
- 修改了哪些文件？
- 哪些功能可以本地验证？
- 运行了哪些命令？
- 哪些检查通过？
- 哪些检查失败？
- 是否有未完成项？
- 是否有潜在风险？
- 下一步最小任务是什么？
