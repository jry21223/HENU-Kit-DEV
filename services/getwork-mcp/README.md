# getWork MCP runtime

This image runs the original [`RyaoVen/getWork`](https://github.com/RyaoVen/getWork)
MCP server at the authorized, pinned commit
`2c7800d65fb22d5094d812107c63ce94734b1c2e`. The owner gave Jerry explicit
permission to connect and use it for HENU Kit (reported 2026-08-26 as
“你直接接去用”). This records project authorization; it does not claim that
the upstream repository has added the missing root `LICENSE` file.

The upstream source and crawler implementation are unchanged. This directory
adds only the production transport boundary:

- Streamable HTTP at `/mcp`;
- a deployment-owned Bearer token;
- `/healthz` for container supervision;
- a fixed upstream commit and archive checksum.
- an exposed-tool allowlist containing only `list_sources` and `crawl_jobs`.

Career calls only `list_sources` and `crawl_jobs`. It does not expose or reuse
upstream `login`, `add_source`, `render_briefing`, or `send_email`.
