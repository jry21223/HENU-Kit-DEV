# 生产发布总检查表

> 更新日期：2026-07-24
> Webhook 开发前代码基线：`main@83c5b7c99fc4a333695e0d59e73c45bc5b9105a8`
> Webhook 实施票：`#103`；候选分支：`feature/infra/hc-103`
> 用途：作为 HENU Kit 全套服务上线前唯一的 Go/No-Go 汇总入口。
> 重要：Issue 关闭、PR 合并、Webhook 收到 `2xx` 或 CI 成功，都**不表示生产发布已经完成**。没有绑定最终 Release SHA 的服务器、数据、恢复、浏览器、真实依赖与观察证据时，结论必须是 **NO-GO**。

## 1. 仓库清理边界

### 已从默认分支工作树移除

- `legacy/v1-next-prisma/` 中不再参与运行的旧 Next.js + Prisma V1 源码；只保留定位说明，完整内容仍可从 Git 历史恢复。
- `docs/archive/` 中旧 Study V2 文档正文和过期阶段总结；只保留定位说明。
- `archive/henukit-planning/` 中公开 `HENU-Kit` 仓库的重复快照；公开说明继续以公开仓为事实来源。
- Foundation 阶段一次性使用的 `.github/workflows/monorepo-import.yml` 与 `scripts/monorepo/import-products.sh`。

### 明确保留

以下内容属于当前运行、兼容、恢复或审计边界，不按“历史文件”删除：

- `apps/study-legacy-admin/` 及其回滚入口。
- `products/quizcraft/` 当前 QuizCraft 运行代码与迁移事实。
- Platform Core、Portal、Study、Console、Notice、Food、Library、QuizCraft 的 OpenAPI、Migration、测试、部署与恢复脚本。
- 数据库备份、Migration 历史、生产 Runbook、审计和回滚证据。
- Git 历史中的所有已删除文件。

## 2. GitHub Tracker 与实施证据

### 已有实现证据

- `#77`：由已合并 PR `#83` 完成；包含 Session 凭据只存 Hash、24 小时清理路径、`henu.edu.cn` 固定边界、并发/重放测试和 Migration 证据。
- `#87`：由已合并 PR `#92` 完成；登录安全随机源不可用时 Fail Closed，并补充正式非官方声明。

### 已收敛但仍是生产硬门禁

- `#44 / #79 / #80 / #81`：QuizCraft 技术停写、最终对账、旧库只读、跨浏览器/移动端验证、全量切换及七天冷备。
- `#45 / #88 / #89 / #90`：可信 Reviewer 身份、高风险 PR 隔离和最终累计发布复审。
- `#93`：Portal 安全与绿色基线。
- `#94–#101`：Portal 的 Library、Practice、Food、Campus、Notice 真实数据与契约迁移。
- `#103`：安全的 GitHub Webhook 自动同步与精确 SHA 发布代理；代码完成后仍需服务器安装、HTTPS、Deploy Key、Hook、Ping/Push、回滚和观察证据。

关闭历史 Issue 只是 Tracker 整理，不是“已完成”声明。下列对应检查未全部通过时必须判定 **NO-GO**。

## 3. 发布候选固定

- [ ] 记录最终 `main` 完整 SHA：`________________________________________`
- [ ] 记录最终 PR、Required Checks、独立 Developer/Tester Review 与人工批准证据。
- [ ] 每个部署单元的 Artifact/Image 均绑定精确 SHA，不使用 `latest` 作为发布证据。
- [ ] 记录 Portal、Console、Platform Core、Platform Worker、Study Web、Study Admin、Study API、Study Worker、Quiz Web、Quiz API/Go Service、Notice、Food、Library、Portal API、Portal Gateway 的实际部署版本。
- [ ] 发布说明列出本次包含与不包含的模块、Migration、Feature Flag、切流顺序和回滚边界。
- [ ] “代码已可发布”和“生产已部署”分别记录，CI 成功不能替代服务器部署证据。

## 4. 代码、契约与供应链检查

