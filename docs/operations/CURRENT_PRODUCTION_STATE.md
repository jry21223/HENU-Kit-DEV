# HENU Kit 生产现状记录

> 本文档是仓库内**唯一按证据记录「生产当前实际状态」**的文件。所有上线检查表描述的是「应该怎样」，本文档只记录「现在实际是什么」。
>
> 现状基准：**2026-08-19（服务器实测回填）**——SSH `root@8.146.200.82:22222` 逐项核验（Runbook §1–§5、§9），全部只读。仓库 env 文件与服务器**不一致**，以本节为准。

## 1. 总体状态（服务器实测 2026-08-19）

| 面 | 当前事实 | 证据 |
|---|---|---|
| 生产容器 | **20 个容器全部运行**，镜像 SHA 一致 `5c40cf0f`（Up 2 天）；postgres/redis Up 5 天 healthy；console-gateway 等 8 个服务 healthy，其余 Up | 服务器 `docker ps` |
| 服务器环境 | `/opt/henukit/.env.henukit`（0600 root，2026-08-19 14:08 更新）**16/18 关键键存在**；缺 `LIBRARY_DOWNLOAD_SERVICE_CLIENT_ID`、`NOTICE_DELIVERY_URL` | env 键矩阵 |
| QuizCraft 开关 | **全部开启**：`PORTAL_ENABLE_QUIZCRAFT_CATALOG=1`、`PORTAL_ENABLE_QUIZCRAFT_V2_READS=1`、`PORTAL_PRACTICE_COMMANDS_ENABLED=1`、`ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=1` | env 值 |
| 边缘路由 | `/` 200、`/practice` 200、`/library` 200、`/console/` 302→console 子域、`/study-api/healthz` 404、`/account-auth/healthz` 404 | 服务器 curl |
| 证书 | henukit.cn（至 2026-10-28）✓、console.henukit.cn（至 2026-11-01）✓；**study/quiz/account.henukit.cn 无专用证书**（回落 superhuazai.me 默认证书） | openssl SNI |
| 旧栈残留 | **无**：无 study-api/worker/web/admin 容器；宿主机无 8080 监听；superhuazai.me 全站 301 到 henukit.cn 栈 | 容器清单/端口 |
| 磁盘 | **89% 已用（剩余 4.5G）⚠️** | df -h |
| 审计 | 2026-07-24 审计报告 NO-GO（当时未核验）；本回填即 2026-08-19 实测 | 本文件 |

**一句话**：新栈 20 服务**已全量在生产运行**；QuizCraft Go 切流**已实际完成**（见 §3）；旧栈已物理退役；剩余缺口集中在「子域落地、排行榜结算、library legacy 适配、通知投递、文档同步」。

## 2. 双 Postgres 拓扑（关键架构事实）

| 实例 | 位置 | 用途 |
|---|---|---|
| **宿主机 postgres 16**（`127.0.0.1:5432`） | 非容器 | `quizcraft`（FastAPI 旧表：question_banks 等，8 表）与 **`quizcraft_v2`（Go 契约表，完整数据）** |
| **docker postgres 17**（容器，无端口映射） | 容器 | henukit 栈 9 库：study / platform / account_portfolio / quizcraft / notice / library / food / career / portal |

注意：docker 里的 `quizcraft` 库是 **FastAPI 表结构且无 Go 表**（portal-api 直读的是它 → 空目录）；Go core 用的是宿主机 postgres 的 `quizcraft_v2`。**两套 quizcraft 数据不在同一实例**。

## 3. QuizCraft 现状（服务器实测：Go 已切流）

| 项 | 状态 |
|---|---|
| **Go core** | **容器化完成并已部署（2026-08-20，release `8b018a27`）**：compose 服务 `quizcraft`（`henukit-quizcraft` 镜像，`Up` 稳定运行）；宿主机 systemd `quizcraft-go.service` **已停用**（宿主 10089 无监听）；`QUIZCRAFT_V2_DATABASE_URL` 指向 `host.docker.internal:5432`（宿主机 postgres 16 `quizcraft_v2`，切流过渡拓扑）；宿主机 postgres 已开 `listen_addresses=localhost,172.17.0.1` + pg_hba 放行 172.17.0.0/16 与 172.19.0.0/16（docker 网段，scram-sha-256） |
| **数据** | `quizcraft_v2`：**12 题库 / 3457 题 / 45+ 个真实 practice session**；迁移表齐（migration_runs/exceptions/receipts、legacy_ranking_snapshots、legacy_feedback_archives） |
| **排行** | `quizcraft_ranking_settlement_events` **3 条**（2026-08-19 首次结算，settleranking 已运行；公开排行 entries 暂空——需用户练习产生数据） |
| **FastAPI** | 已停（10086 无监听，无 uvicorn 进程）；`LEGACY_BASE_URL=http://127.0.0.1:10086` 为历史配置残留 |
| **portal 读** | `/api/v1/practice/catalog`、`/api/v1/rankings/overall` 均 200（走容器内 Go core）；`/api/v1/practice/banks` **404**（ADR-0036/0038 已部署：legacy 幽灵直读路径关闭） |
| **结论** | ADR-0013 切流**已执行**（8-14 起），但仓库文档（README 8-08「FastAPI live」、本文件旧版「未切流」）**全部过时**；无在库 cutover 证据文档 |

