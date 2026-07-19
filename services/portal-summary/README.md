# Portal Summary

Portal Summary is a small read-only owner service for the HENUKit Console Portal module. It exposes `GET /api/v1/console-summary` and never edits Portal content or controls releases.

The protected Portal release pipeline supplies `PORTAL_VERSION`, `PORTAL_COMMIT_SHA`, and `PORTAL_DEPLOYED_AT`. The service concurrently probes the configured readiness URL, key pages, and product entrances with bounded timeouts. An optional feedback endpoint may return `{ "pending_count": 0, "recent_count": 0, "as_of": "..." }`; when it is absent or unavailable, the summary is explicitly `partial`.

The endpoint accepts only Portal-specific Basic + HMAC credentials configured in both this service and Console Gateway. The verifier accepts one active key and an optional retiring key during a bounded deployment grace window; removing the retiring key revokes it. Timestamp and signature are validated before the nonce is claimed in Redis, and replayed nonces are rejected.

The optional feedback-summary hop uses a second audience-specific Basic + HMAC credential. It cannot reuse either inbound Gateway verification secret. The feedback owner is responsible for the same timestamp, nonce, signature, and replay checks.

Portal navigation, copy, and page structure remain repository-owned Portal Configuration changed through Git, PR review, and CI/CD. There are no content editor, deploy, rollback, or version-switch APIs.
