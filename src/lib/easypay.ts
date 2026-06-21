import { createHash, timingSafeEqual } from "node:crypto";

export type EasypayParams = Record<string, string | number | boolean | null | undefined>;

export type EasypayConfig = {
  pid: string;
  key: string;
  gatewayUrl: string;
  paymentType: string;
  appUrl: string;
  configured: boolean;
};

function getDevDefault(name: "pid" | "key" | "gatewayUrl") {
  if (process.env.NODE_ENV === "production") {
    return "";
  }

  const defaults = {
    pid: "1001",
    key: "dev-only-easypay-key",
    gatewayUrl: "https://example.invalid/submit.php",
  };
  return defaults[name];
}

export function getEasypayConfig(): EasypayConfig {
  const configured = Boolean(
    process.env.EASYPAY_PID && process.env.EASYPAY_KEY && process.env.EASYPAY_GATEWAY_URL,
  );
  const pid = process.env.EASYPAY_PID || getDevDefault("pid");
  const key = process.env.EASYPAY_KEY || getDevDefault("key");
  const gatewayUrl = process.env.EASYPAY_GATEWAY_URL || getDevDefault("gatewayUrl");
  const appUrl = (process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000").replace(/\/$/, "");

  return {
    pid,
    key,
    gatewayUrl,
    paymentType: process.env.EASYPAY_TYPE || "alipay",
    appUrl,
    configured,
  };
}

function normalizeValue(value: EasypayParams[string]) {
  if (value === null || value === undefined) {
    return "";
  }
  return String(value);
}

export function buildEasypaySignString(params: EasypayParams) {
  return Object.entries(params)
    .filter(([key, value]) => key !== "sign" && key !== "sign_type" && normalizeValue(value) !== "")
    .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
    .map(([key, value]) => `${key}=${normalizeValue(value)}`)
    .join("&");
}

export function signEasypayParams(params: EasypayParams, key: string) {
  return createHash("md5")
    .update(`${buildEasypaySignString(params)}${key}`, "utf8")
    .digest("hex");
}

export function verifyEasypaySignature(params: EasypayParams, key: string) {
  const receivedSign = normalizeValue(params.sign).toLowerCase();
  if (!receivedSign) {
    return false;
  }

  const expectedSign = signEasypayParams(params, key).toLowerCase();
  const received = Buffer.from(receivedSign);
  const expected = Buffer.from(expectedSign);
  return received.length === expected.length && timingSafeEqual(received, expected);
}

export function formatMoney(value: unknown) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount < 0) {
    return null;
  }
  return amount.toFixed(2);
}

export function buildEasypayPaymentParams(input: {
  orderId: string;
  name: string;
  money: string;
  config?: EasypayConfig;
  paymentType?: string;
}): EasypayParams & { sign: string; sign_type: "MD5" } {
  const config = input.config ?? getEasypayConfig();
  const params: EasypayParams = {
    pid: config.pid,
    type: input.paymentType || config.paymentType,
    out_trade_no: input.orderId,
    notify_url: `${config.appUrl}/api/payments/easypay/notify`,
    return_url: `${config.appUrl}/api/payments/easypay/return`,
    name: input.name,
    money: input.money,
  };

  const sign = signEasypayParams(params, config.key);
  return {
    ...params,
    sign,
    sign_type: "MD5",
  };
}

export function buildEasypayPaymentUrl(params: EasypayParams, gatewayUrl: string) {
  if (!gatewayUrl) {
    return null;
  }

  const url = new URL(gatewayUrl);
  for (const [key, value] of Object.entries(params)) {
    const normalized = normalizeValue(value);
    if (normalized) {
      url.searchParams.set(key, normalized);
    }
  }
  return url.toString();
}
