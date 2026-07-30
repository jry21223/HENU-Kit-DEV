# henukit.cn production cutover

This runbook records the verified production ingress for issue #179. It is
separate from the SMTP identity release so each high-risk change has its own
review and rollback window.

## Verified baseline

Recorded at 2026-07-30 19:28 Asia/Shanghai:

| Evidence | Verified value |
|---|---|
| ECS instance | `i-2ze0nqpaly4kg7pe29jb` in `cn-beijing` |
| Public origin | `8.146.200.82` |
| SSH | `root@8.146.200.82:22222` |
| Host edge | Ubuntu Nginx 1.24 on ports 80/443 |
| Compose edge | `127.0.0.1:8088` |
| Legacy certificate | `superhuazai.me`, valid through 2026-10-22 |
| New certificate | `henukit.cn` and `www.henukit.cn`, valid through 2026-10-28 |
| Legacy vhost | `/etc/nginx/sites-enabled/superhuazai.me` |
| New vhost | `/etc/nginx/sites-enabled/henukit.cn` |
| Nginx rollback snapshot | `/etc/nginx/rollback/20260730T113123Z` |
| Authoritative DNS | Alibaba Cloud DNS (`dns23.hichina.com`, `dns24.hichina.com`) |

The repository's former `115.191.22.22` comment was historical and must not be
used as a deployment target.

The Alibaba Cloud DNS snapshot recorded for the cutover contains:

| Record | Type | Value / purpose | Record ID |
|---|---|---|---|
| `@` | A | `8.146.200.82` | `2082791229322081280` |
| `www` | CNAME | `henukit.cn` | `2082791235785549824` |
| `notify` | MX | `mx01.dm.aliyun.com`, priority 1 | `2081981597901085696` |
| `notify` | TXT | DirectMail SPF | `2081980646792375296` |
| `aliyun-cn-hangzhou._domainkey.notify` | TXT | DirectMail DKIM | `2081979763031553024` |
| `_dmarc.notify` | TXT | `p=none` aggregate reporting | `2081981142961665024` |

At the time of the snapshot, DirectMail reported `notify.henukit.cn` as
verified; Alibaba Cloud DNS did not expose a separate ownership record. Before
changing authoritative nameservers, export the current zone and compare every
record with the DirectMail console. If the console exposes an ownership record,
copy it exactly and keep it DNS-only.

## DNS and TLS order

1. Keep `superhuazai.me` and its vhost active.
2. Point `@` to `8.146.200.82` and `www` to `henukit.cn` at the current
   authoritative provider.
3. Serve the ACME challenge over HTTP, then issue one certificate covering both
   hostnames.
4. Validate the origin directly with explicit SNI before enabling a proxy.
5. Add the Cloudflare zone and copy every web and DirectMail record. MX, SPF,
   DKIM, DMARC and ownership records remain DNS-only.
6. Set SSL/TLS mode to Full (strict), enable the web proxy only for apex/www,
   and then change nameservers at the registrar.
7. Verify public DNS from multiple resolvers, apex HTTPS, `www` 308, OAuth,
   Console, API failures, and the DirectMail domain status.

Do not create account, study, quiz, API, Console or deploy subdomains until an
independent service boundary is approved.

## Application origins

The production environment must explicitly use:

```env
PUBLIC_ORIGIN=https://henukit.cn
PORTAL_ORIGIN=https://henukit.cn
CONSOLE_ORIGIN=https://henukit.cn/console
QUIZ_ORIGIN=https://henukit.cn/quiz
PLATFORM_ACCOUNT_ORIGIN=https://henukit.cn/account-auth
PLATFORM_CORE_PUBLIC_URL=https://henukit.cn/account-auth
PORTAL_REDIRECT_URI=https://henukit.cn/api/v1/auth/callback
CONSOLE_REDIRECT_URI=https://henukit.cn/console-api/v1/auth/callback
STUDY_CORS_ALLOWED_ORIGINS=https://henukit.cn
QUIZCRAFT_CORS_ORIGINS=https://henukit.cn
```

Recreate only the services that consume changed environment values. A plain
container restart does not update environment variables.

## OAuth callback registration

`PORTAL_REDIRECT_URI` and `CONSOLE_REDIRECT_URI` are not sufficient by
themselves. Platform Core performs an exact match against
`oauth_clients.redirect_uris`. Before recreating either gateway:

1. Back up the `portal-gateway` and `console-gateway` rows and confirm they
   contain only the expected legacy callbacks.
2. In one transaction, add the two `henukit.cn` callbacks while retaining the
   legacy callbacks for the rollback window.
3. Query the rows after commit and run both OAuth callback Smokes.
4. After the observation window, remove callbacks for the retired domain in a
   separate, audited change.

Run the transaction through the production PostgreSQL container without
printing database credentials:

```sql
BEGIN;
SELECT id, redirect_uris
FROM oauth_clients
WHERE id IN ('portal-gateway', 'console-gateway')
FOR UPDATE;

UPDATE oauth_clients
SET redirect_uris = ARRAY[
  'https://superhuazai.me/api/v1/auth/callback',
  'https://henukit.cn/api/v1/auth/callback'
]
WHERE id = 'portal-gateway';

UPDATE oauth_clients
SET redirect_uris = ARRAY[
  'https://superhuazai.me/console-api/v1/auth/callback',
  'https://henukit.cn/console-api/v1/auth/callback'
]
WHERE id = 'console-gateway';
COMMIT;
```

If either OAuth Smoke fails, restore the backed-up arrays in a transaction
before restoring the gateway environment.

## Rollback

1. Disable the Cloudflare proxy or restore the previous nameservers.
2. Keep or restore the legacy `superhuazai.me` DNS and vhost.
3. Restore the backed-up `oauth_clients.redirect_uris` arrays transactionally.
4. Restore the environment backup made immediately before cutover.
5. Recreate the affected gateways with their prior fixed image tags.
6. Validate legacy HTTPS, OAuth callback, Console login and API readiness.

Do not delete the new certificate, old vhost, old DNS records or mail identity
during the observation window.