- [ ] 所有 Go 模块通过 `gofmt`、`go vet`、`staticcheck`、`go test -race`、`govulncheck` 和二进制构建。
- [ ] 前端通过 `pnpm install --frozen-lockfile`、lint、类型检查、单元测试、生产构建和必要的 Playwright。
- [ ] QuizCraft Python/FastAPI 兼容路径通过格式/静态检查、pytest、后端 Smoke 和数据库一致性测试。
- [ ] 所有 OpenAPI 3.1、JSON Schema、生成代码漂移和 breaking-change 检查通过。
- [ ] Compose、Dockerfile、镜像构建和镜像漏洞扫描通过。
- [ ] Secret Scan、敏感日志扫描、大文件和许可证检查通过。
- [ ] 仓库治理使用可信权限或受保护配置识别 Reviewer；审批正文自报角色不能单独授予审核资格。
- [ ] 高风险 PR 只包含一个主要风险类别，并明确影响单元、发布顺序和回滚边界。

## 5. 数据库、Migration 与恢复检查

- [ ] 所有相关数据库在发布前完成校验和备份，记录 SHA-256、时间、大小、数据库版本和存放位置。
- [ ] 在隔离环境真实恢复备份，并通过计数、Hash、关键查询、Readiness 和业务 Smoke。
- [ ] 所有本次 Migration 完成前置检查、锁/执行时间评估、Up/重复 Up/Down/Up 或等价的可恢复验证。
- [ ] 生产不执行未审计的破坏性 AutoMigrate；采用 `expand -> migrate -> contract`。
- [ ] QuizCraft 最终 catch-up 后重新计算题库、题目、题型、答案、章节、反馈、旧排行和内容 Hash。
- [ ] 对账绑定本次 Migration Run 与 Source Head；未解决异常为零。
- [ ] 通过旧数据库连接执行安全写入探针，确认旧角色在切流后确实只读。
- [ ] `cursor=head`、计数与 Hash 一致后才允许生成最终双库快照和开启 Go 写入。

## 6. 账户、权限与安全检查

- [ ] 随机源故障时 CSRF、设备标识、幂等键、验证码、Session 和加密路径全部 Fail Closed，且没有 Cookie、Session、验证码记录或邮件副作用。
- [ ] Session Token 仅持久化 Hash，不存在可恢复的完整 Token。
- [ ] 24 小时验证码、Nonce、幂等事实和相关加密负载清理任务已在生产调度并验证。
- [ ] 正式登录代码和生产配置只接受 `henu.edu.cn` 边界；没有可配置扩域漏洞。
- [ ] OAuth state、PKCE、精确 Callback、协议相对/外部 `return_to`、授权码单次交换与撤销全部通过。
- [ ] 15 天 Core Session、8 小时业务/Console Session、注销、Core 撤销及撤销后拒绝重新授权通过。
- [ ] 浏览器自报的 `user_id`、角色、会员和 Entitlement 不作为权限或所有权事实。
- [ ] 日志中不存在完整邮箱、验证码、Authorization、Cookie、Session Token、服务密钥、SMTP 密码或邮件正文。
- [ ] 所有正式页面展示“学生自主运营 · 非河南大学官方项目”。

## 7. Portal 真实数据与故障诚实性

若 Portal 纳入本次生产发布，下列项全部必需；否则必须关闭对应生产路由和入口，不能以 Mock 冒充上线：

- [ ] Portal API、Portal Gateway、Portal 前端格式化、类型检查、构建和 HTTP 边界回归通过。
- [ ] Library/Practice 新旧契约先并存并标记 Deprecated；消费者迁移完成且有证据后再移除旧契约。
- [ ] Library 课程与资料来自真实 Gateway 数据，选择上下文正确，不回退到无关静态样例。
- [ ] Practice 题库标识贯穿题库选择、题目加载、作答和统计；生产无硬编码 `ds-tree` 或固定题库回退。
- [ ] Food 首页与排行榜在异步响应后可靠重渲染。
- [ ] Practice 排行和统计请求失败时显示不可用，不生成伪数据。
- [ ] Campus Market 的 `isMine`、领取状态和身份语义由服务端计算。
- [ ] Notice 摘要由 Notice Owner 服务产生并经 Gateway 提供，Portal API 不返回伪成功空占位。
- [ ] 每个异步页面明确区分 `loading / empty / error / success`。
- [ ] 只有完全没有 Gateway 的开发环境可使用确定性 Mock；生产依赖失败不得伪装成功。

