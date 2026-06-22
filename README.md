# 一站式学习平台 V2

> 面向高校学生的课程级学习平台：资料、刷题、AI、Wiki 共创、社区讨论、积分会员与前台销售闭环。

## 1. 项目概览

本项目是一个面向高校学生的课程级学习平台。平台按「学校 → 学院 → 专业 → 课程」组织学习内容，围绕期末复习、课程资料、刷题训练、AI 讲解、Wiki 共创和学生社区构建完整学习闭环。

项目当前按 V2 架构重构，目标不是简单资料站，而是一个可运营、可商业化、可扩展的学习平台。

### 核心目标

* 让学生快速找到自己学校、专业、课程对应的复习资料。
* 支持课程刷题、错题本、薄弱点统计和 AI 针对性强化。
* 用 Wiki、博客、帖子、动态沉淀学生原创内容和学习经验。
* 用积分和会员体系建立创作者激励与 AI 成本控制。
* 用 LangBot 前台销售承接咨询、介绍权益、引导注册和购买。
* 用独立管理后台完成内容审核、AI 草稿审核、用户运营和系统配置。

## 2. 产品形态

| 产品端          | 面向对象                     | 说明                              |
| ------------ | ------------------------ | ------------------------------- |
| Web 主站       | 学生、创作者、普通访客              | 找资料、刷题、AI 问答、Wiki、博客、帖子、动态、会员购买 |
| Admin 管理后台   | 管理员、审核员、运营人员             | 内容审核、用户管理、AI 工作流、积分会员、系统配置      |
| LangBot 前台销售 | 潜在用户、QQ群/微信群访客、客服入口      | 自动答疑、介绍套餐、引导注册、发放购买链接、售后分流      |
| Go API 服务    | Web、Admin、LangBot、Worker | 统一业务 API、鉴权、数据读写、支付回调、权限校验      |

## 3. 推荐目录结构

```txt
.
├── apps/
│   ├── web/                    # Next.js Web 主站
│   └── admin/                  # Vue 3 管理后台
├── services/
│   ├── api/                    # Go API 单体服务
│   ├── worker/                 # AI Worker / 异步任务服务
│   └── langbot/                # LangBot 前台销售适配层，可选
├── infra/
│   ├── docker/                 # Docker 相关配置
│   └── nginx/                  # Nginx 配置，可选
├── docs/
│   ├── architecture.md         # 架构设计
│   ├── api.md                  # API 文档
│   ├── database.md             # 数据库设计
│   ├── business.md             # 业务逻辑
│   ├── ai-workflow.md          # AI 工作流
│   ├── langbot-sales.md        # LangBot 前台销售说明
│   └── deployment.md           # 部署说明
├── scripts/
├── uploads/
├── docker-compose.yml
├── docker-compose.dev.yml
├── .env.example
└── README.md
```

## 4. 新成员快速上手路线

1. 先读本 README，理解项目边界。
2. 再读 `docs/architecture.md`，理解系统架构。
3. 再读 `docs/business.md`，理解业务流程。
4. 后端开发读 `services/api/internal`。
5. Web 开发读 `apps/web`。
6. 后台开发读 `apps/admin`。
7. AI/异步任务开发读 `services/worker`。
8. LangBot 销售开发读 `docs/langbot-sales.md` 和 `services/langbot`。
9. 接口开发前先看 `docs/api.md`。
10. 数据库改动前先看 `docs/database.md`。
