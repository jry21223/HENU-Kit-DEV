# WeChat Pay Native

This document describes the current WeChat Pay Native integration status for final-review-platform. It is an internal-test and launch-readiness guide, not a statement that live collection is already open.

## Current Status

- WeChat Pay Native is the only current payment target.
- Yipay is not the active payment path.
- Development and test environments may use `WECHAT_PAY_MODE=mock`.
- Production must use `WECHAT_PAY_MODE=live`; mock payment is rejected.
- Live Native ordering code signs WeChat API v3 requests with the merchant private key and verifies the signed WeChat response before storing `code_url`.
- Live notify handling verifies callback signatures, decrypts `resource`, checks `appid`, `mchid`, `out_trade_no`, and integer-fen amount, then grants package entitlement idempotently only after `SUCCESS`.
- Real merchant end-to-end testing is still required before public paid sales.

## Environment

Required live variables:

```env
WECHAT_PAY_MODE=live
WECHAT_PAY_APPID=
WECHAT_PAY_MCH_ID=
WECHAT_PAY_API_V3_KEY=
WECHAT_PAY_MERCHANT_SERIAL_NO=
WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH=
WECHAT_PAY_PLATFORM_CERTS_DIR=
WECHAT_PAY_NOTIFY_URL=
WECHAT_PAY_NATIVE_EXPIRE_MINUTES=15
```

`WECHAT_PAY_API_V3_KEY` must be exactly 32 characters. Merchant private keys and platform certificates must be mounted through ignored `secrets/` and `certs/` directories, never committed.

Run production config preflight before exposing paid traffic:

```bash
cd services/api
go run ./cmd/preflight -env-file ../../.env.production
```

## Order Flow

1. User creates a package order through `POST /api/v1/orders`.
2. The server prices the order from `course_packages.price_fen`.
3. User requests Native payment through `POST /api/v1/payments/wechat/native`.
4. In live mode, the API calls WeChat `/v3/pay/transactions/native`.
5. The API stores `code_url`, `expires_at`, and sets local order status to `paying`.
6. The frontend renders the QR code and polls read-only order status.
7. WeChat calls `POST /api/v1/payments/wechat/notify`.
8. The API verifies and decrypts the callback.
9. On verified `SUCCESS`, the API marks the order `paid`, records `payment_records`, and grants package entitlement once.

Frontend polling never grants entitlement.

## Mock Mode

Development/test mock mode exists only to test backend boundaries without real merchant traffic:

```bash
cd services/api
go run ./cmd/smoke \
  -base-url http://localhost:8080/api/v1 \
  -email smoke-pay@stu.henu.edu.cn \
  -code 123456 \
  -mock-wechat-pay \
  -mock-wechat-secret mock-notify-secret
```

The API must run with `WECHAT_PAY_MODE=mock` and `WECHAT_PAY_API_V3_KEY=mock-notify-secret`. Mock Native ordering does not mark orders paid. Only a signed mock notify can mark the test order paid and grant entitlement.

Do not use mock smoke as proof that live merchant payment is ready.

## Live Native Smoke

Use this against a staging/live API after real merchant configuration passes preflight:

```bash
cd services/api
go run ./cmd/smoke \
  -base-url https://review.example.com/api/v1 \
  -email smoke-live@stu.henu.edu.cn \
  -code <email-code> \
  -package-id <positive-price-package-id> \
  -wechat-live-native
```

This smoke:

- logs in with a fresh student account;
- verifies paid material is denied before entitlement;
- creates a positive-price package order;
- requests a non-mock WeChat Native `codeUrl`;
- immediately calls `POST /payments/wechat/close`;
- verifies the order becomes `closed`;
- verifies no entitlement was granted.

Do not scan or pay the QR code during this smoke. It validates live ordering, response verification, local order state, and live close-order wiring. It does not validate successful payment notify or entitlement issuance from a real paid transaction.

## Notify Acceptance

A real payment acceptance test still requires:

- a low-risk internal package/order;
- a real WeChat scan and payment from an internal tester;
- official WeChat notify delivery to the configured HTTPS notify URL;
- order status changing to `paid`;
- exactly one `payment_records` row for the transaction;
- exactly one order-source package entitlement;
- paid material download succeeding only after the notify path grants entitlement.

## Close Order

`POST /api/v1/payments/wechat/close`:

- requires login;
- lets users close only their own pending/paying orders;
- lets admin/super_admin close any pending/paying order;
- rejects paid, closed, expired, and unsupported-provider orders;
- in live mode, calls WeChat close-order before local status changes for paying orders.

## Known Gaps

- Real merchant successful-payment E2E is not complete.
- Refund flow is not implemented.
- Automatic platform certificate rotation is not implemented.
- Live merchant settlement reconciliation is not implemented.
- Payment incident routing is basic: incidents can be recorded, counted, and handled manually, but there is no full operations escalation workflow yet.