## 8. 浏览器、移动端与用户流程检查

- [ ] Chromium、Firefox、WebKit 均使用固定、不可变的发布目录验证。
- [ ] 公开登录、刷题、反馈、收藏、排行、Workshop、资料检索/下载和关键 Console 流程通过。
- [ ] 360px 与 390px 视口无溢出、遮挡、不可操作控件或键盘陷阱。
- [ ] 任一控制台错误、未解释 HTTP 错误、错误写入或身份泄漏均阻断发布。
- [ ] 浏览器报告、Trace、截图和测试版本绑定最终 Release SHA。

## 9. 邮件、事件与后台任务检查

- [ ] 真实 `henu.edu.cn` 测试邮箱收到当次验证码；验证码通过隐藏终端交互输入，不进入环境、参数或日志。
- [ ] 邮件成功、鉴权拒绝、请求错误、暂时失败、永久失败、重试和重放均产生脱敏结构化审计。
- [ ] Critical、Transactional、Digest 队列隔离；验证码不会被摘要邮件阻塞。
- [ ] Outbox/Consumer Group/Reclaim/重试/DLQ 在 Redis、Worker 重启和第三方超时下不丢不重。
- [ ] 重复事件不重复产生通知、积分、邮件或状态写入。

## 10. QuizCraft 切流与冷备检查

- [ ] 进入有技术证据的停写窗口，旧服务与 Go 的持久化写入都被阻断。
- [ ] 写入承诺点前失败可用最终快照恢复；承诺点后失败保持维护并只允许前向修复，除非完整反向同步与重新对账通过。
- [ ] 全量 Go 关键流程通过后停止 FastAPI，并证明实际生效的 Nginx 配置没有 Legacy Upstream。
- [ ] 旧数据库角色只读，记录冷备开始/结束时间、Legacy SHA、最终快照和零写审计位置。
- [ ] 七天冷备期间不向旧服务发送灰量或观察流量。
- [ ] 最终备份与 Hash 至少保留 30 天。

## 11. 部署、监控与回滚检查

- [ ] Staging 部署使用与生产相同的不可变 Artifact，并完成 Readiness、Contract、Smoke 和 E2E。
- [ ] 生产变更经人工批准，按单一部署单元逐步执行。
- [ ] Nginx、Systemd/容器编排、环境变量、Secrets、Deploy Key、Webhook 和服务器 Remote 已核验。
- [ ] 发布前后记录 5xx、延迟、登录成功率、队列积压、数据库连接、邮件错误和关键业务成功率。
- [ ] Readiness 失败或 5xx 超阈值立即停止放量并回滚对应部署单元。
- [ ] 应用回滚不回滚向前兼容 Migration；数据库恢复只在明确 Runbook 条件下执行。
- [ ] 生产 Smoke 与监控观察期结束后才允许标记发布完成。

## 12. GitHub Webhook 自动同步与发布检查

### 代码和配置

