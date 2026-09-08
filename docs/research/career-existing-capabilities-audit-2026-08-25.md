# Career 已有邮件、模型与 getWork `login` 能力审计

审计日期：2026-08-25（Asia/Shanghai）
范围：HENU-Kit 当前工作树的一手源码、生产编排契约，以及 `RyaoVen/getWork` 精确提交 [`2c7800d65fb22d5094d812107c63ce94734b1c2e`](https://github.com/RyaoVen/getWork/tree/2c7800d65fb22d5094d812107c63ce94734b1c2e)。
限制：初始审计为只读研究，不发送新邮件、不调用模型。2026-08-26 的后续实施按用户授权修复了生产 migration 漂移；文中已明确标注原始观察与修复后状态。

## 结论先行

1. **HENU-Kit 已经有完整的通用发信链，也已经有 Career 专用摘要入队契约。** Career 不应再调用 getWork 的 `send_email`，更不需要再建一套 SMTP。正确复用点是现有的 `DigestSender -> Platform Core 加密 Outbox -> mail worker -> SMTP provider`。
2. **Career 自己已经有真实的大模型接入。** 简历提取和“简历酥化”共用一组 operator-owned OpenAI-compatible `base URL / API key / model`。可以复用这组 provider 配置和调用基础设施做岗位匹配，但当前代码没有 `MatchFunc`/岗位匹配模型契约；当前匹配仍是来源适配器内的确定性关键词计分。因此“可复用”是增加一个很薄、受约束的匹配调用，不是把提取器或酥化器硬改名后直接调用。
3. **getWork 的 `login` 是招聘网站来源登录，不是 HENU 账号登录。** 它仅面向 `dynamic` 来源，用来源账号密码驱动 Playwright，随后把 cookie 按来源写入本地 JSON。HENU 登录由 Platform Core 的邮箱验证码/密码和 Core Session 负责，两者没有账号、会话或授权关系。
4. **生产运行态要与代码能力分开说。** 2026-08-25 的只读核验表明通用验证码邮件链真实发过信、Career 模型也真实完成过一次提取；当时 Career 摘要因生产数据库缺少 `mail_outbox.recipient_user_id` 而入队失败。2026-08-26 已应用仓库内 `000019`/`000020` 迁移并观测到 2 条 Career digest 被 Platform Core 接受、SMTP provider 成功处理。

## 一、已有发邮件能力

### 1.1 Career 已经有专用调用入口

Career 的调用面不是任意收件人/主题/HTML，而是受控的 `DigestRequest`：只包含 `user_id`、`search_id`、统计摘要、结果页和最多若干岗位；收件人不在请求中，由 Platform Core 根据用户 ID 查已验证邮箱。[`services/career-opportunities/digest.go:22-56`](../../services/career-opportunities/digest.go#L22-L56)

生产实现 `HTTPDigestSender` 固定 POST `/api/v1/career-digest-mails`，设置 `Idempotency-Key: career_search_completed:{search_id}`，并用服务凭据、时间戳、nonce、正文哈希做 HMAC 签名。[`services/career-opportunities/digest.go:63-88`](../../services/career-opportunities/digest.go#L63-L88) [`services/career-opportunities/digest.go:104-146`](../../services/career-opportunities/digest.go#L104-L146)

Career 启动时通过以下四个配置项装配发送器；URL 为空则明确关闭摘要邮件，URL 存在但凭据不完整则启动失败，而不是静默降级：

- `PLATFORM_CORE_CAREER_DIGEST_URL`
- `PLATFORM_CORE_CAREER_DIGEST_CLIENT_ID`
- `PLATFORM_CORE_CAREER_DIGEST_KEY_ID`
- `PLATFORM_CORE_CAREER_DIGEST_SECRET`

证据：[`services/career-opportunities/cmd/server/main.go:321-338`](../../services/career-opportunities/cmd/server/main.go#L321-L338)、[`services/career-opportunities/.env.example:20-24`](../../services/career-opportunities/.env.example#L20-L24)。生产编排要求这些值存在，并把 Career 的目标固定为 Platform Core：[`docker-compose.henukit.prebuilt.yml:172-196`](../../docker-compose.henukit.prebuilt.yml#L172-L196)。

### 1.2 Career 完成搜索后如何调用

搜索结果先在事务内落库，再创建 `career_digest_deliveries(status='sending')`；事务提交后才尝试摘要入队。这样邮件故障不会回滚已完成的搜索结果。[`services/career-opportunities/worker.go:154-196`](../../services/career-opportunities/worker.go#L154-L196)

实际发送前，Career 会：

1. 在未配置 sender 时标记 `skipped`；
2. 读取搜索所有者与完成时间；
3. 检查 Career Profile 的 `email_notification_enabled`；
4. 用 `email_sent_at` 防止已接受入队的搜索再次发送；
5. 只取前 5 个达到阈值的岗位组成摘要；
6. Platform Core 返回成功后，原子写入 `email_sent_at` 和 delivery `sent`。

证据：[`services/career-opportunities/worker.go:323-379`](../../services/career-opportunities/worker.go#L323-L379)、[`services/career-opportunities/worker.go:382-420`](../../services/career-opportunities/worker.go#L382-L420)。这里的 `sent` 是“Platform Core 已接受入队”，不是“用户邮箱已投递”；API 契约也明确了这个语义。[`packages/api-contracts/openapi/career.yaml:460-465`](../../packages/api-contracts/openapi/career.yaml#L460-L465)

Career 自身还有一层持久重试账本。失败后写为 `retry`；worker 用 `FOR UPDATE SKIP LOCKED` 认领失败或超时的 `sending` 记录，一分钟后重试。每次重试仍使用同一个 search-scoped 幂等键。[`services/career-opportunities/db/migrations/000004_career_digest_deliveries.up.sql:1-12`](../../services/career-opportunities/db/migrations/000004_career_digest_deliveries.up.sql#L1-L12) [`services/career-opportunities/worker.go:199-249`](../../services/career-opportunities/worker.go#L199-L249)

### 1.3 Platform Core 入队与收件人解析

Platform Core 已注册内部入口 `POST /api/v1/career-digest-mails`。[`services/platform-core/internal/httpapi/handler.go:104-122`](../../services/platform-core/internal/httpapi/handler.go#L104-L122) 入口验证 Basic、服务 ID、key ID、五分钟时间窗、24 字节 nonce、HMAC，并用 Redis `SETNX` 防 nonce 重放；浏览器不能直达，也不能指定收件人。[`services/platform-core/internal/httpapi/handler.go:716-770`](../../services/platform-core/internal/httpapi/handler.go#L716-L770)

通过验证后，Platform Core 根据 `user_id` 查询已验证 Email Identity，解密邮箱，把收件人与摘要分别加密，然后创建 `kind='career_digest'` 的 Outbox 行。去重键固定为 `career_search_completed:{search_id}`；唯一键冲突直接作为幂等 replay 成功处理。[`services/platform-core/internal/careerdigestmail/service.go:73-119`](../../services/platform-core/internal/careerdigestmail/service.go#L73-L119) 数据查询和写入形状见 [`services/platform-core/db/queries/verification_mail.sql:30-42`](../../services/platform-core/db/queries/verification_mail.sql#L30-L42)。

### 1.4 Outbox、worker、provider 与 SMTP

`mail_outbox` 是持久队列：状态包括 `pending / processing / retry_due / accepted / delivered / failed`，默认最多 5 次尝试；另有 delivery receipt、dead letter 和 append-only audit event。[`services/platform-core/db/migrations/000003_verification_mail.up.sql:26-66`](../../services/platform-core/db/migrations/000003_verification_mail.up.sql#L26-L66) [`services/platform-core/db/migrations/000003_verification_mail.up.sql:68-117`](../../services/platform-core/db/migrations/000003_verification_mail.up.sql#L68-L117)

独立 `platform-mail-worker` 从 Outbox 认领任务，解密 `career_digest`，映射到 `henukit_career_digest` 模板，再调用 provider。临时失败按 1 分钟起、最多 1 小时的指数退避重试；永久错误或达到最大次数进入失败/dead-letter 状态。[`services/platform-core/internal/mailworker/worker.go:89-188`](../../services/platform-core/internal/mailworker/worker.go#L89-L188) [`services/platform-core/internal/mailworker/worker.go:221-252`](../../services/platform-core/internal/mailworker/worker.go#L221-L252) SQL 使用过期 lease 回收、`FOR UPDATE SKIP LOCKED` 与尝试计数确保并发安全：[`services/platform-core/db/queries/verification_mail.sql:173-225`](../../services/platform-core/db/queries/verification_mail.sql#L173-L225)。

worker 的 provider client 固定发送模板化 JSON，并把同一个幂等键继续传到 HTTP header 和 body；4xx 中除 timeout/conflict/rate limit 外归为永久失败。[`services/platform-core/internal/mailworker/http_sender.go:71-132`](../../services/platform-core/internal/mailworker/http_sender.go#L71-L132)

现有本地 provider 再通过 SMTP/SMTPS 发信。它要求 TLS（465 隐式 TLS，其他端口必须 STARTTLS）、SMTP 用户名/密码/from，并把 DATA 成功关闭视为 SMTP 接受。[`services/platform-core/internal/smtpprovider/smtp.go:22-96`](../../services/platform-core/internal/smtpprovider/smtp.go#L22-L96) 摘要邮件已有独立主题、纯文本和 HTML 模板，而不是依赖 getWork 渲染 HTML。[`services/platform-core/internal/smtpprovider/smtp.go:99-109`](../../services/platform-core/internal/smtpprovider/smtp.go#L99-L109) [`services/platform-core/internal/smtpprovider/smtp.go:190-220`](../../services/platform-core/internal/smtpprovider/smtp.go#L190-L220)

SMTP provider 还有第二层磁盘幂等账本：以幂等键哈希生成稳定 Message-ID；已有 `.accepted.json` 时直接 replay；存在不确定的 `.pending` 时 fail closed，避免 worker 重试造成重复投递。[`services/platform-core/internal/smtpprovider/provider.go:170-244`](../../services/platform-core/internal/smtpprovider/provider.go#L170-L244)

### 1.5 代码能力与 2026-08-25 生产状态

生产编排确实包含 `platform-mail-worker` 和 `platform-smtp-provider`，并要求 provider token、SMTP 地址、用户名、密码与发件人配置存在。[`docker-compose.henukit.prebuilt.yml:39-58`](../../docker-compose.henukit.prebuilt.yml#L39-L58)

本次协调线程提供的 2026-08-25 生产只读核验（未记录任何密钥值）显示：

- `platform-mail-worker`、`platform-smtp-provider`、`career-opportunities` 容器均在运行；SMTP 用户名/密码和 Career digest 服务配置均非空。
- `platform.mail_outbox` 中 `verification_code accepted=77`；SMTP provider 在 `2026-08-24 23:46:04Z` 有 `succeeded` 日志。这证明通用发信链曾真实走到 provider/SMTP 接受，不只是代码存在或健康检查为 200。
- 但 `career_digest_deliveries` 当前有 `retry=2`，Platform Core 日志反复返回 503。数据库错误是 `mail_outbox.recipient_user_id` 不存在（PostgreSQL `SQLSTATE 42703`）。仓库迁移 `000019` 明确应该添加此列：[`services/platform-core/db/migrations/000019_career_digest_mail.up.sql:1-18`](../../services/platform-core/db/migrations/000019_career_digest_mail.up.sql#L1-L18)。

因此更新后的准确结论是：**通用邮件基础设施和 Career 摘要接入都已存在；2026-08-26 生产 migration 漂移已修复，当时积压的 2 条摘要已经过完整链路被 SMTP provider 接受。** GitHub Actions 部署监视器也将这两个幂等迁移设为每次激活的必备项，避免后续版本重现该漂移。

## 二、已有大模型能力

### 2.1 Career 已有两条真实模型调用

**简历提取**已经定义 `ExtractFunc`，把 PDF/DOCX/TXT 转成 Career Profile 草稿字段；用户确认前不会保存为正式 Profile。[`services/career-opportunities/extract.go:29-45`](../../services/career-opportunities/extract.go#L29-L45) 生产实现调用 operator-configured OpenAI-compatible `/chat/completions`，参数是 `BaseURL / APIKey / Model`，默认 60 秒超时，拒绝 credential-bearing redirect，并把模型输出按严格 JSON 契约解析、限长与校验。[`services/career-opportunities/extract.go:80-126`](../../services/career-opportunities/extract.go#L80-L126) [`services/career-opportunities/extract.go:126-175`](../../services/career-opportunities/extract.go#L126-L175) [`services/career-opportunities/extract.go:233-289`](../../services/career-opportunities/extract.go#L233-L289)

**简历酥化**已经定义 `SuifyFunc`，把当前 Resume Text 和内嵌 Skill 发往同一个 OpenAI-compatible `/chat/completions` endpoint，返回临时草稿，不自动保存 Profile。[`services/career-opportunities/suify.go:18-47`](../../services/career-opportunities/suify.go#L18-L47) [`services/career-opportunities/suify.go:47-116`](../../services/career-opportunities/suify.go#L47-L116) 它的 Prompt 明确禁止新增事实和执行简历里的提示注入指令。[`services/career-opportunities/skills/resume-suify.md:1-13`](../../services/career-opportunities/skills/resume-suify.md#L1-L13)

Suification 的 HTTP 入口已经具备 actor scope、请求哈希、幂等 replay、同键并发锁和每小时限流；模型调用不是一个裸 HTTP helper。[`services/career-opportunities/suifications.go:43-137`](../../services/career-opportunities/suifications.go#L43-L137) 简历提取则是异步 job，由共享 Career worker 在 60 秒任务 deadline 内执行，结束时清除上传文件字节。[`services/career-opportunities/worker.go:251-311`](../../services/career-opportunities/worker.go#L251-L311)

### 2.2 Provider 配置是共享的

`buildExtractor` 和 `buildSuifier` 都调用同一个 `loadCareerAIProvider()`，读取：

- `CAREER_AI_MODE`
- `CAREER_AI_BASE_URL`
- `CAREER_AI_API_KEY`
- `CAREER_AI_MODEL`
- `CAREER_REQUIRE_AI`
- 两个精确的 HTTP 例外开关

证据：[`services/career-opportunities/cmd/server/main.go:136-205`](../../services/career-opportunities/cmd/server/main.go#L136-L205) [`services/career-opportunities/cmd/server/main.go:207-287`](../../services/career-opportunities/cmd/server/main.go#L207-L287)。`CAREER_REQUIRE_AI=1` 时，空配置、mock、placeholder 或不允许的明文 HTTP 都会让启动失败；服务启动前还会运行不含用户数据的 PDF 提取探针。[`services/career-opportunities/cmd/server/main.go:55-61`](../../services/career-opportunities/cmd/server/main.go#L55-L61) [`services/career-opportunities/cmd/server/main.go:290-307`](../../services/career-opportunities/cmd/server/main.go#L290-L307)

生产镜像编排要求真实 `base URL / API key / model`，并固定 `CAREER_REQUIRE_AI=1`、`CAREER_AI_MODE=""`。[`docker-compose.henukit.prebuilt.yml:192-200`](../../docker-compose.henukit.prebuilt.yml#L192-L200)

2026-08-25 的生产只读核验进一步显示：Career AI base/key/model 非空，`CAREER_REQUIRE_AI=1`、mode 为空，`career_resume_extractions` 有 `completed=1`，最新完成时间为 `2026-08-23 11:25:52Z`。这是一条真实 provider 成功结果证据；它不等于所有格式/所有请求都已验收。

### 2.3 能否复用做岗位匹配

**能，但现成的是 provider 与治理能力，不是已经完成的 AI 岗位匹配函数。**

当前 HENU 匹配发生在来源适配器内。以美团为例，它把目标岗位、技术栈、地点和岗位类型做确定性字符串命中，生成 0–100 分与可展示理由。[`services/career-opportunities/meituan_source.go:232-286`](../../services/career-opportunities/meituan_source.go#L232-L286) `NewGetWorkWork` 只聚合来源结果，并把 `MatchScore >= 50` 计为推荐；它没有调用 LLM。[`services/career-opportunities/getwork.go:54-107`](../../services/career-opportunities/getwork.go#L54-L107)

现有两个函数类型又是用途专属的：`ExtractFunc(fileName, bytes) -> ExtractedProfile` 与 `SuifyFunc(original) -> string`，并没有 `MatchFunc(profile, jobs) -> scored jobs`。[`services/career-opportunities/extract.go:41-45`](../../services/career-opportunities/extract.go#L41-L45) [`services/career-opportunities/suify.go:21-23`](../../services/career-opportunities/suify.go#L21-L23)

如果决定用模型增强岗位匹配，最小复用方式是：

1. 继续让原 MCP/来源只返回规范化岗位事实；
2. 在 Career worker 内新增一个有界的 `MatchFunc`，复用同一 operator-owned provider 配置、HTTP 传输限制、错误码清洗、并发/限流和结果持久化规则；
3. 输入固定为 frozen `profile_snapshot` 与有上限的规范化岗位字段，输出严格为 `job identity + score + reasons`；
4. 保留确定性过滤/验证，模型失败时明确降级，而不是把模型判断伪装成来源事实。

上游 getWork 本身也不提供“LLM 匹配 MCP tool”。它暴露的是 `list_sources / crawl_jobs / login / logout / add_source / render_briefing / send_email`；其 `find-jobs` Skill 要求宿主 Agent 在抓取后用机械打分加“你的判断”修正。[`getwork/server.py:77-167`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/server.py#L77-L167) [`find-jobs/SKILL.md:44-54`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/.claude/skills/find-jobs/SKILL.md#L44-L54) 其 `match.py` 也是本地确定性关键词计分，并非模型 API。[`getwork/match.py:27-78`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/match.py#L27-L78)

所以，直接使用 getWork MCP 并不会自动获得其 README/Skill 中的“模型判断”；若 HENU 需要这部分，正好可以复用现有 Career LLM，而无需复制 getWork 的邮件和 Agent 流程。

## 三、getWork 所称 `login` 到底是什么

### 3.1 精确源码语义

getWork MCP 的 `crawl_jobs` 在来源爬虫抛出 `LoginRequiredError` 时返回 `status="login_required"`，提示调用 `login` 后重试。[`getwork/server.py:91-125`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/server.py#L91-L125)

其 `login(source, username, password, headed=false)`：

- 仅接受配置为 `strategy="dynamic"` 的招聘来源；
- 参数是**该招聘来源的账号和密码**；
- `headed=true` 用于弹真实浏览器，让用户处理验证码/滑块；
- 调用 `login_source` 后只返回来源 key 和 cookie 过期时间。

证据：[`getwork/server.py:128-160`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/server.py#L128-L160)。

底层 `login_source` 用 Playwright 打开来源配置的 login URL，按配置的 username/password/submit selector 填表并点击，等待 URL/selector 成功条件；失败可能被判为错误凭据或验证码要求。成功后读取 browser context cookies 并按来源保存。[`getwork/crawlers/browser.py:505-555`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L505-L555)

密码参数本身没有由这段代码写盘，但 cookie 明文 JSON 会写到 `data/sessions/{source}.json`；后续抓取把 cookies 加入 Playwright context。[`getwork/sessions.py:1-48`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/sessions.py#L1-L48) [`getwork/crawlers/browser.py:161-211`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L161-L211)

还要注意：在该精确提交的默认 `config/companies.yaml` 中，没有任何来源显式设置 `needs_login: true` 或 `login:`；`Source.needs_login` 的默认值是 `false`。[`getwork/config.py:46-73`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/config.py#L46-L73) [`config/companies.yaml`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml) 因此 `login` 是为将来/自定义的需登录 dynamic 来源准备的能力，不是默认 18 个来源的必经步骤。

### 3.2 它与 HENU 账号登录无关

HENU 账号登录由 Platform Core 注册 `/login/code`、`/login/verify`、`/login/password`，并最终建立 HENU Core Session。[`services/platform-core/internal/httpapi/handler.go:80-100`](../../services/platform-core/internal/httpapi/handler.go#L80-L100) 邮箱验证码路径调用 HENU verification service 并写 Core Session cookie；密码路径以 HENU Email Identity + HENU Password Credential 校验后写同一类 session cookie。[`services/platform-core/internal/httpapi/handler.go:1277-1358`](../../services/platform-core/internal/httpapi/handler.go#L1277-L1358) [`services/platform-core/internal/httpapi/handler.go:1361-1401`](../../services/platform-core/internal/httpapi/handler.go#L1361-L1401)

所以两种 login 的边界是：

| 名称 | 身份对象 | 凭据 | 会话存储 | 用途 |
| --- | --- | --- | --- | --- |
| HENU Account login | HENU Platform User | HENU 邮箱验证码或 HENU 密码 | Platform Core Core Session | 登录 HENU-Kit |
| getWork source login | 某个招聘网站来源账号 | 招聘站账号/密码，可能加人工验证码 | getWork 本地按来源 cookie JSON | 让 Playwright 抓取受登录墙保护的岗位 |

它们不能互换，也不应映射。HENU 用户登录成功不意味着已登录任何招聘网站；招聘网站 cookie 也不能作为 HENU 身份或授权证据。

### 3.3 对当前接入范围的影响

若改为“原 getWork MCP + 官方 Go MCP SDK + 薄映射”，当前只需要 allowlist 后调用 `list_sources` 和 `crawl_jobs`。`login` 不应进入 HENU 浏览器 API，也不应让 Career 把 HENU 密码、邮箱密码或招聘站密码传给模型/MCP。

当某来源真的返回 `login_required` 时，v1 可以把该来源记为不可用/跳过，并保留其他公开来源结果。只有产品明确决定支持招聘站账号后，才需要另开一条凭据托管、交互式验证码、cookie 加密/隔离/撤销和合规审查链路；这不是 HENU Account login 的复用问题。

## 四、对“为什么搞复杂”的直接回答

现有代码已经证明三件事可以拆开：

- **抓岗位**：原 getWork MCP 或固定公开来源负责返回岗位事实；
- **理解与匹配**：HENU Career 已有模型 provider，可在 owner worker 内增加受约束匹配；
- **发邮件**：HENU Platform Core 已有 Outbox/worker/provider/SMTP，Career 只需投递受控 digest。

因此没有必要复制 getWork 的 `send_email`、SMTP 配置、render briefing 或宿主 Agent 流程，也没有必要把 getWork 的招聘站 `login` 和 HENU Account login 混在一起。真正必要的“适配”只剩很薄的一层：MCP tool 调用、岗位字段映射、allowlist/超时/错误映射，以及把规范化结果交给已有匹配和邮件链路。

当时的运行故障是生产数据库缺失 `000019` 所声明的列；这是 migration 漂移，不是缺少邮件 SDK/能力。该漂移已于 2026-08-26 修复并通过真实摘要入队与 SMTP provider 成功记录核验。
