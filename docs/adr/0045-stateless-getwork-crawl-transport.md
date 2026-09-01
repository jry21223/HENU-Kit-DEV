---
status: accepted
---

# Use bounded stateless MCP calls for remote getWork crawls

ADR-0042 required Career to call both getWork tools through the official Go
MCP SDK. ADR-0043 later moved the browser-bearing server across a restricted
reverse SSH tunnel. Production acceptance showed that the SDK's Streamable
HTTP session transport is not safe on that boundary: four per-source sessions,
one shared session, and a shared session requesting JSON responses each caused
the reverse SSH connection to close with `Broken pipe` after about 20 seconds.
The same relay completed four independent stateless `crawl_jobs` requests in
176 seconds without restarting the tunnel.

## Decision

- This ADR amends only ADR-0042's transport requirement for `crawl_jobs`.
  Career still uses the official Go MCP SDK at startup to negotiate the
  protocol, reject unexpected tools, call `list_sources`, and validate every
  discovered source against the fixed official-URL policy.
- A Work Radar scan calls `crawl_jobs` through a project-owned stateless
  Streamable HTTP client. Each source uses one independent JSON-RPC POST with
  the deployment-owned endpoint and bearer credential; the browser, actor,
  profile, and model cannot select the endpoint, method, token, or source set.
- Every POST carries the exact protocol version negotiated by the startup SDK
  probe. Protocols at or after `2026-07-28` also carry the required per-request
  metadata plus `Mcp-Method` and `Mcp-Name` standard headers. Career rejects a
  `crawl_jobs` schema containing unsupported `x-mcp-header` annotations rather
  than silently omitting required parameter headers.
- The client keeps the existing six-minute scan deadline and maximum
  concurrency of four. It rejects redirects, non-2xx responses, responses over
  8 MiB, unsupported or missing media types, unframed event streams, JSON-RPC
  versions or IDs that do not match the request, malformed events, ambiguous
  multiple responses, server calls, protocol errors, tool errors, and absent
  or malformed results. Well-formed server notifications may precede the one
  matching response.
- Raw JSON is accepted only as `application/json`; event data is accepted only
  as a correctly framed `text/event-stream`. Result decoding continues through
  the official SDK's `CallToolResult` types before Career normalizes jobs.
- Tunnel loss remains fail-closed. Production never starts a local crawler,
  removes a source, retries through an arbitrary endpoint, or treats a health
  response as a completed search.
- Tests must cover JSON and SSE success, media-type and framing rejection,
  response-size and response-ID bounds, bearer and redirect behavior, all
  discovered sources, bounded concurrency, and the production actor-scoped
  acceptance scan. An upstream or SDK upgrade must repeat the WSL reconnect and
  full Career acceptance gates.

## Considered options

- **One SDK session per source:** preserves ADR-0042 literally, but repeatedly
  terminated the production SSH tunnel during the first four crawls.
- **One shared SDK session:** reduced initialization count but failed at the
  same production boundary, including when JSON responses were requested.
- **Serial SDK calls:** avoids concurrent sessions but cannot reliably scan all
  18 sources inside the six-minute product deadline; four parallel calls alone
  required 176 seconds in the production comparison.
- **Public HTTPS MCP:** avoids SSH multiplexing but introduces a public service
  and certificate boundary that ADR-0043 intentionally rejected.

## Consequences

- Career owns a small protocol transport for `crawl_jobs` in addition to its
  existing normalization adapter. The security and compatibility matrix above
  is therefore release-critical and must remain explicit.
- The official SDK remains the startup authority for protocol, tool, and source
  discovery; this is not permission to add tools or implement a parallel
  crawler.
- Career continues to own profiles, matching, task state, persistence, history,
  and digest delivery. WSL continues to run only the pinned browser crawler.
- ADR-0043's private tunnel, forwarding-only identity, provenance, reconnect,
  rollback, and no-local-fallback requirements remain unchanged.
