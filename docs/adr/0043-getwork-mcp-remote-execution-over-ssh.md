---
status: accepted
---

# Run the Job Source MCP on a remote WSL node over a restricted SSH tunnel

ADR-0042's ownership and read-only tool boundary remain unchanged, but the browser-bearing getWork MCP may run on Jerry's always-on WSL2 node instead of the capacity-constrained HENUKit production host. The WSL node initiates a supervised SSH tunnel with a dedicated forwarding-only identity; HENUKit exposes the tunnel only to the internal Career service, continues to require the deployment-owned bearer credential, and never copies a production root key to WSL.

The remote node runs the complete pinned getWork source set unchanged and exposes only `list_sources` and `crawl_jobs`. Career continues to own the Career Profile, Opportunity Match, persistence, task state, and Opportunity Digest. Tunnel or WSL failure fails the search safely and never starts a local browser crawler as an automatic fallback.

The preferred WSL release handoff is built on the exact `main` commit by the
repository GitHub Actions workflow. That workflow attests a deterministic
manifest binding only the getWork image archive and runtime archive to their
SHA-256 digests, source repository, source ref, workflow, platform, and full
source SHA. Before executing the runtime installer, the WSL operator verifies
that manifest with the root-owned operating-system GitHub CLI and the downloaded
attestation bundle. Verification pins the repository, workflow, `main` ref,
source digest, and GitHub-hosted runner. The installer and node verifier repeat
the same check, and both compare the release SHA with a freshly fetched current
`origin/main` before activation. Release-workflow Actions are pinned to immutable
commit SHAs so a movable upstream tag cannot silently change the trusted build
or attestation path. The existing SSH-signed local-builder handoff remains a
mutually exclusive fallback when Actions is unavailable; it is not combined
with the Actions trust path.

## Considered Options

- Running the complete crawler on production preserves the original same-host bridge but exceeds the intended production resource boundary.
- Publishing the MCP directly over public HTTPS removes SSH supervision but creates a public service and certificate boundary that this deployment does not need.
- A restricted SSH tunnel keeps the MCP private, reuses the existing bearer boundary, and lets WSL supply the browser workload without moving Career-owned data or responsibilities.

## Consequences

- The tunnel identity, WSL crawler service, production-private relay, health probes, and rollback procedure become release-critical operational assets.
- The Actions path needs OIDC attestation permissions and a trusted OS GitHub
  CLI on WSL. It carries no long-lived release-signing private key. If
  attestation trust roots cannot be resolved or any pinned identity differs,
  installation fails closed.
- Release archive permissions are normalized during packaging so an ext4
  checkout with a permissive umask cannot make runtime programs group-writable.
- Plain HTTP is permitted only on each host's private loopback/internal bridge segments inside the end-to-end SSH transport; the MCP is never published on a public listener.
- Production activation requires proof of the complete discovered source set, a real normalized Job Opportunity result when upstream data exists, supervised reconnect behavior, and safe failure when the tunnel is unavailable.
