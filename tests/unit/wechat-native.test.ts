import assert from "node:assert/strict";
import {
  buildMockWechatNativeCodeUrl,
  createWechatNativePayment,
  WechatPayLiveNotImplementedError,
} from "../../src/lib/payments/wechat/native";
import { getWechatPayConfig, WechatPayConfigError } from "../../src/lib/payments/wechat/config";

const codeUrl = buildMockWechatNativeCodeUrl({ outTradeNo: "FR202606210001" });
assert.equal(codeUrl.startsWith("weixin://wxpay/bizpayurl?pr="), true);
assert.equal(codeUrl.includes("FR202606210001"), false);

async function main() {
  const mockResult = await createWechatNativePayment(
    {
      orderId: "order-1",
      outTradeNo: "FR202606210002",
      description: "Discrete Math Package",
      amountTotal: 1990,
    },
    getWechatPayConfig({
      NODE_ENV: "test",
      WECHAT_PAY_MODE: "mock",
      WECHAT_PAY_NATIVE_EXPIRE_MINUTES: "20",
    }),
  );
  assert.equal(mockResult.mode, "mock");
  assert.equal(mockResult.codeUrl.startsWith("weixin://wxpay/bizpayurl?pr="), true);
  assert.equal(mockResult.expiresAt.getTime() > Date.now() + 19 * 60 * 1000, true);

  await assert.rejects(
    () =>
      createWechatNativePayment(
        {
          orderId: "order-2",
          outTradeNo: "FR202606210003",
          description: "Bad Amount",
          amountTotal: 0,
        },
        getWechatPayConfig({ NODE_ENV: "test", WECHAT_PAY_MODE: "mock" }),
      ),
    /amountTotal must be a positive integer/,
  );

  await assert.rejects(
    () =>
      createWechatNativePayment(
        {
          orderId: "order-3",
          outTradeNo: "FR202606210004",
          description: "Production Mock",
          amountTotal: 100,
        },
        getWechatPayConfig({ NODE_ENV: "production", WECHAT_PAY_MODE: "mock" }),
      ),
    WechatPayConfigError,
  );

  await assert.rejects(
    () =>
      createWechatNativePayment(
        {
          orderId: "order-4",
          outTradeNo: "FR202606210005",
          description: "Live",
          amountTotal: 100,
        },
        getWechatPayConfig({
          NODE_ENV: "production",
          WECHAT_PAY_MODE: "live",
          WECHAT_PAY_APPID: "wx-app",
          WECHAT_PAY_MCH_ID: "mch-1",
          WECHAT_PAY_API_V3_KEY: "api-v3-key",
          WECHAT_PAY_MERCHANT_SERIAL_NO: "serial-1",
          WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH: "C:/secrets/apiclient_key.pem",
          WECHAT_PAY_PLATFORM_CERTS_DIR: "C:/certs",
          WECHAT_PAY_NOTIFY_URL: "https://review.example.com/api/payments/wechat/notify",
        }),
      ),
    WechatPayLiveNotImplementedError,
  );

  console.log("wechat native unit tests passed");
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
