# HENU Kit EasyPay gateway patch

The production gateway is a server-local service outside this repository.
`patches/0001-henukit-query-and-notify-outbox.patch` is the versioned source
change that must be applied to that service before the HENU tenant can be
enabled.

The patch:

- replaces the unauthenticated local-store query with a tenant-signed
  `POST /api/query.php`;
- prevents cross-tenant order queries;
- persists downstream notification attempts and retry deadlines in the order
  store;
- retries failed merchant notifications with bounded exponential backoff;
- changes JSON persistence to an atomic owner-only file replacement.

Apply and verify in a staging copy:

```sh
patch -p1 < patches/0001-henukit-query-and-notify-outbox.patch
npm test
```

The HENU tenant and Account Portfolio Provider remain disabled until this patch,
the public callback ingress, database migration, and end-to-end smoke checks
are deployed in one authorized payment release.
