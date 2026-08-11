---
status: accepted
amends: 0022, 0023, 0024
---

# Materials use one fenced, journaled privileged activation path

## Decision

- Production has one materials webhook URL (`/webhooks/materials`), loopback
  listener, credential-loaded HMAC secret, and durable latest-arrival queue.
  The queue retains at most one running and one waiting delivery.
- The receiver remains `henukit-deploy`; the fixed queue consumer is a confined
  root service. An accepted event supplies only delivery, repository, ref, and
  SHA. The root orchestrator checks those values against root-owned configuration,
  runs preparation as `henukit-deploy`, seals independently as root, then passes
  only the sealed release ID and receipt digest to activation.
- Activation is serialized with `flock`. Before changing either surface it
  creates `/opt/henukit-materials/public/.maintenance`; Nginx returns 503 for
  `/materials/` and Library API routes while that file exists. Catalog rows use
  immutable `releases/<release-id>/<public-path>` storage keys; Nginx maps only
  those public URLs to `releases/<release-id>/public/` and never exposes
  `current`. The catalog import remains transactional.
- `activation-journal.json` records `prepared`, `static_switched`,
  `database_running`, and `database_committed`. Pre-database failure restores
  the previous pointer and marker. An uncertain database result stays fenced;
  rerunning the same approved release reconciles it idempotently. A
  `database_committed` retry completes the marker/pointer without rerunning SQL.
- The first immutable cutover archives legacy unversioned mirror rows only from
  a root-owned, reviewed legacy-key inventory plus exact paths in the approved
  manifest. Removed legacy assets therefore move in the same catalog transaction
  without treating checksums as an ownership marker.
- Rollback is a new forward activation of a retained, previously sealed release
  and its receipt digest. Direct file copying, manual catalog SQL, deletion of
  the fence/journal, and the retired sync scripts are not rollback paths.

## Consequences

- Public files and Library catalog are never intentionally observable on
  different releases; immutable URLs make the one-day public cache safe, while
  uncertainty fails closed behind the maintenance fence.
- Only the authenticated, accepted queue event supplies the exact source SHA;
  repository/ref policy, paths, commands, and the database target remain fixed
  outside the webhook payload.
- Accepted repository configuration is capability, not deployment evidence;
  enabling the watcher still requires host configuration and smoke verification.
