# 最简开发与部署流程

这份文档只保留日常开发和上线必须知道的内容。

## 1. 当前仓库结构

HENU Kit 是一个 Monorepo，但各服务独立构建、独立部署：

- `apps/portal`：HENU Kit 主站
- `services/portal-api`：Portal 数据 API
- `services/portal-gateway`：Portal Gateway / BFF
- `apps/web`：资料库 Web（原学习平台兼容代码）
- `apps/study-legacy-admin`：资料库旧管理后台
- `services/api`：资料库兼容 API
- `services/worker`：资料库 Worker
- `products/quizcraft`：QuizCraft
- `services/platform-core`：统一账户与平台核心
- `apps/console`：统一管理后台

## 2. 本地开发

```bash
cp .env.example .env
pnpm install --frozen-lockfile
docker compose -f docker-compose.dev.yml up --build
```

只开发单个前端时，可使用对应包的 `dev` 命令。例如：

```bash
pnpm --filter @henukit/portal dev
```

## 3. 提交前检查

```bash
pnpm run lint
pnpm run test
pnpm run build

cd services/api && go test ./...
cd ../worker && go test ./...
```

只修改一个模块时，可以只执行该模块的检查，但合并前 CI 必须通过。

## 4. 开发流程

```text
创建分支
→ 修改代码
→ 本地检查
→ 提交 Pull Request
→ CI 通过
→ 合并 main
```

不要直接向 `main` 推送生产改动。

## 5. 部署原则

生产服务器只负责运行服务，不负责源码构建。

```text
main 更新
→ GitHub Actions 安装依赖、测试和构建
→ 生成绑定完整 Commit SHA 的 Artifact
→ 上传服务器
→ 校验、解压、重启对应服务
→ health/readiness 检查
→ 失败则回滚
```

禁止在生产服务器执行完整的：

```text
pnpm install
next build
go build
```

## 6. 当前部署现状

目前仓库中已经验证的正式发布流程是：

```text
.github/workflows/deploy-study.yml
```

它负责构建并发布：

- `apps/web`
- `apps/study-legacy-admin`
- `services/api`
- `services/worker`

Portal、Portal API、Portal Gateway、Platform Core、Console 和 Monorepo 内的 QuizCraft 仍需要各自的 Artifact Workflow。它们应复用同一种模式，不应在服务器本地构建。

## 7. 新服务上线模板

每个新部署单元只需要补齐六项：

1. GitHub Actions 构建和测试
2. Artifact 打包
3. 服务器运行目录和环境文件
4. systemd Service
5. Nginx 路由（需要公网时）
6. `/healthz` 或 `/readyz` 与回滚命令

## 8. 最简单的上线顺序

```text
Portal API
→ Portal Gateway
→ Portal Web
→ Platform Core
→ Console
→ QuizCraft 发布来源接入 Monorepo
```

每次只上线一个部署单元，验证正常后再继续下一个。

## 9. 发布完成标准

一个服务只有同时满足以下条件，才算上线：

- CI 通过
- Artifact 对应完整 Commit SHA
- systemd 服务为 `active`
- health/readiness 正常
- 公网页面或 API Smoke 正常
- 可以回滚到上一版本

> 学生自主运营 · 非河南大学官方项目
