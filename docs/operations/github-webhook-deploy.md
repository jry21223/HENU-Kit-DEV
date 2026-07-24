# GitHub Webhook 自动同步与发布 Runbook

> 适用仓库：`jry21223/HENU-Kit-DEV`
> 目标分支：`main`
> 结论边界：Webhook 负责安全接收、同步、排队和执行既定发布 Hook；它不替代数据库、Migration、真实邮箱、浏览器、备份恢复和人工生产批准。

## 1. 设计目标

每次 `main` 更新后，GitHub 向服务器发送 `push` Webhook。服务器在 30 秒内完成签名和边界校验并返回 `202`，随后由 Systemd 异步、串行地处理部署。

```text
GitHub push(main)
  -> HTTPS /webhooks/github
  -> HMAC-SHA256 / repository / branch / SHA / delivery validation
  -> persistent queue
  -> systemd path
  -> unprivileged deploy runner
  -> exact origin/main SHA fetch
  -> immutable release worktree
  -> root-owned prepare / activate / verify / rollback hooks
```

Webhook 只接受：

- `ping`：用于连通性验证；
- `push`：仅 `jry21223/HENU-Kit-DEV` 的 `refs/heads/main`；
- `after` 为合法完整 Git SHA，且部署时必须仍等于远端 `origin/main`。

Issue、PR、Review、Label、Release 等事件均返回 `202 ignored`，不会触发部署。

## 2. 安全约束

1. 外部只暴露 Nginx HTTPS；Go 进程固定监听 `127.0.0.1` 或 `::1`。
2. Webhook Secret 为至少 256 bit 随机值，保存在 root-only 文件，通过 Systemd Credential 注入。
3. 原始请求体使用 `X-Hub-Signature-256` 验证，比较采用常量时间 HMAC。
4. Deploy Key 只授予仓库读取权限，不授予写权限。
5. HTTP 接收进程不执行命令；Systemd `.path` 单独启动 oneshot runner。
6. Runner 只执行固定绝对路径命令，SHA、Delivery、Repository、Ref 作为独立参数传递，不拼接来自请求的 shell 程序。
7. 发布工作区按完整 SHA 创建，不在当前活动目录 `git pull`，不 stash 服务器改动。
8. 过期 Delivery 会因事件 SHA 不等于当前 `origin/main` 而安全忽略。
9. `prepare` 完成前不改变在线流量；`activate` 或 `verify` 失败时按逆序运行 `rollback`。
10. 首次发布及 Migration、认证、契约、基础设施、Platform Core、切流脚本等高风险变更默认要求 root 显式批准 SHA。

## 3. 服务器前置条件

- Linux + Systemd + Nginx；
- Go `1.26.5`；
- Git、OpenSSH、OpenSSL、`flock`；
- 对需要在服务器构建的 Hook，安装对应 Node 22、pnpm 11.9、Python 或 Docker；
- 一个对外 HTTPS 域名，例如 `deploy.henukit.cn`；
- 服务器时间同步正常；
- 当前服务已有可验证的应用回滚方式。

## 4. 一次性安装

在仓库根目录执行：

```bash
sudo services/deploy-webhook/deploy/install.sh
```

需要直接启用现有 Study 发布 Hook 时：

```bash
sudo services/deploy-webhook/deploy/install.sh --enable-study-hook
```

需要使用一个服务器自定义总发布命令时：

```bash
sudo services/deploy-webhook/deploy/install.sh --enable-command-hook
sudoedit /etc/henukit-deploy/deploy.env
```

安装器会：

- 创建 `henukit-deploy` 非登录用户；
- 编译并安装 `henukit-deploy-webhook`；
- 安装 receiver、path、runner 三个 Systemd unit；
- 生成 `/etc/henukit-deploy/webhook-secret`；
- 生成只读 Deploy Key；
- 创建状态、release、approval 和 hook 目录；
- 启动 receiver；queue watcher 默认保持禁用，直到仓库、Hook、HTTPS 和回滚验证完成；
- 输出 Secret 指纹和 Deploy Key 公钥。

安装器不会自动把公钥提交到 GitHub，也不会自动编辑 Nginx、DNS 或生产 Hook。

## 5. 配置只读 Deploy Key

查看安装器生成的公钥：

```bash
sudo cat /var/lib/henukit-deploy/.ssh/id_ed25519.pub
```

在 GitHub 仓库中进入：

```text
Settings -> Deploy keys -> Add deploy key
```

- Title：`henukit-production-readonly`
- Key：粘贴公钥
- **不要**勾选 Allow write access

从可信来源核验并写入 `github.com` SSH host key：

