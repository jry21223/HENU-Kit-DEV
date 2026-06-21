import assert from "node:assert/strict";
import {
  buildEasypayPaymentParams,
  buildEasypaySignString,
  formatMoney,
  signEasypayParams,
  verifyEasypaySignature,
} from "../../src/lib/easypay";

const params = {
  pid: "1001",
  type: "alipay",
  out_trade_no: "order-1",
  notify_url: "https://example.com/notify",
  return_url: "https://example.com/return",
  name: "离散数学期末复习包",
  money: "19.90",
  empty: "",
  sign_type: "MD5",
};

const signString = buildEasypaySignString(params);
assert.equal(
  signString,
  "money=19.90&name=离散数学期末复习包&notify_url=https://example.com/notify&out_trade_no=order-1&pid=1001&return_url=https://example.com/return&type=alipay",
);

const sign = signEasypayParams(params, "secret");
assert.equal(verifyEasypaySignature({ ...params, sign }, "secret"), true);
assert.equal(verifyEasypaySignature({ ...params, sign: "bad-sign" }, "secret"), false);

const paymentParams = buildEasypayPaymentParams({
  orderId: "order-2",
  name: "测试课程包",
  money: "9.90",
  config: {
    pid: "1001",
    key: "secret",
    gatewayUrl: "https://pay.example.com/submit.php",
    paymentType: "wxpay",
    appUrl: "https://review.example.com",
    configured: true,
  },
});

assert.equal(paymentParams.out_trade_no, "order-2");
assert.equal(paymentParams.type, "wxpay");
assert.equal(paymentParams.notify_url, "https://review.example.com/api/payments/easypay/notify");
assert.equal(paymentParams.return_url, "https://review.example.com/api/payments/easypay/return");
assert.equal(paymentParams.sign_type, "MD5");
assert.equal(verifyEasypaySignature(paymentParams, "secret"), true);

assert.equal(formatMoney("1"), "1.00");
assert.equal(formatMoney("1.2"), "1.20");
assert.equal(formatMoney("-1"), null);
assert.equal(formatMoney("abc"), null);

console.log("easypay unit tests passed");
