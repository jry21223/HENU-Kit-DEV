---
status: accepted
---

# Use the authorized getWork MCP as Career's internal job source

The upstream owner directly authorized HENU-Kit to use `RyaoVen/getWork`, as
reported by Jerry on 2026-08-26 with the words “你直接接去用”. The
repository's public license file is still absent, so this is project-specific
permission rather than a general MIT-license claim.

Career already owns profile matching, result persistence, AI provider access,
and controlled digest delivery through Platform Core. Copying getWork's mail,
profile, or agent workflow would duplicate those owners.

## Decision

- Run upstream commit `2c7800d65fb22d5094d812107c63ce94734b1c2e`
  unchanged as the crawler/MCP implementation. Verify its source archive digest
  during the image build.
- Add only a deployment wrapper that selects Streamable HTTP, exposes a health
  check, and requires a private bearer credential. The service is reachable
  only on the internal Compose network.
- The same-host Docker daemon and root operator are trusted deployment
  principals. Plain HTTP is accepted only inside that bridge after the server
  removes every tool except `list_sources` and `crawl_jobs`; publishing the
  endpoint, moving it across hosts, or adding write/credential tools requires
  TLS and a new credential-boundary decision.
- Career calls `list_sources` and `crawl_jobs` using the official Go MCP SDK.
  Source keys come from an operator-owned allowlist. Browser clients and model
  prompts never select endpoints, credentials, or arbitrary sources.
- Career continues to own normalization, deterministic Opportunity Match,
  durable search results, and the existing Platform Core digest-mail path.
  getWork's `send_email`, `render_briefing`, `add_source`, and `login` tools are
  outside this integration.
- Default upstream sources require no source login. A future authenticated job
  source requires a separate credential-isolation and consent decision; HENU
  Account login is never reused as recruitment-site login.
- GitHub Actions builds and verifies the pinned MCP image. Production activation
  keeps the two idempotent Career digest-mail migrations mandatory so a release
  cannot reintroduce the observed mail-outbox schema drift.

## Consequences

- HENU uses the original MCP protocol and crawler behavior without maintaining
  a parallel crawler adapter layer.
- The thin boundary remains necessary for transport authentication, source
  allowlisting, and conversion into Career's owned Job Opportunity contract.
- Upstream upgrades are explicit: change the commit and checksum together,
  re-run protocol/crawl smoke tests, and recheck authorization and licensing.