```bash
sudo install -o henukit-deploy -g henukit-deploy -m 0600 /path/to/verified_known_hosts \
  /var/lib/henukit-deploy/.ssh/known_hosts
```

然后克隆：

```bash
sudo -u henukit-deploy git clone --branch main --single-branch \
  git@github.com:jry21223/HENU-Kit-DEV.git /opt/henukit/repository
```

验证 remote 与当前 SHA，并确认 `/etc/henukit-deploy/deploy.env` 中的 `HENUKIT_EXPECTED_REMOTE_URL` 与输出完全一致：

```bash
sudo -u henukit-deploy git -C /opt/henukit/repository remote -v
sudo -u henukit-deploy git -C /opt/henukit/repository fetch --prune origin main
sudo -u henukit-deploy git -C /opt/henukit/repository rev-parse origin/main
sudo grep '^HENUKIT_EXPECTED_REMOTE_URL=' /etc/henukit-deploy/deploy.env
```

Remote URL 发生漂移时发布会 fail closed；不要通过修改 Git 工作区规避 root-owned 策略。

## 6. 配置发布 Hook

Hook 位于 `/etc/henukit-deploy/hooks.d`，必须 root-owned 且不可被 group/other 写入。模板安装在：

```text
/usr/share/doc/henukit-deploy-webhook/hooks/
```

### 6.1 通用总发布命令

```bash
sudo install -o root -g root -m 0755 \
  /usr/share/doc/henukit-deploy-webhook/hooks/10-command.example \
  /etc/henukit-deploy/hooks.d/10-command
sudoedit /etc/henukit-deploy/deploy.env
```

示例：

```env
HENUKIT_PREPARE_COMMAND=/usr/local/libexec/henukit/release-all prepare
HENUKIT_ACTIVATE_COMMAND=/usr/local/libexec/henukit/release-all activate
HENUKIT_VERIFY_COMMAND=/usr/local/libexec/henukit/release-all verify
HENUKIT_ROLLBACK_COMMAND=/usr/local/libexec/henukit/release-all rollback
```

命令必须非交互、幂等，并根据 `HENUKIT_RELEASE_DIR` 与 `HENUKIT_RELEASE_SHA` 操作。

### 6.2 Study 现有 artifact 发布

```bash
sudo install -o root -g root -m 0755 \
  /usr/share/doc/henukit-deploy-webhook/hooks/20-study.example \
  /etc/henukit-deploy/hooks.d/20-study
```

该 Hook 复用仓库现有构建结构和 `/usr/local/bin/final-review-study-deploy`，依次执行：

- frozen pnpm install 与 ops tests；
- Go API/Worker 构建；
- Next.js 与 Study Legacy Admin 构建；
- SHA artifact 打包；
- 既有服务器原子部署脚本；
- `/readyz`、主页和 `deploy-probe.txt` 验证。

若现有部署脚本需要 sudo，只授予精确命令的 NOPASSWD 规则，禁止给 `henukit-deploy` 通用 root shell。

### 6.3 其他服务

Portal、Console、Platform Core、Notice、Food、Library、QuizCraft 等部署单元应分别实现 Hook 或由统一 `release-all` 驱动。每个 Hook 都必须实现：

```text
prepare -> activate -> verify -> rollback
```

QuizCraft 的不可逆 Go 写切换不得由普通 push 自动跨越承诺点；它仍需执行专用 cutover gate 和人工批准。

## 7. Nginx 与 HTTPS

将 `services/deploy-webhook/deploy/nginx.conf.example` 合入 HTTPS vhost：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

外部 Payload URL 示例：

```text
https://deploy.henukit.cn/webhooks/github
```

不要把 `127.0.0.1:10087` 直接暴露到公网。`/statusz` 应保持 loopback-only 或受管理网络保护。

## 8. 创建 GitHub Webhook

读取 Secret 时避免进入 shell history或聊天记录：

```bash
sudo cat /etc/henukit-deploy/webhook-secret
```

在仓库中进入：

```text
Settings -> Webhooks -> Add webhook
```

配置：

- Payload URL：`https://deploy.henukit.cn/webhooks/github`
- Content type：`application/json`
- Secret：服务器文件内容
- SSL verification：Enable
- Events：Just the push event
- Active：启用

先使用 GitHub 的 Ping 交付确认 HTTP 200。确认只读 clone、root-owned Hook、回滚命令和 Nginx 均已验证后，再启用队列 watcher：

```bash
sudo systemctl enable --now henukit-deploy-webhook.path
```

随后执行一个可回滚的受控 `main` push。不要在 watcher 禁用期间发送真实 Push；若已发送，应在启用后检查队列和 GitHub Delivery。

## 9. 首次与高风险 SHA 批准

