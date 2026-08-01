# Account Portfolio Payment Provider Spike

Status: the existing EasyPay compatibility route is selected and implemented
behind an explicit disabled-by-default gate. This document records the
read-only production evidence collected for #174 on 2026-07-30, the HENU Kit
order namespace decision, and the implementation evidence. It does not
authorize a real payment.

Evidence date: 2026-07-30. The "EasyPay" implementation is not an external
supplier product with a separate published protocol. It is the server-local
`metaview-epay-gateway` version 1.0.0, whose deployed source and live HTTP
behavior were inspected directly. Its compatibility contract is therefore
implementation evidence, not a vendor guarantee.

## Existing MetaView gateway

`metaview.top` runs a server-local Node/Express service named
`epay-gateway.service`. It accepts an EasyPay-compatible signed
`POST /submit.php`, creates a WeChat Pay API v3 Native order, serves the QR
checkout page, verifies and decrypts the WeChat notification, then forwards an
MD5-signed EasyPay notification to the caller's configured callback.

Observed production evidence:

- the supplied checkout returned HTTP 200 and a real WeChat Native `code_url`;
- its local order remained `pending`, which is expected before a user pays;
- the gateway store contained 35 orders: 30 `paid` and 5 `pending`;
- historical successful notifications received HTTP 200 `success` from
  MetaView;
- MetaView's unsigned EasyPay notification probe returned `fail`;
- the server could reach the WeChat API and received the expected
  unauthenticated HTTP 401 probe response;
- the gateway now listens only on `127.0.0.1:9219`; Nginx owns the public TLS
  boundary.

No credential values, payment payloads, or user data are recorded here.

## Protocol and security findings

The compatibility request uses the EasyPay fields `pid`, `type`,
`out_trade_no`, `notify_url`, `return_url`, `name`, `money`, `sign`, and
`sign_type=MD5`. The gateway sorts non-empty fields except `sign` and
`sign_type`, appends the tenant secret, and computes MD5. It performs WeChat
Native order creation and verifies the RSA/AES-GCM payment notification.

The current implementation is not yet a complete Account Portfolio Provider:

- its query endpoint reads only the gateway's local JSON order store and does
  not call WeChat's merchant-order query API;
- it has no close-order, refund, or refund-query implementation;
- notification verification does not enforce timestamp skew, replay storage,
  or `Wechatpay-Serial` key selection;
- WeChat API response signatures are not verified and platform key rotation is
  manual;
- failed downstream notification forwarding has no durable retry queue;
- the HENU public notification route currently returns HTTP 404;
- one JSON file and one process are not sufficient financial durability for
  Account Portfolio.

WeChat's current merchant documentation requires authenticated order queries
and recommends verifying callback signatures, serial identity, timestamp
freshness, and replay resistance. Refund handling requires querying the paid
order, applying an idempotent refund request, then querying or consuming refund
notification state:

- <https://pay.wechatpay.cn/doc/v3/merchant/4012526922>
- <https://pay.wechatpay.cn/doc/v3/merchant/4012791859>
- <https://pay.wechatpay.cn/doc/v3/merchant/4013053420>
- <https://pay.wechatpay.cn/doc/v3/merchant/4013071031>

Documentation versions observed on 2026-07-30: transaction-id query updated
2025-02-20, merchant-order query updated 2024-12-27, signature verification
updated 2024-11-29, and refund development guidance updated 2026-06-09.

## Route decision

| Capability | Existing EasyPay compatibility route | Direct WeChat API v3 route |
| --- | --- | --- |
| Merchant authentication | Shared-field MD5, now separated per tenant | RSA merchant request signature |
| Create | Implemented by forwarding to WeChat Native create | Required in the Account Portfolio adapter |
| Query | Local JSON status only; not authoritative | Official merchant-order and transaction-id query |
| Payment notification | WeChat RSA/AES-GCM, then MD5 forward | Verify raw WeChat notification in the adapter |
| Replay protection | No timestamp window or durable replay store | Required before enablement |
| Close | Missing | Official close-order call required |
| Refund | Missing | Query paid order, refund idempotently, query/consume result |
| Error semantics | Mostly plain 400/502 strings | Typed WeChat status/error mapping required |
| Domain boundary | MetaView works; HENU callback is HTTP 404 | HENU ingress and callback route still required |
| Key custody | Server-local root files and tenant secret | Dedicated adapter-only secret mount and rotation |

The product owner selected the existing EasyPay compatibility route. Account
Portfolio therefore implements a dedicated `easypay` Provider and does not
implement a direct WeChat adapter. The Provider remains disabled until the
versioned gateway patch, Account Portfolio service, and public ingress are
released together.

## Implemented production slice

The Account Portfolio `easypay` Provider now:

- signs create and query requests with the independent HENU tenant key;
- accepts only the fixed ¥9.90 lifetime product and `HNK` merchant order
  namespace;
- verifies the tenant PID, MD5 signature, amount, payment status, merchant
  prefix, and transaction identity on callbacks;
- queries the gateway before applying a callback;
- reconciles pending orders through the signed merchant-order query when the
  owner next lists membership orders;
- grants the lifetime entitlement, membership event, and notification in the
  existing single database transaction, with duplicate and concurrent
  callbacks remaining idempotent.

The Provider is constructed only when
`ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=1`; absent or incomplete configuration
continues to return the explicit Provider-unavailable behavior.

Because the gateway source is server-local, the deployable source delta is
versioned at
`infra/epay-gateway/patches/0001-henukit-query-and-notify-outbox.patch`.
It adds tenant-authenticated query backed by WeChat's merchant-order query,
cross-tenant rejection, atomic owner-only JSON persistence, a durable
downstream notification retry state, and bounded exponential retry.

## Remaining release blockers

