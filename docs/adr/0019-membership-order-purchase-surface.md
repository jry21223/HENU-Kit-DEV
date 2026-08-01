---
status: accepted
amends: 0017
---

# Portal Gateway forwards a user's own membership order creation

ADR-0017 reserved its Portal write exception to a user's own support tickets and
notifications and required any further product command to make its own decision.
This is that decision for the one remaining self-service command: creating the
user's own ¥9.9 lifetime membership order.

Portal Gateway may forward an authenticated Portal Session user's membership
order creation to Account Portfolio under exactly the constraints ADR-0017
already established: the Gateway stays stateless and thin, never accepts a
browser-selected actor or service credential, binds the actor from the verified
Portal Session, signs the actor-bound request, requires a command idempotency
key, and fails closed on any dependency failure. No other membership order
command joins this exception: close, refund, and refund-status stay operator
commands on the Console Gateway path.

## The checkout handle

A membership order is paid by scanning a WeChat Native QR code. The private HNK
merchant order number must never reach a browser, so the browser is given the
WeChat `weixin://` payment URI itself rather than any URL that contains the
merchant order number.

The EasyPay gateway's `code_url` for the HENU tenant therefore returns the
authoritative WeChat Native URI, not the gateway's own `/pay/{out_trade_no}`
checkout page. That page stays unreachable for the HENU tenant, as does the
unauthenticated `/api/status/{out_trade_no}` probe. The merchant order number
stays server-side in every direction.

The `weixin://` URI is a WeChat-generated single-use payment code carrying no
merchant order number, so handing it to the browser leaks nothing. It is
returned only when the order is created or re-read while still awaiting payment,
and never as part of the general membership order representation that appears in
order listings.

## Consequences

- The exception now covers a user's own support tickets, notifications, and
  membership order creation. It still permits no Portal write for points,
  memberships, refunds, or any other product owner.
- Payment status remains server-confirmed. The browser polls its own order and
  never asserts that a payment succeeded; entitlement is granted only from a
  verified payment fact.
- A checkout URI that is not a `weixin://` URI, or that contains the merchant
  order number, is rejected rather than shown, so a gateway regression cannot
  reintroduce the leak.
- The gateway's HENU checkout page and unauthenticated status probe stay 404
  permanently; they are not a fallback.
- Enabling this surface still requires the payment Provider to be enabled, which
  remains a separate authorized release decision.
