# QuizCraft CN

QuizCraft CN 是一个面向中文课程题库的刷题系统。它用 React + TypeScript 提供网页端练习体验，用 FastAPI 提供题库、答题、反馈和管理接口，生产环境以 PostgreSQL 保存题库快照、排行榜、用户统计和题目统计，并暴露MCP接口供Agent处理反馈。


目前题库为河大专供
网站：superhuazai.me


## 功能概览

- 刷题练习：支持随机、难题、章节等练习模式。
- 排行榜与统计：记录用户答题结果、正确率和题目统计。
- 反馈看板：公开查看反馈列表，管理端可标记处理状态。
- 随机大转盘：用于课堂或社群场景的随机抽取。
- 题库工坊：上传 PDF、Word、TXT 或 JSON，检查、编辑并保存题库。
- AI 解析：通过 OpenAI、DeepSeek 或兼容接口为题目生成解析。
- Ops 模式：生产版默认保留刷题、排行榜、反馈看板和大转盘，隐藏题库工坊等管理入口。

## 技术栈

| 层级 | 技术 | 用途 |
| --- | --- | --- |
| 前端 | React 18、TypeScript、Vite、React Router | 单页应用、路由和构建 |
| UI 与状态 | Tailwind CSS、Framer Motion、Zustand、Lucide React | 样式、动效、客户端状态和图标 |
| 后端 | Python 3.11、FastAPI、Pydantic v2、Uvicorn | REST API、WebSocket 进度推送和请求校验 |
| 数据库 | PostgreSQL、psycopg 3 | 题库快照、排行榜、用户统计和题目统计 |
| 文件与 AI | PyPDF2、python-docx、httpx | 文件解析和 LLM 接口调用 |
| 部署 | Nginx、systemd、环境文件 | 静态站点、反向代理和后端进程管理 |

## 项目结构

```text
.
├── server.py                  # FastAPI 后端入口
├── db_storage.py              # PostgreSQL 数据访问层
├── requirements.txt           # Python 依赖
├── start.sh                   # 本地完整开发环境
├── start_ops.sh               # 本地 ops 预览
├── web-app/                   # React 前端
├── scripts/                   # 安装、构建、迁移、题库和反馈脚本
├── deploy/                    # systemd、Nginx、环境文件模板
├── docs/                      # 运维和专项工作流文档
├── tiku/                      # 本地题库导入/导出缓存
├── generated/                 # 题库构建产物和审计中间文件
├── electron-app/              # Electron 桌面端壳
└── android-app/               # Android 端工程
```

## 快速开始

### 环境要求

- Python 3.11 或兼容的 Python 3
- Node.js 与 npm
- PostgreSQL 可选；未配置 `DATABASE_URL` 时，后端会使用本地 JSON 运行时兜底。生产环境必须配置 PostgreSQL。

### 安装依赖

```bash
scripts/install_deps.sh
```

这个脚本会创建 `.venv`，安装 `requirements.txt`，并安装 `web-app` 的 npm 依赖。

### 启动完整开发环境

```bash
./start.sh
```

默认访问地址：

- 前端: <http://localhost:5173>
- 后端 API: <http://localhost:10086>
- API 文档: <http://localhost:10086/docs>

也可以分开启动：

```bash
.venv/bin/python server.py
npm --prefix web-app run dev
```

## 常用命令

```bash
# 安装后端和前端依赖
scripts/install_deps.sh

# 启动后端
.venv/bin/python server.py

# 启动前端开发服务
npm --prefix web-app run dev

# 前端检查与构建
npm --prefix web-app run lint
npm --prefix web-app run test:syntax
npm --prefix web-app run build

# 后端 smoke test
.venv/bin/python scripts/smoke_backend.py

# 本地预览 ops 版本
./start_ops.sh
```

## 运行模式

完整开发模式包含首页、刷题、题库工坊、排行榜、反馈看板和大转盘。

Ops 模式由 `VITE_APP_MODE=ops` 控制，当前保留：

- `/practice`
- `/ranking`
- `/feedback-board`
- `/wheel`

Ops 模式会把首页重定向到刷题页，并隐藏 `/extract` 题库工坊入口。生产前端默认使用同源 `/api` 请求后端。

`web-app/.env.ops` 是 ops 构建的前端配置入口，常见变量包括：

```bash
VITE_APP_MODE=ops
VITE_API_BASE_URL=/api
VITE_DONATE_QR_URL=/wechat-receive-qrcode.jpg
VITE_ANNOUNCEMENT_MESSAGE=公告文案
VITE_ANNOUNCEMENT_QQ=
VITE_QQ_GROUP_QR_URL=/henu-kit-qq-group.png
```

## 题库工作流

网页端“题库工坊”是题库导入、校验、编辑、AI 解析和保存的主入口。命令行脚本只封装现有后端管理接口，不维护第二套解析逻辑。

```bash
# 从原始文件提取题库
python3 scripts/extract_bank_via_api.py source.docx \
  --key my_bank \
  --name "题库名称" \
  --output /tmp/my_bank.parsed.json

# 为已有 JSON 生成解析，可选择保存回线上题库
python3 scripts/analyze_bank_via_api.py /tmp/my_bank.parsed.json \
  --key my_bank \
  --save

# 上传标准 JSON
python3 scripts/upload_bank_via_api.py /tmp/my_bank.parsed.json \
  --key my_bank
```

如果解析结果有选项缺失、答案错位或校验问题，应优先修正源文件、导出的 JSON 或仓库里的后端解析逻辑，再重新走同一条接口链路。不要在临时脚本里绕过题库工坊和 `/api/extract/*` 主流程。