默认策略 `HENUKIT_REQUIRE_APPROVAL=high-risk`。当 `/statusz` 或 journal 显示 `manual approval is required` 时：

1. 固定完整 SHA；
2. 完成该 SHA 对应的代码、Migration、数据、恢复和安全检查；
3. 执行：

```bash
sudo henukit-approve-release <full-main-sha>
```

4. `henukit-approve-release` 会把该 SHA 最近一次失败的 Delivery 重新入队，并启动 oneshot runner。检查：

```bash
journalctl -u henukit-deploy-runner.service -n 100 --no-pager
curl --fail http://127.0.0.1:10087/statusz
```

若该 SHA 尚无失败 Delivery，批准仍会保留，但工具会提示需要在 GitHub Delivery 页面 Redeliver，或等待下一次目标 Push。批准文件只认可同一完整 SHA，不允许批准分支名、短 SHA 或 `latest`。

## 10. 验证与观察

```bash
systemctl status henukit-deploy-webhook.service henukit-deploy-webhook.path henukit-deploy-runner.service
journalctl -u henukit-deploy-webhook.service -u henukit-deploy-runner.service --since today
curl --fail http://127.0.0.1:10087/healthz
curl --fail http://127.0.0.1:10087/readyz
curl --fail http://127.0.0.1:10087/statusz
cat /var/lib/henukit-deploy-webhook/deployed-sha
readlink -f /opt/henukit/current
```

必须核对：

- GitHub Delivery 返回 2xx；
- `last_success.event.after` 等于当前 `origin/main`；
- `deployed-sha`、release 目录、服务版本接口和实际运行 artifact SHA 一致；
- 所有部署单元 readiness/业务 smoke 通过；
- 5xx、延迟、登录、邮件、队列和数据库指标没有越过回滚阈值。

## 11. 与现有 GitHub Actions Study 部署切换

现有 `.github/workflows/deploy-study.yml` 保留为 break-glass fallback，并限制为 Study 相关路径。只有服务器 Webhook 已完成以下验证后，才设置仓库变量：

```text
HENUKIT_DEPLOY_MODE=webhook
```

设置后，Actions 的 Study deploy job 不再自动运行，避免同一 commit 被 GitHub Actions 与服务器 Webhook 双重部署。手工 `workflow_dispatch` 仍可作为回退入口；恢复 Actions 自动发布时删除或修改该变量。

## 12. 故障与回滚

- 错误签名、错误仓库、错误分支或非法 SHA：不入队。
- 重复成功 Delivery：返回 duplicate，不重复部署。
- 进程/主机重启：`running.json` 在 runner 重启时重新入队。
- 高风险批准：批准工具按完整 SHA 自动重排最近失败 Delivery；没有匹配事件时要求 GitHub Redeliver。
- 旧 Delivery 晚到：目标 SHA 不等于当前 `origin/main`，记录 stale 并跳过。
- `prepare` 失败：不执行在线切换。
- `activate`/`verify` 失败：按逆序调用全部 Hook 的 `rollback`。
- 某次部署失败：记录到 `failed/` 和 `last-failure.json`；后续修复 push 仍可继续处理。
- Webhook 故障：使用现有 GitHub Actions `workflow_dispatch` 或服务专用人工 Runbook。

禁止通过删除失败记录、修改 `deployed-sha` 或把 `HENUKIT_REQUIRE_APPROVAL` 临时设为 `never` 来伪造成功。

## 13. 交接清单

移交给下一位维护者时必须交付：

- GitHub 仓库管理员与 Webhook 管理权限；
- Webhook Secret 轮换流程，但不通过聊天或仓库传递当前 Secret；
- Deploy Key 公钥登记位置与私钥服务器路径；
- `/etc/henukit-deploy/webhook.env`、`deploy.env` 和 hooks.d 的脱敏说明；
- 每个 deploy unit 的 prepare/activate/verify/rollback 行为；
- 数据库备份恢复、Migration、Nginx、Systemd、域名和证书 Runbook；
- 最近成功生产 SHA、最近回滚 SHA和观察期证据；
- Break-glass GitHub Actions 与人工发布入口。

## 14. 上线判定

以下任一项未完成，结论均为 **NO-GO**：

- HTTPS Webhook、HMAC Secret、只读 Deploy Key、目标仓库/分支过滤未实测；
- 没有可执行 deployment Hook；
- 精确 SHA、stale、duplicate、restart、failure、rollback 演练未通过；
- 实际运行 SHA 无法从每个 deploy unit 读取；
- 数据库备份恢复、Migration、真实邮箱、浏览器矩阵或关键业务 smoke 未通过；
- 高风险 SHA 缺少可信独立评审和生产人工批准。
