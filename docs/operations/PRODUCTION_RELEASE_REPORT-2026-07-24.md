# HENU Kit 生产发布审计报告（2026-07-24）

## 结论

**当前结论：NO-GO。**

本次变更建立的是 GitHub Webhook 自动同步与精确 SHA 发布基础设施，不是一次已经在生产服务器完成的全套服务上线。没有服务器访问、生产 Secret、数据库、真实邮箱、域名/Nginx、部署单元版本和监控环境，不能把代码、PR 或 CI 结果写成生产成功。

## 发布候选

| 项目 | 当前事实 |
|---|---|
| 开发前 `main` | `83c5b7c99fc4a333695e0d59e73c45bc5b9105a8` |
| 实施 Issue | `#103` |
| 实施分支 | `feature/infra/hc-103` |
| 最终 PR / Head SHA | 待 GitHub PR 创建后记录 |
| 最终 `main` SHA | 未固定 |
| 生产运行 SHA | 未核验 |
| 生产部署时间 | 未执行 |

## 本次代码交付

- 独立 Go Webhook Receiver；HMAC-SHA256、Payload 上限、Delivery、仓库、分支与完整 SHA 校验。
- Receiver/Runner 进程隔离；快速 `202`、持久队列、成功去重、失败重试和重启恢复。
- 只读 Deploy Key、精确 `origin/main` SHA、远端 URL 策略、不可变 worktree 与 stale Push 拒绝。
- Root-owned Hook 协议：`prepare -> activate -> verify -> rollback`。
- 首次发布和高风险路径完整 SHA 人工批准。
- Systemd、Nginx、安装器、Study Hook、通用 Hook、运维 Runbook 与 CI。
- 旧 GitHub Actions Study 发布保留为 break-glass，并增加路径过滤和 Webhook 切换变量，避免双重部署。

## 已执行的本地验证

| 检查 | 结果 | 限制 |
|---|---|---|
| Go Unit/Integration Tests | 通过 | 当前执行环境仅有 Go 1.23.2；临时将模块兼容指令改为 1.23.2 后测试，随后恢复仓库要求的 Go 1.26.5。正式结果以 PR CI 的 Go 1.26.5 为准。 |
| `go vet ./...` | 通过 | 同上 |
| Deploy Script Git Fixture | 通过 | 覆盖精确 SHA、stale、remote URL、worktree、Hook 和批准门禁。 |
| Shell `bash -n` | 通过 | 不替代 ShellCheck。 |
| Study Workflow Node Tests | 通过 | 3 项通过。 |
| Systemd Unit Verify | 通过 | 使用临时占位可执行文件完成静态验证。 |
| Secret/Private Key Repository Scan | 待 PR CI | CI 已配置。 |
| `govulncheck` / Race / ShellCheck | 待 PR CI | 本地环境未提供匹配 Go 1.26.5 与 ShellCheck。 |

## 尚未完成的生产硬门禁

- Webhook 服务器安装、Root-only Secret、只读 Deploy Key、known_hosts 和 GitHub Webhook 配置。
- HTTPS 域名、Nginx 配置、GitHub Ping 与受控 Push 实测。
- 所有真实部署单元的 prepare/activate/verify/rollback Hook。
- 每个部署单元的实际运行 SHA、Readiness、Smoke、监控和回滚演练。
- 独立 Developer/Tester 当前提交评审与生产人工批准。
- 全仓最终 CI、供应链、镜像和 Secret 扫描。
- 数据库备份、SHA-256、隔离恢复、Migration 和数据对账。
- 真实 `henu.edu.cn` 邮件、OAuth、Session、撤销和脱敏日志验证。
- Chromium、Firefox、WebKit、360px、390px 和关键业务 E2E。
- Portal 真实数据/错误状态门禁。
- QuizCraft 技术停写、最终对账、旧库只读、Go 写入承诺点、Nginx 切流与七天冷备。
- 生产观察期和最终负责人签字。

## 上线顺序

1. 合并并固定经过独立评审的 Webhook PR。
2. 在 Staging/目标服务器安装 Receiver，但暂不启用 queue watcher。
3. 注册只读 Deploy Key，核验 SSH Host Key，克隆私有仓库。
4. 配置并演练所有目标部署单元 Hook 与回滚。
5. 配置 HTTPS Nginx 和 GitHub Webhook，完成 Ping。
6. 完成备份恢复、Migration、真实依赖、浏览器和业务门禁。
7. 启用 queue watcher，批准最终完整 SHA，执行受控 Push。
8. 核对 GitHub Delivery、队列、运行版本、Readiness、Smoke 和指标。
9. 确认 Webhook 稳定后再设置 `HENUKIT_DEPLOY_MODE=webhook`，避免与 Actions 双重部署。
10. 完成规定观察期后，由负责人签署 Production Approval。

在上述证据齐全前，禁止对外声明“整套服务已完成生产发布”。
