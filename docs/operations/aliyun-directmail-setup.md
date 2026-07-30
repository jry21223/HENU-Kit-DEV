# 阿里云邮件推送（DirectMail）配置清单

HENU Kit 的生产事务邮件身份为：

```text
HENU Kit <noreply@notify.henukit.cn>
```

`notify.henukit.cn` 是独立的发信子域。它把验证码邮件的信誉、SPF、DKIM
和 DMARC 与主站 `henukit.cn` 隔离。`noreply` 地址不接收用户回复；如果以后
需要处理回复，应另设受监控的 `Reply-To`，不要假装读取 noreply 邮箱。

## DirectMail 配置

在华东 1（杭州）的邮件推送控制台中配置：

| 项目 | 生产值 |
|---|---|
| 发信域名 | `notify.henukit.cn` |
| 发信地址 | `noreply@notify.henukit.cn` |
| 类型 | 触发邮件 |
| SMTP | `smtpdm.aliyun.com:465` |

发信域名必须保持“验证通过”，发信地址必须保持“正常”。SMTP 密码是专用
凭据，不是阿里云登录密码。

## DNS 记录

DirectMail 控制台展示的记录值是唯一事实来源。至少包括：

- 控制台要求的域名归属 TXT 或 CNAME，主机名与值必须原样配置；
- `notify.henukit.cn` 的 SPF TXT；
- `notify.henukit.cn` 的 MX；
- `aliyun-cn-hangzhou._domainkey.notify.henukit.cn` 的 DKIM TXT；
- `_dmarc.notify.henukit.cn` 的 DMARC TXT，初始策略为 `p=none`。

如果权威 DNS 在 Cloudflare，所有邮件记录必须为 **DNS only**。不得代理
MX、TXT 或 DKIM。切换权威 DNS 前，先在新 DNS 中完整复制并核对这些记录。

## 运行时密钥

生产环境文件只保存于受控服务器，权限应为 `0600`，不得提交：

```env
PLATFORM_CORE_SMTP_ADDRESS=smtpdm.aliyun.com:465
PLATFORM_CORE_SMTP_USERNAME=noreply@notify.henukit.cn
PLATFORM_CORE_SMTP_PASSWORD=<smtp-specific-password>
PLATFORM_CORE_SMTP_FROM=noreply@notify.henukit.cn
PLATFORM_CORE_SMTP_MESSAGE_ID_DOMAIN=notify.henukit.cn
```

应用生成的 `Message-ID` 也使用 `notify.henukit.cn`，避免继续引用旧域名。

## 切换与回退

1. 在新权威 DNS 中复制 SPF、MX、DKIM、DMARC，并用公共解析器核对。
2. 确认 DirectMail 发信域名仍为“验证通过”。
3. 仅更新 SMTP 用户名、From 和 Message-ID 域；保留旧 SMTP 身份作为回退。
4. 重建并只重启 `platform-smtp-provider` 与 `platform-mail-worker`。
5. 从真实登录流程发送一封验证码邮件，核对 outbox、供应商投递记录和收件箱。
6. 回退时恢复旧 SMTP 用户名、From 与密码，并重建上述两个服务。

日志不得记录 SMTP 密码、验证码、完整收件地址或邮件正文。
