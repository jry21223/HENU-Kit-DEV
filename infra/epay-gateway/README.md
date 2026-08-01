# HENU Kit EasyPay gateway patches

The production gateway is a server-local service outside this repository
(`metaview-epay-gateway`). The files under `patches/` are the versioned source
changes that must be applied to that service, in order, before the HENU tenant
can be enabled.

## 0001 — tenant-signed query and notification outbox

- replaces the unauthenticated local-store query with a tenant-signed
  `POST /api/query.php`;
- prevents cross-tenant order queries;
- persists downstream notification attempts and retry deadlines in the order
  store;
- retries failed merchant notifications with bounded exponential backoff;
- changes JSON persistence to an atomic owner-only file replacement.

## 0002 — close, refund, and WeChat response verification

Adds the operator commands #203 needs, and closes the two verification gaps the
payment provider spike recorded.

- `POST /api/close.php`, `POST /api/refund.php`, and `POST /api/refund-query.php`,
  each tenant-signed and each resolving the order only inside the calling
  tenant, so one merchant can never close or refund another merchant's order;
- refund amounts always come from the stored order, never from the request;
- a refund is durably reserved against `out_refund_no` before submission, so
  duplicate and concurrent commands settle on exactly one refund; a reservation
  that never recorded a WeChat refund id is re-submitted, which is safe because
  `out_refund_no` makes the WeChat refund idempotent;
- closing asks WeChat first, so a paid order is refused rather than closed on
  stale local state;
- WeChat API **responses** are now signature-verified. Previously only callbacks
  were verified and API responses were trusted unconditionally;
- the verifying platform key is selected by `Wechatpay-Serial`, so certificate
  rotation is a configuration change. An unknown serial fails closed.

### Operational note before deploying 0002

This merchant runs in WeChat's **public-key mode**: `WECHAT_PLATFORM_CERT_PATH`
points at a `BEGIN PUBLIC KEY` file (the 微信支付公钥), not an X.509 platform
certificate. That is a supported configuration here:

- `WECHAT_PLATFORM_CERTS_DIR` finds no parseable certificate, so the serial
  registry stays empty and the gateway verifies with the single configured
  public key — the same key that already verifies callbacks today;
- WeChat signs API responses with the same platform private key it uses for
  callbacks, so enforcing response verification needs no new key material;
- serial-based selection therefore stays dormant in this mode. It matters only
  if this merchant is ever migrated to platform certificates, where staging the
  current and incoming certificate together in `WECHAT_PLATFORM_CERTS_DIR` makes
  rotation a configuration change instead of an outage.

The verifying key is already proven current: callbacks are verified with it and
live collection is working, so a stale key would have broken payments before
this patch. Public keys are also long-lived and merchant-rotated, unlike
platform certificates, so enforcing response verification does not introduce an
expiry-driven outage risk here.

## 0003 — private checkout handle

Delivers the checkout surface ADR-0019 decided, without ever putting a private
merchant order number in a browser URL.

- a merchant now declares `hostedCheckout`. A tenant whose order numbers are
  private (HENU Kit) sets it false and receives WeChat's own `weixin://` payment
  URI as its `code_url`, instead of the gateway's `/pay/{out_trade_no}` page;
- the gateway-hosted checkout page and the unauthenticated status probe stay 404
  for such a tenant, and the guards now key off that flag rather than a
  hard-coded tenant name;
- MetaView keeps the hosted checkout page it has always used, unchanged;
- the authoritative WeChat URI is still stored server-side for reconciliation.

## Apply and verify in a staging copy

```sh
patch -p1 < patches/0001-henukit-query-and-notify-outbox.patch
patch -p1 < patches/0002-henukit-close-refund-and-response-verification.patch
patch -p1 < patches/0003-henukit-private-checkout-handle.patch
npm test
```

The HENU tenant and Account Portfolio Provider remain disabled until these
patches, the public callback ingress, database migration, and end-to-end smoke
checks are deployed in one authorized payment release.
