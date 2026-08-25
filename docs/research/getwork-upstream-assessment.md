# RyaoVen/getWork upstream assessment

Date checked: 2026-08-13 (Asia/Shanghai)

## Identity and maturity

- The target is [`RyaoVen/getWork`](https://github.com/RyaoVen/getWork), a public Python repository created on 2026-08-11. Its package reports version `0.1.0` and requires Python 3.11+ ([`pyproject.toml`](https://github.com/RyaoVen/getWork/blob/master/pyproject.toml)).
- It is a personal job-search MCP server, not a recruitment-management product: the documented flow crawls campus/internship postings, lets an agent match them against a resume profile, renders a briefing, and sends it by SMTP ([README](https://github.com/RyaoVen/getWork/blob/master/README.md)).
- The default configuration currently contains 18 sources using dynamic-browser and direct-platform strategies ([`config/companies.yaml`](https://github.com/RyaoVen/getWork/blob/master/config/companies.yaml)).
- The complete history is a one-day implementation burst on 2026-08-11, with no tagged release and no published GitHub release ([commit history](https://github.com/RyaoVen/getWork/commits/master/), [releases](https://github.com/RyaoVen/getWork/releases)). There is no repository CI workflow or conventional unit-test suite; validation is supplied as executable smoke/E2E scripts ([`scripts/smoke_test.py`](https://github.com/RyaoVen/getWork/blob/master/scripts/smoke_test.py), [`scripts/e2e_local.py`](https://github.com/RyaoVen/getWork/blob/master/scripts/e2e_local.py), [`scripts/e2e_real.py`](https://github.com/RyaoVen/getWork/blob/master/scripts/e2e_real.py)).
- The README says MIT, but the repository currently has no `LICENSE` file and GitHub reports no detected license. On 2026-08-26 Jerry reported direct authorization from the owner (“你直接接去用”) for this HENU-Kit integration. That project-specific permission allows the pinned internal use; it is not a general public-license claim.

## Runtime and behavior

The documented process launches over stdio with `uv run python -m getwork`. Its pinned MCP SDK also exposes the same server as Streamable HTTP, but the upstream repository does not supply deployment authentication, a tenant model, database, or scheduler ([`.mcp.json`](https://github.com/RyaoVen/getWork/blob/master/.mcp.json), [`getwork/__main__.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/__main__.py)).

It exposes seven tools in [`getwork/server.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/server.py):

1. `list_sources`
2. `crawl_jobs`
3. `login`
4. `logout`
5. `add_source`
6. `render_briefing`
7. `send_email`

All crawler outputs normalize into `JobRecord`: title, company, source, location, department, type, dates, application URL, description, and requirements ([`getwork/models.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/models.py)). Sources are configuration-driven and use three adapters: direct JSON APIs, Playwright-rendered/captured APIs, or static HTML/CSS selectors ([crawler package](https://github.com/RyaoVen/getWork/tree/master/getwork/crawlers)).

Matching is deterministic keyword scoring in [`getwork/match.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/match.py), but it is not exposed as an MCP tool. The bundled `find-jobs` skill asks the agent to build/update the profile, call crawl tools, apply mechanical scoring plus model judgment, create Markdown, render it, and send it ([skill](https://github.com/RyaoVen/getWork/blob/master/.claude/skills/find-jobs/SKILL.md)).

An isolated check on 2026-08-13 cloned current `master`, completed `uv sync --frozen`, started the MCP server, and successfully listed all seven tools and all 18 configured sources. This proves package/startup/tool discovery only; it does not prove that every external recruitment site still crawls successfully.

## Production risks relevant to HENUKit

The upstream is suitable as a prototype or source of adapters, but not as a directly exposed HENUKit service.

- **No multi-user boundary:** profile, outputs, logs, cookies, and SMTP configuration are process-local files. There is no actor-derived ownership or cross-user isolation.
- **Credentials cross the agent tool boundary:** `login(source, username, password)` accepts raw credentials from the agent. Passwords are not deliberately persisted, but resulting cookies are stored as plaintext JSON files ([`getwork/server.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/server.py), [`getwork/sessions.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/sessions.py)). This conflicts with HENUKit's established rule that LangBot must not receive reusable credentials or provider cookies.
- **Arbitrary outbound targets:** `add_source` accepts arbitrary HTTP(S) URLs, strategy data, selectors, and API configuration. The crawlers then make HTTP or Playwright requests to these targets without a production allowlist or private-network guard ([`getwork/server.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/server.py), [`getwork/detect.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/detect.py)). Exposing this tool would create an SSRF/browser-pivot surface.
- **File-to-email exfiltration:** `send_email` accepts an attachment path; absolute paths are preserved and the mailer reads the selected file before sending it ([`getwork/server.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/server.py), [`getwork/mailer.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/mailer.py)). A model-facing production tool must instead use owner-issued artifact IDs and fixed recipients/consent policy.
- **Untrusted content rendering:** crawled text and agent-authored Markdown become HTML email. The current Markdown/Jinja path has no explicit HTML sanitization policy ([`getwork/briefing.py`](https://github.com/RyaoVen/getWork/blob/master/getwork/briefing.py)).
- **Privacy leak in the public tree:** the committed sample profile contains a real-looking recipient email and resume filename ([`config/profile.yaml`](https://github.com/RyaoVen/getWork/blob/master/config/profile.yaml)). HENUKit must never use repository-backed shared profile files for user data.
- **Operational fragility:** many sources depend on reverse-engineered web APIs, browser response capture, selectors, anti-bot behavior, and a hard-coded Moka decryption method. Each source needs health, provenance, rate-limit, terms, and failure-state handling before production use.

## Recommended HENUKit integration shape

Use the authorized upstream MCP as an internal Career job source, not as a raw LangBot plugin and not as browser-to-MCP access.

The smallest credible first slice is read-only:

1. Pin and checksum the directly authorized upstream revision instead of copying its crawler implementations into Career.
2. Run its existing MCP server on the private Compose network with one deployment-owned bearer credential and an allowlisted source registry.
3. Let Career call only `list_sources` and `crawl_jobs` through the official Go MCP SDK; do not expose `add_source`, `login`, arbitrary SMTP, filesystem paths, or raw Playwright controls.
4. Let Portal Gateway derive the actor and preferences from HENUKit-owned state. LangBot receives only typed search inputs and normalized results, never resume files, email credentials, cookies, or reusable assertions.
5. Render results through the finite HENUKit assistant-card/action contract with fixed trusted application URLs. Email/digest delivery, saved profiles, authenticated sources, and automated application flows should be separate, consented tickets.

Before implementation, resolve product ownership and data-retention decisions, upstream licensing, which companies/sources are permitted, whether resumes are uploaded or only structured preferences are stored, and whether this is a Portal-only capability or also has a Console operations surface.
