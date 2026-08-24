# HENU Kit 练习服务

该目录承载 HENU Kit 的练习域实现。中文公共界面统一使用“练习服务”，不作为独立产品运营。

## 当前能力

- 选择题库、章节与练习模式并开始刷题。
- 由服务端判题并记录幂等作答结果。
- 在 HENU Kit Portal 中查看收藏、错题、统计与排行。
- 提交题目纠错并查看处理状态。

以下历史表面已经退出，不属于当前产品：独立首页、独立 OAuth 登录、浏览器题库管理、后台口令界面、反馈管理看板和转盘。旧 URL 只会回到刷题或纠错页面，不恢复第二套产品导航。

## 目录

```text
products/quizcraft/
├── go-service/   # Go/PostgreSQL 练习服务
├── web-app/      # 兼容练习壳；仅保留练习域页面
├── docs/         # 历史迁移与运维记录
└── generated/    # 题库构建与审计产物
```

Portal 是 HENU Kit 用户的主入口。浏览器通过 Portal Gateway 使用练习读写契约；练习服务不签发独立产品 Session，也不实现自己的登录回调。内部 service ID、数据库表名和生成类型为兼容既有契约可继续使用原标识。

## 本地验证

在仓库根目录运行：

```bash
pnpm --filter quiz-app test:syntax
pnpm --filter quiz-app build
pnpm --filter quiz-app test:browser:practice
pnpm --filter quiz-app test:browser:legacy-ranking
```

验证 Go 服务：

```bash
cd products/quizcraft/go-service
go test ./...
```

## 运行边界

- PostgreSQL 是题库、练习会话、作答、收藏、排行和纠错事实的运行时来源。
- Portal 命令路由默认关闭；只有 Portal 与服务端门禁协同启用后才接受真实用户写入。
- 游客身份只能由服务端签发的匿名 HttpOnly Cookie 建立；浏览器不能自报用户 ID。
- 收藏、个人统计与用户纠错状态由 Portal 的已认证会话绑定；旧练习壳遇到认证要求时引导回 HENU Kit Portal。
- 健康检查、页面 200 或一次跳转不能替代完整作答、收藏、纠错和登录旅程验收。

版本化题库管理契约目前只为迁移和可信服务调用保留，不提供浏览器入口。删除该冻结契约需要单独的破坏性契约发布，不能在前端恢复管理页面。

## 发布

正式发布必须使用仓库的 HENU Kit production release 工作流，并分别记录候选 SHA、CI、合并 SHA、部署 SHA 和生产用户旅程证据。
