# getWork 全来源与 Career 扫描可见性研究

日期：2026-08-26（Asia/Shanghai）
精确上游：[`RyaoVen/getWork@2c7800d65fb22d5094d812107c63ce94734b1c2e`](https://github.com/RyaoVen/getWork/tree/2c7800d65fb22d5094d812107c63ce94734b1c2e)

本文是只读研究：一手证据仅使用该精确上游提交、当前 HENU-Kit 源码/契约/测试和官方 MCP Go SDK 文档。它证明代码与配置行为，不声称 18 家外部招聘站在任意时刻都可用。

## 结论先行

1. **精确上游配置了 18 个来源，不是只有美团。** 其中 13 个 `dynamic` （Playwright），5 个 `platform` （直调 JSON API）。默认配置没有任何 `needs_login: true` 或 `login:`，所以这 18 源不以账号/Cookie/CAPTCHA 为正常前置。上游 README 只列 16 家“已覆盖”；`pdd` 明标“部分”，`tongcheng` 已配置但未进 README 列表。[README 60–64](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/README.md#L60-L64) [`companies.yaml:21-434`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml#L21-L434)
2. **当前 HENU 必须改代码才能“不设选择白名单、默认全开”。** 启动强制要求非空 `CAREER_GETWORK_SOURCE_ALLOWLIST`，且每源必须存在于 `approvedGetWorkApplyHosts`；该表目前只有 `meituan`/`tencent`。仅删环境变量会启动失败或落回空 seam，不会全开。[`cmd/server/main.go:101-129`](../../services/career-opportunities/cmd/server/main.go#L101-L129) [`getwork_mcp.go:58-81`](../../services/career-opportunities/getwork_mcp.go#L58-L81) [`getwork_mcp.go:312-329`](../../services/career-opportunities/getwork_mcp.go#L312-L329)
3. **当前评分可以让“其他条件都匹配”仍然必定 0 推荐。** 目标岗位完整子串命中加 55；若未命中，技术栈最多 24 + 地点 15 + 类型 6 = 45，达不到 50 分推荐阈值。匹配完全不使用 `resume_text`/`graduation_year`，也不调用已有 Career AI。[`meituan_source.go:232-286`](../../services/career-opportunities/meituan_source.go#L232-L286) [`getwork_mcp.go:275-309`](../../services/career-opportunities/getwork_mcp.go#L275-L309)
4. **“结果无法查看”是两次信息丢失。** Worker 把 `jobs` 和每来源 `sources` 状态写入 JSONB；但状态接口解码时只声明 `Jobs`，丢掉 per-source 成功/失败/发现数。Portal 再只渲染 `match_score >= 50` 的岗位，低分真实岗位不可见。[`worker.go:154-190`](../../services/career-opportunities/worker.go#L154-L190) [`searches.go:333-355`](../../services/career-opportunities/searches.go#L333-L355) [`result.ts:3-9`](../../apps/portal/src/lib/career/result.ts#L3-L9)
5. **全开不等于取消安全边界。** 可取消的是运营人工选中少数源的 allowlist；必须保留只读工具面、固定上游清单、HTTPS 官方投递域名校验、单源降级、总 deadline 和结果量上限。官方 SDK 本来就支持 `ListTools`/`CallTool` 与 Streamable HTTP，不需要再造协议层。[MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) [Streamable transport](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md#transports)

## 上游工具契约

`list_sources` 每次重读配置，返回每源 `key/name/strategy/platform/company_key/needs_login/url`。配置文件不存在时返回空来源，YAML 解析错误才是 `config_error`。[`server.py:77-88`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/server.py#L77-L88) [`config.py:129-159`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/config.py#L129-L159)

`crawl_jobs(source, since_days)` 只抓一源，返回 `status/source/fetched_at/count/jobs`。未知 key 为 `unknown_source`，捕获到 `LoginRequiredError` 才是 `login_required`，其他异常为 `crawl_failed`。[`server.py:69-125`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/server.py#L69-L125) 日期窗是 best-effort：能解析的旧日期被过滤，缺失/不可解析的日期保留。[`base.py:178-202`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/base.py#L178-L202)

`dynamic` 会启动 headless Chromium、监听 `response_match` 对应 API，再点页或重放翻页。页面正常打开但没捕获匹配 API 时，代码只记 warning 并返回空 jobs，MCP 最终仍是 `status=ok,count=0`。[`browser.py:161-222`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L161-L222) [`browser.py:241-303`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L241-L303)

`platform` 用 `httpx` 直调 JSON API，最多 50 页；空 items 正常返回，HTTP/解析/解密异常上抛。Moka 源依赖固定 AES-CBC 解密。[`platform.py:28-110`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/platform.py#L28-L110)

## 18 个来源的实际配置

| key | 策略 | 已配数据 | 典型 0 结果条件 | 官方 host |
| --- | --- | --- | --- | --- |
| `tencent` | dynamic | 标题/城市/类型/URL，详情 API 补职责/要求 | API 未触发/字段变更 | `join.qq.com` |
| `jd` | dynamic | 标题/城市/类型/日期，详情 API 补职责/要求 | API 未捕获/日期超窗 | `campus.jd.com` |
| `bytedance` | dynamic | 列表含职责/要求/详情 URL | API 未捕获/项目 ID 过期 | `jobs.bytedance.com` |
| `alibaba` | dynamic | 标题/城市/日期，访详情页抽职责/要求 | API 未捕获/批次 ID 过期 | `campus-talent.alibaba.com` |
| `dewu` | dynamic | 与字节同平台字段，含详情 URL | API 未捕获/项目 ID 过期 | `campus.dewu.com` |
| `xfusion` | dynamic | 列表只标题，详情 API 补职责/要求 | API 未捕获 | `career.xfusion.com` |
| `pdd` | dynamic | **上游明标“部分”**：实习项目，岗位细分待补 | API 未捕获/项目非岗位 | `careers.pddglobalhr.com` |
| `beike` | dynamic | 只标题/类型，URL 回列表页 | API 未捕获 | `campus.ke.com` |
| `tencentmusic` | dynamic | 标题/职责/类型/城市，URL 回列表页 | API 未捕获 | `join.tencentmusic.com` |
| `xiaohongshu` | dynamic | 含职责/要求；上游 source URL 仍是 HTTP | API 未捕获/HTTPS 校验丢回退 URL | `job.xiaohongshu.com` |
| `kuaishou` | dynamic | 只标题，URL 回列表页 | API 未捕获 | `zhaopin.kuaishou.cn` |
| `netease` | dynamic | 标题/类型/职责/要求/部门 | API 未捕获 | `hr.163.com` |
| `meituan` | platform | 含职责/要求/详情 URL | API 空列表/日期超窗 | `zhaopin.meituan.com` |
| `didi` | platform/Moka | 标题/城市/职能/日期，URL 为列表页 | Moka 参数/解密变更 | `app.mokahr.com` |
| `vipshop` | platform/Moka | 与滴滴同类 Moka 字段/解密 | Moka 参数/解密变更 | `app-tc.mokahr.com` |
| `tongcheng` | dynamic | 只标题/城市，未进 README 16 家列表 | API 未捕获 | `mhr.ly.com` |
| `ctrip` | platform | 含职责/要求/日期/岗位族 | API 空列表/日期超窗 | `job.ctrip.com` |
| `baidu` | platform | 实习 API，含职责/要求/日期/类型 | API 空列表/日期超窗 | `talent.baidu.com` |

配置证据：[`tencent`–`alibaba`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml#L22-L118)；[`dewu`–`tencentmusic`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml#L120-L204)；[`xiaohongshu`–`netease`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml#L206-L246)；[`meituan`–`vipshop`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml#L249-L355)；[`tongcheng`–`baidu`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/config/companies.yaml#L357-L434)。

### 登录/Cookie/CAPTCHA 边界

`Source.needs_login` 默认 `false`、`login` 默认 `None`，18 源均未覆盖。[`config.py:46-73`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/config.py#L46-L73) BrowserCrawler 只在 `needs_login=true` 无会话，或配了 `login.wall_*` 且页面命中时，才抛 `LoginRequiredError`。因此招聘站临时展示登录/验证码/反爬页时，当前更可能呈现为“未捕获 API → 成功但 0 岗位”，而非 `login_required`。[`browser.py:161-166`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L161-L166) [`browser.py:453-464`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L453-L464)

上游 `login`/CAPTCHA 分支是为未来/自定义 dynamic 源准备：headless 登录失败可提示 `headed=true`，成功后明文 JSON 保存 Cookie。HENU 已移除该 tool，不应把 HENU 账号或招聘站凭据引入当前公开源路径。[`server.py:128-160`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/server.py#L128-L160) [`browser.py:505-555`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/crawlers/browser.py#L505-L555) [`sessions.py:18-48`](https://github.com/RyaoVen/getWork/blob/2c7800d65fb22d5094d812107c63ce94734b1c2e/getwork/sessions.py#L18-L48)

## HENU 从 MCP 到持久化的契约与丢失点

```text
Portal 画像快照 → queued → worker running/crawling
  → 顺序 crawl_jobs → Career 规范化/评分
  → payload={jobs,sources} 写 JSONB
  → GET status 解码成 {counts,summary,jobs}
  → Portal 仅渲染 score>=50
```

创建时 Career 在事务中冻结 `profile_snapshot`，并执行幂等、每小时限额和单 active 任务门禁。[`searches.go:67-162`](../../services/career-opportunities/searches.go#L67-L162) Worker 把任务设为 `running/stage=crawling`，成功后在同一事务写 `completed` 和 `career_search_results`；payload 是完整 JSONB。[`worker.go:121-190`](../../services/career-opportunities/worker.go#L121-L190) [`000001_career_searches.up.sql:10-35`](../../services/career-opportunities/db/migrations/000001_career_searches.up.sql#L10-L35)

MCP WorkFunc 对每源顺序调用。非 `ok` 记 `failed`并继续；`ok` 即使 0 岗位也记 `success,found=0`；只有全部源非 `ok` 才让整任务失败。[`getwork_mcp.go:105-163`](../../services/career-opportunities/getwork_mcp.go#L105-L163) 测试证明一源失败、一源成功空结果时，内部 payload 会保留两源状态。[`getwork_mcp_test.go:122-155`](../../services/career-opportunities/getwork_mcp_test.go#L122-L155)

读路径丢失了它：OpenAPI result 没有 `sources`，`decodeSearchResult` 也只取 `Jobs`。[`career.yaml:475-507`](../../packages/api-contracts/openapi/career.yaml#L475-L507) [`searches.go:333-355`](../../services/career-opportunities/searches.go#L333-L355)
