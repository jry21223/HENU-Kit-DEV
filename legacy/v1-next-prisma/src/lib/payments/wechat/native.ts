import {
  assertWechatPayConfig,
  getWechatPayConfig,
  type WechatPayConfig,
} from "@/lib/payments/wechat/config";

export type WechatNativePaymentInput = {
  orderId: string;
  outTradeNo: string;
  description: string;
  amountTotal: number;
};

export type WechatNativePaymentResult = {
  codeUrl: string;
  expiresAt: Date;
  mode: WechatPayConfig["mode"];
};

export class WechatPayLiveNotImplementedError extends Error {
  constructor() {
    super("WeChat Pay live Native API is not implemented in this round");
    this.name = "WechatPayLiveNotImplementedError";
  }
}

export function buildMockWechatNativeCodeUrl(input: Pick<WechatNativePaymentInput, "outTradeNo">) {
  const payload = Buffer.from(`mock:${input.outTradeNo}`, "utf8").toString("base64url");
  return `weixin://wxpay/bizpayurl?pr=${payload}`;
}

export async function createWechatNativePayment(
  input: WechatNativePaymentInput,
  config = getWechatPayConfig(),
): Promise<WechatNativePaymentResult> {
  assertWechatPayConfig(config);

  if (!Number.isInteger(input.amountTotal) || input.amountTotal <= 0) {
    throw new Error("amountTotal must be a positive integer in cents");
  }

  if (config.mode === "live") {
    throw new WechatPayLiveNotImplementedError();
  }

  return {
    codeUrl: buildMockWechatNativeCodeUrl(input),
    expiresAt: new Date(Date.now() + config.nativeExpireMinutes * 60 * 1000),
    mode: "mock",
  };
}
