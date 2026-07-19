# Portal Summary Context

## Owns

- The versioned, read-only Portal Console summary endpoint.
- Deployment metadata supplied by the protected Portal release pipeline.
- Bounded live probes for Portal readiness, key public pages, product entries, and the optional Portal feedback summary source.
- Verification and replay defense for the Portal-only Console Gateway credential.

## Does not own

- Portal content, navigation editing, deployment, rollback, or version switching.
- Console Sessions, Platform Core authorization, product databases, or feedback writes.
- Historical availability or incident persistence; current probe failures are reported as current exceptions only.

## Current boundary

HC-11 provides honest read-only Portal state. Missing optional feedback data is reported as partial, never synthesized. Portal Configuration continues through Git, PR review, and CI/CD.
