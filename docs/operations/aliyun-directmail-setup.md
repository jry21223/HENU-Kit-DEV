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

发信域名必须保持“验证通过”，发信地址必须保持“正常”。SMTP 密码是按发信
地址单独设置的专用凭据，不是阿里云登录密码；更换发信地址时，必须为新地址
设置专用密码，不能沿用旧地址的密码。见[阿里云 SMTP 发信说明](https://www.alibabacloud.com/help/en/direct-mail/user-guide/send-emails-using-smtp)。

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
2. 确认 DirectMail 发信域名仍为“验证通过”、新发信地址为“正常”，且已为新地址设置专用 SMTP 密码；回退窗口内保持旧域名验证通过、旧发信地址正常且旧密码有效，并在受控环境文件备份中保留旧配置。
3. 在受控生产环境文件中，将以下四项作为一组同时更新，不得只更换用户名或复用旧地址的 SMTP 密码：

   ```env
   PLATFORM_CORE_SMTP_USERNAME
   PLATFORM_CORE_SMTP_PASSWORD
   PLATFORM_CORE_SMTP_FROM
   PLATFORM_CORE_SMTP_MESSAGE_ID_DOMAIN
   ```

4. 从当前已验证的生产发布记录确认固定 SHA、受控环境文件和同一发布的 Compose 文件路径，分别赋给 `DIRECTMAIL_RELEASE_SHA`、`DIRECTMAIL_ENV_FILE` 和 `DIRECTMAIL_COMPOSE_FILE`。仅重新创建这两个服务，使新环境变量生效；不要从环境文件中推断可能过期的 `RELEASE_SHA`：

   ```bash
   : "${DIRECTMAIL_RELEASE_SHA:?需要当前已验证的生产 SHA}"
   : "${DIRECTMAIL_ENV_FILE:?需要受控环境文件路径}"
   : "${DIRECTMAIL_COMPOSE_FILE:?需要同一发布的 Compose 文件路径}"
   RELEASE_SHA="$DIRECTMAIL_RELEASE_SHA" docker compose \
     --env-file "$DIRECTMAIL_ENV_FILE" \
     -f "$DIRECTMAIL_COMPOSE_FILE" \
     up -d --no-deps --force-recreate platform-smtp-provider platform-mail-worker
   ```
5. 从真实登录流程发送一封验证码邮件，核对 outbox、供应商投递记录和收件箱。
6. 回退时从受控备份整体恢复上述四项（包括旧发信地址对应的 SMTP 密码与旧 Message-ID 域），再按第 4 步重新创建上述两个服务。

日志不得记录 SMTP 密码、验证码、完整收件地址或邮件正文。
