import assert from "node:assert/strict";
import {
  getWechatPayConfig,
  validateWechatPayConfig,
  WechatPayConfigError,
  assertWechatPayConfig,
} from "../../src/lib/payments/wechat/config";

const devMock = getWechatPayConfig({
  NODE_ENV: "development",
  WECHAT_PAY_MODE: "mock",
});
assert.deepEqual(validateWechatPayConfig(devMock), []);

const testMock = getWechatPayConfig({
  NODE_ENV: "test",
  WECHAT_PAY_MODE: "mock",
});
assert.deepEqual(validateWechatPayConfig(testMock), []);

const productionMock = getWechatPayConfig({
  NODE_ENV: "production",
  WECHAT_PAY_MODE: "mock",
});
assert.match(validateWechatPayConfig(productionMock).join("\n"), /not allowed in production/);
assert.throws(() => assertWechatPayConfig(productionMock), WechatPayConfigError);

const liveMissingConfig = getWechatPayConfig({
  NODE_ENV: "production",
  WECHAT_PAY_MODE: "live",
});
const liveMissingErrors = validateWechatPayConfig(liveMissingConfig).join("\n");
assert.match(liveMissingErrors, /WECHAT_PAY_APPID/);
assert.match(liveMissingErrors, /WECHAT_PAY_MCH_ID/);
assert.match(liveMissingErrors, /WECHAT_PAY_API_V3_KEY/);
assert.match(liveMissingErrors, /WECHAT_PAY_MERCHANT_SERIAL_NO/);
assert.match(liveMissingErrors, /WECHAT_PAY_PLATFORM_CERTS_DIR/);
assert.match(liveMissingErrors, /WECHAT_PAY_NOTIFY_URL/);
assert.match(liveMissingErrors, /WECHAT_PAY_MERCHANT_PRIVATE_KEY/);

const liveCompleteConfig = getWechatPayConfig({
  NODE_ENV: "production",
  WECHAT_PAY_MODE: "live",
  WECHAT_PAY_APPID: "wx-app",
  WECHAT_PAY_MCH_ID: "mch-1",
  WECHAT_PAY_API_V3_KEY: "api-v3-key",
  WECHAT_PAY_MERCHANT_SERIAL_NO: "serial-1",
  WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH: "C:/secrets/apiclient_key.pem",
  WECHAT_PAY_PLATFORM_CERTS_DIR: "C:/certs",
  WECHAT_PAY_NOTIFY_URL: "https://review.example.com/api/payments/wechat/notify",
});
assert.deepEqual(validateWechatPayConfig(liveCompleteConfig), []);

console.log("production payment config unit tests passed");