- [ ] `services/deploy-webhook` 的格式、Vet、Race Test、构建、漏洞扫描、ShellCheck、Systemd 校验和 Secret 扫描全部通过。
- [ ] GitHub 官方 HMAC-SHA256 测试向量通过；错误签名、错误仓库、错误分支、非法 SHA、超大 Payload 均 fail closed。
- [ ] 仅 `push` 到 `main` 入队；Ping 只验证连通性，Issue/PR/Review/Label 等事件不执行部署。
- [ ] Receiver 只监听 loopback；公网只通过 HTTPS Nginx 暴露精确 Webhook 路径。
- [ ] Receiver 与 Runner 分进程；HTTP 进程不执行部署命令，Runner 只调用固定绝对路径并以独立参数传递已验证元数据。
- [ ] Webhook Secret、只读 Deploy Key、known_hosts、部署配置、批准文件和 Hooks 的 Owner/Mode 检查通过，仓库无私钥或当前 Secret。
- [ ] 远端 URL、仓库、分支与 SHA 同时匹配 root-owned 策略；活动工作区不执行 `git pull`、stash 或请求控制的 shell。
- [ ] 发布目录为精确 SHA 的不可变 Git worktree；重试前恢复到干净提交树。

### 服务器实测

- [ ] `henukit-deploy-webhook.service` 启动并通过 loopback `/healthz`、`/readyz`。
- [ ] GitHub Webhook 已启用 SSL verification，Content type 为 `application/json`，只订阅 Push；Ping Delivery 为 2xx。
- [ ] 受控 Push 在 30 秒内收到 `202`，持久队列由 Systemd 异步串行消费。
- [ ] 重复成功 Delivery 不重复部署；失败 Delivery 在批准或 Redeliver 后可恢复；服务/主机重启可恢复 `running.json`。
- [ ] 连续 Push 会忽略过期 SHA，只部署仍等于 `origin/main` 的最新精确 SHA。
- [ ] 每个本次启用的部署单元均存在 root-owned `prepare / activate / verify / rollback` Hook，并完成失败注入与回滚演练。
- [ ] 首次发布和高风险路径默认要求完整 SHA 人工批准；批准不能使用分支名、短 SHA 或 `latest`。
- [ ] `/statusz`、`deployed-sha`、`/opt/henukit/current/REVISION`、各服务版本接口与实际运行 Artifact SHA 完全一致。
- [ ] 服务器 Webhook 验证完成后才设置 `HENUKIT_DEPLOY_MODE=webhook`，确认没有与 GitHub Actions 双重部署；break-glass `workflow_dispatch` 可用。
- [ ] Webhook 故障、GitHub 不可达、Deploy Key 失效、Hook 失败和 Nginx 故障均有可执行人工发布/回滚入口。

## 13. 最终审批记录

| 项目 | 负责人 | 证据位置 | 结论/时间 |
|---|---|---|---|
| Standards Review |  |  |  |
| Spec Review |  |  |  |
| Security Review |  |  |  |
| Data/Migration Review |  |  |  |
| Browser/Mobile Test |  |  |  |
| Backup/Restore |  |  |  |
| Webhook Live Activation |  |  |  |
| Staging Acceptance |  |  |  |
| Production Approval |  |  |  |
| Post-deploy Observation |  |  |  |

## 14. 停止条件

出现任一情况立即判定 **NO-GO**：

- 上述必需检查存在未完成、无证据或与最终 Release SHA 不一致。
- 仅有 CI 成功，没有实际部署、恢复、对账、浏览器或生产 Smoke 证据。
- Webhook 代码已合并但服务器尚未安装，或只收到 Ping/202 而没有 Hook、运行 SHA、Readiness 与回滚证据。
- 自动部署会绕过独立评审、生产人工批准、备份恢复、Migration 或 QuizCraft 写入承诺点。
- 发现敏感信息泄漏、错误身份信任、伪成功/伪数据或无法验证的数据库写入。
- 计划在同一窗口执行破坏性 Migration、仓库改名和 DNS/流量切换。
- 活跃回滚单元、Migration、备份、生产数据或审计证据被当作“旧文件”删除。

## 15. 历史内容恢复

清理的源码和文档仍在 Git 历史中。需要取证或临时恢复时，从清理提交的父提交检出：

```bash
git checkout <cleanup-parent-sha> -- legacy/v1-next-prisma
git checkout <cleanup-parent-sha> -- docs/archive
git checkout <cleanup-parent-sha> -- archive/henukit-planning
```

也可以整体 Revert 清理提交。恢复历史文件不等于允许将旧应用重新投入生产。
