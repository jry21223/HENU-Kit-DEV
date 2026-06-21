# 架构设计

## 当前结论

当前仓库为空，没有既有架构。推荐从单体 Next.js MVP 开始，避免在早期引入微服务、复杂权限中心、独立后端服务或过多基础设施。

## 推荐架构

应用形态：

- Next.js App Router 单体应用。
- 页面、API、服务层和数据访问层在同一仓库内。
- PostgreSQL 作为主数据库。
- Prisma 管理 schema、migration 和 seed。

核心分层：

- `src/app/`：页面和 API Route Handlers。
- `src/components/`：可复用 UI 组件。
- `src/lib/`：数据库、认证、权限、邮件、文件存储、校验等基础能力。
- `src/services/`：课程、资料、认证、下载等业务服务。
- `src/types/`：跨模块类型。
- `src/constants/`：枚举和常量。
- `prisma/`：数据库 schema、migration、seed。
- `docs/`：项目计划、API、数据库、路线图、自检记录。
- `uploads/`：本地开发文件存储。
- `tests/`：单元、集成、E2E 测试。

## 目录建议

```text
src/
  app/
    page.tsx
    login/
    courses/
    materials/
    me/
    admin/
    api/
  components/
    ui/
    layout/
    course/
    material/
    auth/
    admin/
  lib/
    auth.ts
    db.ts
    permissions.ts
    email.ts
    storage.ts
    validators.ts
  services/
    course-service.ts
    material-service.ts
    auth-service.ts
    download-service.ts
  types/
    index.ts
  constants/
    enums.ts
prisma/
  schema.prisma
  seed.ts
  migrations/
docs/
tests/
uploads/
```

## 模块边界

用户与认证：

- 负责邮箱验证码、session、当前用户、退出登录。
- 不负责支付、不负责 admin 业务操作。

学校、学院、专业、课程：

- 负责学校层级结构和课程筛选。
- MVP 只维护河南大学软件学院相关数据。

资料：

- 负责资料展示、预览、下载权限、下载记录。
- draft、pending_review、archived 永远不展示给普通用户。

题库与错题：

- Phase 7 后实现。
- 题目列表接口不能暴露答案。
- 错题必须按当前用户隔离。

管理后台：

- 所有 admin API 必须在服务端检查角色。
- 前端隐藏按钮不能作为权限控制。

支付、AI、审核：

- 支付已完成易支付兼容基础版，真实商户和退款/对账后续迭代。
- AI 已完成本地草稿任务基础版，真实模型后续接入。
- 审核必须先人工确认，不能自动发布。

## 权限原则

- 未登录用户可以看公开页面和 free 资料。
- 登录且邮箱验证用户可以下载 login_required 资料。
- paid 内容必须由后端 entitlement 判断。
- reviewer 当前可以审核投稿；AI 草稿审核权限后续细化。
- admin 可以管理核心数据和 AI 任务。

## 文件存储原则

- 本地开发阶段使用 `uploads/`。
- 下载必须经过 API 权限检查。
- 不允许直接暴露可猜测的私有文件 URL。
- PDF 水印在下载 API 中动态生成，不能覆盖原文件。
- 非 PDF 文件不加水印，保持正常下载并显式返回水印状态。

## 可运行性原则

每个阶段都必须满足：

- 可以安装依赖。
- 可以本地启动。
- 核心页面可访问。
- 当前阶段核心流程可验证。
- 未完成内容明确标注，不冒充完成。