标准题库 JSON 至少包含：

```json
{
  "meta": {
    "name": "题库名称",
    "color": "#1976d2"
  },
  "questions": [
    {
      "id": "q0001",
      "type": "single",
      "chapter": "第一章",
      "content": "题目内容",
      "options": ["选项 A", "选项 B"],
      "answer": 0,
      "analysis": "解析内容"
    }
  ]
}
```

题型支持 `single`、`multi`、`judge` 和 `blank`。更多 WSL 和批处理说明见 [docs/wsl-bank-upload.md](docs/wsl-bank-upload.md)。

## API 与权限边界

普通刷题和公开看板接口保持公开：

- `GET /api/banks`
- `POST /api/practice/start`
- `POST /api/practice/submit`
- `GET /api/ranking`
- `GET /api/feedback/dashboard`
- `GET /api/wheel`
- `POST /api/wheel`

生产环境必须只在后端配置 `ADMIN_TOKEN`。浏览器先通过 `POST /api/admin/session`
交换一个短期 HttpOnly Cookie，后续管理请求和分析 WebSocket 都使用该会话；脚本和
CLI 客户端继续使用请求头 `X-Admin-Token`：

- `POST /api/extract/parse`
- `POST /api/extract/analyze`
- `POST /api/extract/export`
- `POST /api/banks/save`
- `POST /api/banks/java/append-from-markdown`
- `PATCH /api/feedback/{feedback_id}/status`

管理会话默认有效 8 小时（可通过 `ADMIN_SESSION_TTL_SECONDS` 调整，范围 5 分钟到
24 小时），刷新页面不会退出，主动退出或过期后需要重新验证。Cookie 使用
`HttpOnly`、`SameSite=Strict`，生产域名必须启用 HTTPS。不要定义
`VITE_ADMIN_TOKEN`：所有 `VITE_*` 变量都会进入公开的浏览器构建产物。也不要把生产
Token 写进仓库、前端环境文件或聊天记录。

## 数据与生产边界

生产环境以 PostgreSQL 为运行时数据源。后端配置 `DATABASE_URL` 后，会从 `question_banks` 和 `bank_questions` 加载题库，并把排行榜、用户统计、题目统计和反馈数据写入数据库。

本地兜底加载是显式 registry 加 `tiku/*.json`：`server.py` 中的内置题库 registry 会指定少量文件源，例如 `generated/software_engineering_process_tests.json`；随后才扫描 `tiku/` 下的 JSON。系统不会自动扫描整个 `generated/` 目录，新增生成题库如果需要本地/API 可见，应加入 registry 或上传到数据库。

`tiku/*.json` 只作为导入、导出、本地编辑或特殊修复流程中的文件缓存，题库 JSON 不应提交到 Git。默认不要在启动时把本地 JSON 同步回数据库：

```bash
QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB=0
```

只有做一次性旧题库迁移或明确需要同步文件源时，才临时设置 `QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB=1`。生产反馈修复的具体顺序见 [docs/feedback-triage-workflow.md](docs/feedback-triage-workflow.md)。

## 生产部署

推荐部署形态：

- Nginx 托管 `web-app/dist`
- Nginx 将 `/api` 和 `/ws` 反向代理到后端 `10086`
- systemd 管理 FastAPI 后端服务
- PostgreSQL 保存生产数据

构建 ops 前端并同步静态目录：

```bash
scripts/install_deps.sh
STATIC_DEPLOY_DIR=/var/www/quizcraft-cn scripts/build_ops.sh
```

Nginx/Caddy 反向代理至少需要：

- `/` 指向静态目录，例如 `/var/www/quizcraft-cn`
- `/api` 转发到后端 HTTP 服务
- `/ws` 转发到后端 WebSocket 服务

生产环境常用变量：

```bash
APP_HOST=127.0.0.1
APP_PORT=10086
CORS_ORIGINS=https://your-domain.example
DATABASE_URL=postgresql://quizcraft:change-me@127.0.0.1:5432/quizcraft
ADMIN_TOKEN=change-me
ADMIN_SESSION_TTL_SECONDS=28800
DISABLED_BANK_KEYS=
QUIZCRAFT_SYNC_LOCAL_BANKS_TO_DB=0
```

默认 CORS 只允许本地开发地址。生产环境必须用 `CORS_ORIGINS` 显式写出允许访问的前端来源，不要使用 `*`。

部署模板在 `deploy/`：

- `deploy/quizcraft-cn.env.example`
- `deploy/quizcraft-cn.service.example`
- `deploy/nginx.conf.example`
- `deploy/quizcraft-feedback-mcp.env.example`
- `deploy/quizcraft-feedback-mcp.service.example`

## 数据迁移与校验

旧排行榜和题目统计迁移到 PostgreSQL：

```bash
.venv/bin/python scripts/migrate_rankings_to_db.py
.venv/bin/python scripts/check_bank_db_consistency.py
.venv/bin/python scripts/smoke_backend.py
```

题库身份、反馈处理和生产修复相关脚本集中在 `scripts/`，运行前先确认当前生产 loader、数据库和环境文件配置。

## 更多文档

- [docs/wsl-bank-upload.md](docs/wsl-bank-upload.md)：WSL/命令行题库上传流程。
- [docs/feedback-triage-workflow.md](docs/feedback-triage-workflow.md)：生产反馈修复流程。
- [docs/performance-comparison-2026-06-30.md](docs/performance-comparison-2026-06-30.md)：前端性能对比记录。
- [electron-app/README.md](electron-app/README.md)：Electron 桌面端说明。
- [android-app/README.md](android-app/README.md)：Android 端说明。

## License

MIT License