- Public Nginx ingress must route the exact HENU notification path to Account
  Portfolio; it currently returns 404 in production.
- Account Portfolio and migration `000006` have not been deployed.
- The current OpenAPI contract has no operator close/refund command and the
  selected gateway has no close/refund endpoint. Those operations require a
  separate contract ticket rather than an uncallable adapter method.
- The owner purchase API intentionally remains outside Portal Gateway under
  ADR-0017, and the public order schema has no private-merchant-safe checkout
  handle. A purchase-surface ticket must define that boundary before users can
  open the QR checkout without exposing the merchant order number.
- Platform-certificate rotation and timestamp/replay hardening at the WeChat
  callback boundary remain gateway release gates.

## Merchant capability and domain boundary

The current merchant/AppID pair successfully created the supplied WeChat
Native order, which proves that the pair is bound and has working Native create
capability. Native payment passes `notify_url` in each create request; unlike
JSAPI, the official Native flow does not define a browser payment-directory
allowlist. The callback documentation instead requires a public HTTPS full
path without query parameters:

- <https://pay.wechatpay.cn/doc/v3/merchant/4015614538> (Native preparation,
  updated 2026-05-19);
- <https://pay.wechatpay.cn/doc/v3/merchant/4012791877> (Native create);
- <https://pay.wechatpay.cn/doc/v3/merchant/4012791882> (Native notification,
  updated 2024-12-27);
- <https://pay.wechatpay.cn/doc/v3/merchant/4012075420> (notification URL
  requirements, updated 2024-07-25).

`https://henukit.cn` already has public TLS, but the intended Account Portfolio
notification path returned HTTP 404 on 2026-07-30. Therefore no additional
Native "authorized directory" conclusion is required, but HENU ingress is
still a hard blocker: the exact callback must accept unauthenticated WeChat
POSTs, preserve the raw signed body and headers, and return the documented
acknowledgement within five seconds.

## Key custody, rotation, and rollback

Use four independent secret classes:

1. **Merchant API certificate private key and serial** — generated through the
   WeChat merchant certificate process; store the PEM as a read-only file
   owned by a dedicated payment-adapter OS identity, mode `0400`. Only the
   adapter receives the path and serial.
2. **API v3 key** — a 32-byte merchant-console value used only for callback
   resource decryption. Inject it from the server secret store as a root-owned
   `0600` environment file or credential mount; never place it in Compose,
   source control, logs, or browser configuration.
3. **WeChat platform public keys/certificates** — public verification material
   indexed by `Wechatpay-Serial`. Keep the current and next serial
   simultaneously and refresh from the official endpoint before expiry.
4. **HENU EasyPay tenant secret** — independent from MetaView/NewAPI, scoped to
   PID 2001, `HNK`, the exact HENU notification URL, and the `henukit.cn`
   return origin. Keep it only if the compatibility route is retained.

Rotation runbook:

1. back up the encrypted secret metadata and current non-secret serials;
2. install the new merchant private key/certificate under a versioned filename,
   update the serial atomically, then prove an authenticated order query before
   retiring the previous file;
3. refresh platform public material by serial and accept both current/next
   verification keys during overlap;
4. rotate the API v3 key only in a maintenance window because notification
   decryption changes atomically at the merchant account; deploy the new value,
   verify a signed callback probe and query, and retain the prior encrypted
   value only for the approved rollback window;
5. rotate the HENU tenant secret with explicit key IDs and a bounded dual-key
   verification window before switching the Account Portfolio adapter;
6. after the observation window, remove old material, record fingerprints and
   serials in audit logs, and verify that no process except the payment adapter
   can read the mounts.

Rollback restores the previous versioned credential mount and serial only when
the upstream merchant configuration still accepts it. Otherwise roll forward
with the newly issued credential; never copy MetaView's tenant secret into
HENU as a shortcut.

## Release-shape exception

Repository policy normally separates schema expansion, behavior migration, and
constraint contraction. This change is a one-time pre-promise-point exception:
Account Portfolio has not been deployed to production, the production process
has no Payment Provider, and the HNK migration itself refuses to run if any
payment intent exists. The owner requested the HENU-specific prefix before
Provider implementation on 2026-07-30. If any environment reports an existing
intent, stop and split the change into explicit expand/migrate/contract
releases instead of overriding the guard.

## HENU Kit namespace decision

HENU Kit uses a separate gateway tenant and a dedicated merchant-order prefix:

```text
HNK<29 uppercase Base32 characters>
```

The result is exactly 32 characters, fits WeChat's `out_trade_no` boundary,
and cannot collide with MetaView `mva...` or NewAPI `USR...` orders. Account
Portfolio persists it before calling a Provider and reuses it for retry,
query, callback correlation, and refund idempotency. The browser continues to
receive only its unrelated public Membership Order ID.

The MetaView server has a separate HENU tenant PID and secret, exact HENU
notification URL, `henukit.cn` return-origin allowlist, and `HNK` prefix
enforcement. The tenant is explicitly disabled and its secret has not been
installed into HENU Kit.

## Enablement gates

Keep the Account Portfolio Provider disabled until a later implementation and
release ticket proves all of the following:

1. the HENU callback route is publicly reachable and forwards raw signed
   notifications only to Account Portfolio;
2. the adapter implements create, authenticated WeChat query, notification
   verification with skew/replay protection, close, refund, and refund query;
3. merchant secrets and private keys are injected only into the payment
   adapter, with documented rotation and rollback;
4. payment orders, attempts, notifications, refunds, and forwarding retries
   are durable and reconciled against WeChat;
5. contract, race, migration, negative-signature, replay, fixed-amount, and
   production smoke checks pass;
6. a separately authorized internal ¥9.9 payment is reconciled end to end
   before public enablement.
