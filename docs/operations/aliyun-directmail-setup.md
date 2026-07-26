# 阿里云邮件推送（DirectMail）配置清单

> 目标：让 HENU Kit 登录验证码能发到 `@henu.edu.cn` 邮箱。  
> 发信身份建议：`noreply@notify.superhuazai.me`（`superhuazai.me` 的 NS 在 Cloudflare）。  
> 程序侧已支持：`platform-mail-worker` + `platform-smtp-provider`（见 `docker-compose.henukit.yml`）。

你只需要在 **阿里云控制台** 和 **Cloudflare DNS** 按下面填；填完后把三行密钥写进本机 `.env.henukit`。

---

## 0. 你要准备的东西

| 项 | 建议值 |
|---|---|
| 发信域名 | `notify.superhuazai.me`（子域，不要用裸域乱发） |
| 发信地址 | `noreply@notify.superhuazai.me` |
| 显示名 | `HENU Kit` 或 `henukit 验证码`（不要写「河南大学官方」） |
| DNS 面板 | **Cloudflare**（因为 `superhuazai.me` 的 NS 在 CF） |
| 邮件产品 | 阿里云 **邮件推送 DirectMail** |

> `henukit.cn` 目前 NS 在万网且几乎空记录；发信先用已在 CF 的 `superhuazai.me` 更省事。

---

## 1. 开通阿里云邮件推送

