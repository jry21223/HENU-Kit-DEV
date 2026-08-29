---
status: accepted
---

# Run the Job Source MCP on a remote WSL node over a restricted SSH tunnel

ADR-0042's ownership and read-only tool boundary remain unchanged, but the browser-bearing getWork MCP may run on Jerry's always-on WSL2 node instead of the capacity-constrained HENUKit production host. The WSL node initiates a supervised SSH tunnel with a dedicated forwarding-only identity; HENUKit exposes the tunnel only to the internal Career service, continues to require the deployment-owned bearer credential, and never copies a production root key to WSL.

The remote node runs the complete pinned getWork source set unchanged and exposes only `list_sources` and `crawl_jobs`. Career continues to own the Career Profile, Opportunity Match, persistence, task state, and Opportunity Digest. Tunnel or WSL failure fails the search safely and never starts a local browser crawler as an automatic fallback.

## Considered Options

- Running the complete crawler on production preserves the original same-host bridge but exceeds the intended production resource boundary.
- Publishing the MCP directly over public HTTPS removes SSH supervision but creates a public service and certificate boundary that this deployment does not need.
- A restricted SSH tunnel keeps the MCP private, reuses the existing bearer boundary, and lets WSL supply the browser workload without moving Career-owned data or responsibilities.

## Consequences

- The tunnel identity, WSL crawler service, production-private relay, health probes, and rollback procedure become release-critical operational assets.
- Plain HTTP is permitted only on each host's private loopback/internal bridge segments inside the end-to-end SSH transport; the MCP is never published on a public listener.
- Production activation requires proof of the complete discovered source set, a real normalized Job Opportunity result when upstream data exists, supervised reconnect behavior, and safe failure when the tunnel is unavailable.