## 4. 接线缺口（服务器实测后缩小为 4 项）

| 缺口 | 实况 | 影响 |
|---|---|---|
| `STUDY_LEGACY_API_URL`（已移除） | **ADR-0037 已删 adapter**（代码层完成 2026-08-19）：命令路由诚实 503，workspace 降级 partial；服务器 env 清理随下次部署 | 待部署后从服务器 env 移除该键 |
| `NOTICE_DELIVERY_URL` 缺失 | 未配置 | 通知分发永远排队（设计内降级） |
| `LIBRARY_DOWNLOAD_SERVICE_CLIENT_ID` 缺失 | 未配置 | 该键在网关侧另有默认；需确认下载 grant 链路实际可用 |
| study/quiz/account 子域 | 无专用 vhost/证书 | 未落地（域名迁移尚未做） |

## 5. 数据现状

- **study 库**：courses **24**、materials **715**（生产数据，Portal `/library` 数据源）
- **library 库**：6 张表就位（public_releases、material_snapshots、activation_events、download_start_events、adapter_*）——OSS 激活管线表结构已建
- **platform 库**：24 张表（users/sessions/oauth_clients/authorization_codes/mail_outbox/verification_codes/permission_codes…）；oauth_clients 回调已注册（portal-gateway：superhuazai+henukit.cn；console-gateway：含 console.henukit.cn）✓
- **account_portfolio 库**：18 张表（会员/订单/支付表就位，EasyPay 开启）

## 6. 待办清单（按优先级）

1. **文档同步（本轮已部分完成）**：本文件回填服务器真相；`ALL_NEW_STACK_CUTOVER.md`、quizcraft README 的「未切流」表述需修正（后续批次）
2. **QuizCraft Go core 容器化部署（方案 2，配置层已完成 2026-08-20）**：随下次发布激活 `quizcraft` 容器服务后——停 `quizcraft-go.service`（`systemctl disable --now`）、把 `/etc/quizcraft-go.env` 的键迁入 `/opt/henukit/.env.henukit`（QUIZCRAFT_V2_DATABASE_URL / AUTH_HMAC_SECRET / CUTOVER_EVIDENCE_SECRET / PORTAL_CATALOG+COMMAND+SUMMARY 凭据 / WRITES_ENABLED / PORTAL_COMMANDS_ENABLED）、`QUIZCRAFT_CORE_URL` 改为 `http://quizcraft:10089`；核验 `/api/v1/practice/catalog` 等路由 200 后移除 `extra_hosts`（quizcraft_v2 迁入 docker postgres 时）
3. **部署 M1（ADR-0036）代码**：生产 gateway 仍是旧代码（直读幽灵路径 200-空目录）；部署后 `/api/v1/practice/banks` 等变 404+迁移提示，catalog/rankings 不变
4. **排行结算**：settleranking 从未运行（0 settlements）；决定是否启用公开排行
5. **library legacy 适配**：✅ **已按 ADR-0037 移除 adapter**（代码层，测试全绿）；服务器 env 的 `STUDY_LEGACY_API_URL`/`STUDY_LEGACY_ADMIN_TOKEN` 清理随下次部署；T1 数据迁移后恢复目录数据
6. **磁盘 89%**：清理旧镜像/日志或扩容，避免写满
7. **子域**：study/quiz/account.henukit.cn 落地需过边界批准（`henukit-domain-cutover.md`）
8. **通知投递**：决定 NOTICE_DELIVERY_URL 接线或明确保持关闭
9. **密钥轮换**：`.env.henukit.prod`（repo）含疑似真实密钥，服务器 env 与 repo 不同步，统一后轮换

## 7. 更新规则

1. 任何生产部署、切流、域名或数据 owner 变更落地后，由执行者更新本文档并在「现状基准」处记录日期。
2. 只写已核验的事实；计划与目标写入对应检查表/ADR，不写进本文档。
3. 本文档与检查表冲突时，以本文档（现状）为准并修漂移。
