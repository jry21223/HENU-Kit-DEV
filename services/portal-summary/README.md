# Portal Summary

Portal Summary is a small read-only owner service for the HENUKit Console Portal module. It exposes `GET /api/v1/console-summary` and never edits Portal content or controls releases.

The protected Portal release pipeline supplies `PORTAL_VERSION`, `PORTAL_COMMIT_SHA`, and `PORTAL_DEPLOYED_AT`. The service concurrently probes the configured readiness URL, key pages, and product entrances with bounded timeouts. An optional feedback endpoint may return `{ "pending_count": 0, "recent_count": 0, "as_of": "..." }`; when it is absent or unavailable, the summary is explicitly `partial`.

The endpoint accepts only the Portal-specific Basic + HMAC credential configured in both this service and Console Gateway. Timestamp and signature are validated before the nonce is claimed in Redis, and replayed nonces are rejected.

Portal navigation, copy, and page structure remain repository-owned Portal Configuration changed through Git, PR review, and CI/CD. There are no content editor, deploy, rollback, or version-switch APIs.
