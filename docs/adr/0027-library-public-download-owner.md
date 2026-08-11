---
status: accepted
amends: 0020, 0025, 0026
---

# Library owns public download grants and download-start facts

ADR-0020 made `services/library` independently deployable but left its Console
operations behind the Study Legacy compatibility adapter. ADR-0025 activates a
local public tree and catalog, while ADR-0026 proves that one sealed asset can
reach private OSS without activation. The first OSS download slice needs a
smaller owner decision: Library, rather than Portal, Portal API, OSS logs, or
Study Legacy, owns whether one active public-free material may receive a
download grant and whether that grant counts.

This adopts issue #250 option A only for the bounded public-material catalog and
download-start surface. It does not claim that the remaining legacy operations
workspace has already been rebuilt.

## Decision

- Library owns the active public-material snapshot, short-lived download-grant
  decision, append-only download-start ledger, and material/global aggregates.
  The reviewed source manifest remains the publication fact source; the sealed
  release and OSS publisher own object identity and verification; OSS/SLS logs
  are asynchronous reconciliation evidence only.
- An **Active Public Material** is a Library-owned record in the current active
  release that is `published`, `public_free`, and bound to an exact material ID,
  release ID, receipt digest, Object key, OSS VersionId, byte count, SHA-256, and
  safe attachment filename. A browser supplies only the material ID. It cannot
  select a Bucket, Endpoint, Object key, VersionId, TTL, filename, access mode,
  or signing command.
- Portal exposes the one browser route
  `GET /api/v1/library/materials/{material_id}/download`. Portal Gateway handles
  that exact route before its legacy Library wildcard and makes one explicitly
  signed owner call. An absent owner client or failed owner call returns an
  honest unavailable response; it never falls back to Portal API, `/materials/`,
  a `storage_key`, local mock data, or a second public download surface.
- The owner call is
  `POST /api/v1/public-materials/{material_id}/download-starts`. It uses a
  dedicated least-privilege service credential and needs no Portal Session or
  invented actor UUID: this ticket permits only anonymous public-free material.
  The browser body, cookies, query, filename, and storage fields are not
  forwarded. On success Library returns one method-`GET` grant with its
  expiration and transient signed location; Portal Gateway validates method
  `GET`, the fixed HTTPS public OSS host and the 60-second bound before returning
  `303`. The owner success envelope contains only `download_start_id`, `method`,
  `location`, and `expires_at`, plus the normal `request_id`. The owner and
  browser responses are `no-store`.
- Before signing, Library re-reads the current active snapshot and HEADs the
  exact Object key plus VersionId in Bucket `henukit`, region `cn-beijing`,
  through `oss-cn-beijing-internal.aliyuncs.com`. It
  requires the expected version, bytes, SHA-256 metadata, and private object
  boundary. It then creates a public-Endpoint presigned `GET` for that exact
  version, with a fixed maximum lifetime of 60 seconds and a sanitized
  `Content-Disposition: attachment` filename. The only accepted browser grant
  host is `henukit.oss-cn-beijing.aliyuncs.com`. The runtime role needs only the
  bounded object-read capabilities; it has no List, Put, Delete, ACL, policy, or
  Bucket-configuration authority.
- After signing, the ledger insert atomically rechecks that the same
  material/release/object snapshot is still active. If that compare-and-insert
  does not create exactly one immutable event, the grant is discarded and no
  redirect is returned. Unknown, inactive, withdrawn, non-free, missing,
  mismatched, unsignable, or unrecordable materials therefore create no
  successful event.
- A **Download Start** means Library persisted one successfully signed grant.
  It does not claim that Portal Gateway delivered the `Location`, that the
  browser followed it, or that OSS transferred every byte. A client disconnect
  after the ledger commit does not retract the fact. Separate successful
  browser requests create separate events; failures never do.
- Download-start events bind an opaque event ID, material ID, release ID,
  receipt digest, exact OSS VersionId, request ID, issued time, and grant expiry.
  They are append-only and never store a signed URL, security token, AccessKey,
  IP address, User-Agent, Cookie, or Portal Session. The global aggregate counts
  every successful event since the first OSS activation and does not decrease
  when a material is later withdrawn; the material aggregate uses the stable
  owner material ID.
- The signed URL is an intentionally bounded bearer capability. The `303`
  `Location` may contain only the OSS presign parameters required for the exact
  Bucket/Object/VersionId, `GET`, attachment response, and at most 60 seconds.
  Pages, JSON exposed to page code, logs, ledgers, error bodies, and telemetry
  must not retain the complete URL, raw ECS role credentials, long-lived
  AccessKeys, the internal Endpoint, or a Bucket listing. The browser response
  is `Cache-Control: no-store` and `Referrer-Policy: no-referrer`.
- The browser route returns `404 MATERIAL_NOT_AVAILABLE` for an unknown,
  inactive, withdrawn, or non-free material without distinguishing those
  states: “资料不存在或已下架，请返回资料库重新选择。” Object verification,
  signing, ledger, or owner availability failures return
  `503 DOWNLOAD_TEMPORARILY_UNAVAILABLE`: “暂时无法生成下载链接，请稍后重试。”
  Neither error includes storage identity or a `Location` header.
- Owner aggregate reads are exact signed routes:
  `GET /api/v1/public-materials/download-starts/aggregate` and
  `GET /api/v1/public-materials/{material_id}/download-starts/aggregate`.
  They return the count, `counting_since`, and `as_of`; the material form also
  returns `material_id`. OSS/SLS data is never joined into the synchronous
  result.
- Issue #333 may create a controlled test fixture containing one active owner
  snapshot, but it installs no production activation writer and changes no
  current release. Issue #334 publishes and atomically activates the complete
  owner catalog. Issue #335 supplies production role, canary, download,
  aggregate, rollback, and no-long-lived-credential evidence.

## Consequences

- Library gains one real independently owned business fact without pretending
  that its whole legacy Console workspace has already migrated.
- Portal and Portal API cannot turn a storage field into authority. Only the
  exact Gateway-to-owner seam can create a browser download capability.
- The cumulative number is truthful but deliberately narrow: accepted signed
  grants since OSS activation, not unique people, completed transfers, or
  reconstructed historical Nginx downloads.
- A signed URL is visible only as the short-lived redirect capability required
  for a private OSS download. Avoiding even that capability would require a
  proxy or CDN design outside this decision.
