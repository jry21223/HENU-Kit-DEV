import assert from "node:assert/strict";
import {
  buildEasypayPaymentParams,
  buildEasypayPaymentUrl,
  buildEasypaySignString,
  formatMoney,
  signEasypayParams,
  validateEasypaySignedParams,
  verifyEasypaySignature,
  type EasypayConfig,
} from "../../src/lib/easypay";

const config: EasypayConfig = {
  pid: "1001",
  key: "secret",
  gatewayUrl: "https://pay.example.com/submit.php",
  paymentType: "wxpay",
  appUrl: "https://review.example.com",
  configured: true,
};

const params = {
  pid: "1001",
  type: "alipay",
  out_trade_no: "order-1",
  notify_url: "https://example.com/notify",
  return_url: "https://example.com/return",
  name: "Discrete Math Package",
  money: "19.90",
  empty: "",
  sign_type: "MD5",
};

const signString = buildEasypaySignString(params);
assert.equal(
  signString,
  "money=19.90&name=Discrete Math Package&notify_url=https://example.com/notify&out_trade_no=order-1&pid=1001&return_url=https://example.com/return&type=alipay",
);

const sign = signEasypayParams(params, "secret");
const signedParams = { ...params, sign };
assert.equal(verifyEasypaySignature(signedParams, "secret"), true);
assert.equal(validateEasypaySignedParams(signedParams, config), true);

assert.equal(verifyEasypaySignature({ ...params, sign: "bad-sign" }, "secret"), false);
assert.equal(validateEasypaySignedParams({ ...signedParams, money: "0.01" }, config), false);
assert.equal(validateEasypaySignedParams({ ...signedParams, pid: "9999" }, config), false);
assert.equal(validateEasypaySignedParams({ ...signedParams, sign_type: "SHA256" }, config), false);
assert.equal(validateEasypaySignedParams({ ...params }, config), false);

const paymentParams = buildEasypayPaymentParams({
  orderId: "order-2",
  name: "Probability Package",
  money: "9.90",
  config,
});

assert.equal(paymentParams.pid, "1001");
assert.equal(paymentParams.out_trade_no, "order-2");
assert.equal(paymentParams.type, "wxpay");
assert.equal(paymentParams.notify_url, "https://review.example.com/api/payments/easypay/notify");
assert.equal(paymentParams.return_url, "https://review.example.com/api/payments/easypay/return");
assert.equal(paymentParams.sign_type, "MD5");
assert.equal(validateEasypaySignedParams(paymentParams, config), true);

const paymentUrl = buildEasypayPaymentUrl(paymentParams, config.gatewayUrl);
assert.equal(paymentUrl?.startsWith("https://pay.example.com/submit.php?"), true);
assert.equal(paymentUrl?.includes("sign="), true);

assert.equal(formatMoney("1"), "1.00");
assert.equal(formatMoney("1.2"), "1.20");
assert.equal(formatMoney("-1"), null);
assert.equal(formatMoney("abc"), null);

console.log("easypay unit tests passed");