1. 打开 [阿里云控制台](https://home.console.aliyun.com/)。
2. 搜索并进入 **邮件推送**（DirectMail）。
3. 若未开通：按提示开通（个人/企业均可；新账号常有免费试用额度，以控制台为准）。
4. 区域建议选 **华东 1（杭州）** 或控制台默认区（后面 SMTP 地址要和区域一致）。

常见 SMTP 地址（控制台「帮助 / SMTP 发信」里也会写，**以你控制台为准**）：

| 区域 | SMTP 地址 | 端口 |
|---|---|---|
| 华东 1（杭州）等国内 | `smtpdm.aliyun.com` | **465**（SSL）或 80/25（不推荐） |
| 新加坡等海外 | `smtpdm-ap-southeast-1.aliyuncs.com` 等 | 同样优先 **465** |

本仓库默认：

```env
PLATFORM_CORE_SMTP_ADDRESS=smtpdm.aliyun.com:465
```

若你开在海外区，把地址改成控制台给的那个。

---

## 2. 配置发信域名（阿里云）

1. 邮件推送 → **发信域名** → **新建域名**。  
2. 域名填：`notify.superhuazai.me`（只要子域，不要填成 `https://...`）。  
3. 创建后，控制台会给出一组 **DNS 记录**（至少包括）：

| 用途 | 一般类型 | 主机记录示例 | 记录值 |
|---|---|---|---|
| 所有权验证 | TXT | `阿里云给的前缀` 或 `@` 下某主机 | 一串 token |
| SPF | TXT | `notify` 或控制台写的主机 | 含 `include:spf.aliyun...` 之类 |
| DKIM | TXT 或 CNAME | 如 `aliyun-xxx._domainkey.notify` | 控制台长串 |
| MX（有的账号要） | MX | `notify` | 控制台给的 MX |

**关键：每一行都以你控制台「发信域名 → 配置」里显示的为准，不要凭记忆编。**

4. 先不要点「验证成功」——先去 Cloudflare 加记录。

---

## 3. 在 Cloudflare 填写 DNS（`superhuazai.me`）

1. 打开 [Cloudflare Dashboard](https://dash.cloudflare.com/) → 选中 **`superhuazai.me`**。  
2. 左侧 **DNS** → **Records** → **Add record**。  
3. 把阿里云给的每一条原样加上：

### 填写规则（必看）

| Cloudflare 字段 | 怎么填 |
|---|---|
| Type | 与阿里云一致（TXT / CNAME / MX） |
| Name | 阿里云「主机记录」。若阿里云写 `notify`，CF Name 填 `notify`；若写 `xxx._domainkey.notify`，就填完整主机名 |
| Content | 阿里云「记录值」（TXT 有时要带引号，CF 一般直接粘贴） |
| Proxy status | **DNS only（灰云）** ← 邮件相关记录全部关闭橙云 |
| TTL | Auto |

4. 全部加上后，回阿里云发信域名页点 **验证**。  
5. 变成「验证成功 / 正常」再继续（DNS 传播可能要几分钟到几小时）。

### 建议再加 DMARC（可选但推荐）

在 CF 为 `superhuazai.me` 增加：

| Type | Name | Content | Proxy |
|---|---|---|---|
| TXT | `_dmarc.notify` | `v=DMARC1; p=none; rua=mailto:你的监控邮箱@...` | DNS only |

先 `p=none` 只观察，不要一上来 quarantine/reject。

---

## 4. 配置发信地址（阿里云）

1. 邮件推送 → **发信地址** → **新建发信地址**。  
2. 邮箱：`noreply`（完整将是 `noreply@notify.superhuazai.me`）。  
3. 回复地址：可填你自己常用邮箱（用户点回复时用）。  
4. 类型：触发邮件 / 验证码类（按控制台选项选「触发」更贴切）。  
5. 保存。

---

## 5. 开 SMTP 并拿密码（阿里云）

1. 找到刚建的发信地址 → **设置 SMTP 密码** / **SMTP 发信设置**。  
2. 设置一枚 **SMTP 专用密码**（不是阿里云登录密码）。  
3. 记下（复制到密码管理器）：

| 项 | 填到哪里 |
|---|---|
| SMTP 地址 | `.env.henukit` → `PLATFORM_CORE_SMTP_ADDRESS` |
| SMTP 用户名 | 一般是 **完整发信地址** `noreply@notify.superhuazai.me` → `PLATFORM_CORE_SMTP_USERNAME` |
| SMTP 密码 | 刚设的 SMTP 密码 → `PLATFORM_CORE_SMTP_PASSWORD` |
| From | 同一发信地址 → `PLATFORM_CORE_SMTP_FROM` |

> 部分控制台用户名显示为发信地址；若文档写「邮箱账号」，就用完整 `noreply@...`。

---

## 6. 本机 `.env.henukit` 必填三行

编辑 `/Users/jerry/Documents/校园平台/.env.henukit`：

```env
PLATFORM_CORE_SMTP_ADDRESS=smtpdm.aliyun.com:465
PLATFORM_CORE_SMTP_USERNAME=noreply@notify.superhuazai.me
PLATFORM_CORE_SMTP_PASSWORD=这里粘贴阿里云SMTP密码
PLATFORM_CORE_SMTP_FROM=noreply@notify.superhuazai.me
PLATFORM_CORE_MAIL_PROVIDER_TOKEN=replace-mail-provider-token-32chars-min!!
```

`PLATFORM_CORE_MAIL_PROVIDER_TOKEN` 只要是本机随机长串即可（worker 与 smtp-provider 共用），**不要提交到 Git**。

然后启动/重建邮件组件：

```bash
cd /Users/jerry/Documents/校园平台
docker compose -f docker-compose.henukit.yml --env-file .env.henukit up -d --build platform-smtp-provider platform-mail-worker
docker compose -f docker-compose.henukit.yml --env-file .env.henukit ps platform-smtp-provider platform-mail-worker
```

看队列是否被消费：

```bash
docker exec henukit-postgres-1 psql -U henukit -d platform \
  -c "SELECT id, status, attempt_count, created_at FROM mail_outbox ORDER BY created_at DESC LIMIT 5;"
```

- `pending` → 仍没投递（worker 没起或 SMTP 失败）  
- `accepted` / 成功态（以表结构为准）→ 已交给供应商  

---

## 7. 自测顺序

1. 打开 http://localhost:8088/account/login  
2. 输入河大邮箱前缀 → 发送验证码  
3. 查 `mail_outbox` 状态与 worker 日志：  
   `docker logs henukit-platform-mail-worker-1 --tail 50`  
   `docker logs henukit-platform-smtp-provider-1 --tail 50`  
4. 登录 **学校邮箱网页版**（含垃圾箱）查信。  
5. 若阿里云控制台 **投递记录** 显示成功但学校没有：多半是学校侧拦截，需看退信/投递详情。

---

## 8. 常见失败对照

| 现象 | 原因 | 处理 |
|---|---|---|
| outbox 一直 `pending`，attempt=0 | worker 没跑 | `docker compose ... up -d platform-mail-worker` |
| worker 报 provider 连不上 | smtp-provider 没起或地址错 | 检查 `PLATFORM_CORE_MAIL_PROVIDER_ENDPOINT` |
| provider 报 SMTP auth fail | 用户名/密码/区域地址错 | 对照控制台 SMTP 设置 |
| 阿里云域名验证失败 | CF 记错 Name 或仍是橙云 | 灰云；Name 用相对主机名 |
| 控制台成功、河大没有 | 学校拒信 / 进垃圾箱 | 查 DirectMail 投递失败原因；完善 DMARC；申请提高信誉 |
| From 不是已验证地址 | 随便写了 From | 必须与「发信地址」一致 |

---

## 9. 和 Cloudflare 的关系（再强调）

- **可以**用 Cloudflare 做 `superhuazai.me` 的 DNS。  
- 邮件记录必须 **DNS only（灰云）**。  
- Cloudflare Email Routing **只收不发**，不能替代 DirectMail。  
- 你之前「域名配不好」若指发信：多半是 **缺 SPF/DKIM + 应用未投递**；不是 CF 不能用。

---

## 10. 你填完后回我这 4 项即可（可打码密码）

1. 发信域名是否「验证成功」  
2. 发信地址完整邮箱  
3. SMTP 地址（是否 `smtpdm.aliyun.com:465`）  
4. 本机 `mail_outbox` 最新一条的 `status`  

我可以再帮你看日志是否真发出。
