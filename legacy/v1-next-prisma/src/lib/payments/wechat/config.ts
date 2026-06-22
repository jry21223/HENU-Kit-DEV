export type WechatPayMode = "mock" | "live";

export type WechatPayConfig = {
  mode: WechatPayMode;
  nodeEnv: string;
  appId: string;
  mchId: string;
  apiV3Key: string;
  merchantSerialNo: string;
  merchantPrivateKey: string;
  merchantPrivateKeyPath: string;
  platformCertsDir: string;
  notifyUrl: string;
  nativeExpireMinutes: number;
};

export class WechatPayConfigError extends Error {
  errors: string[];

  constructor(errors: string[]) {
    super(errors.join("; "));
    this.name = "WechatPayConfigError";
    this.errors = errors;
  }
}

function readEnv(env: NodeJS.ProcessEnv, key: string) {
  return env[key]?.trim() ?? "";
}

function parseMode(value: string): WechatPayMode {
  return value === "live" ? "live" : "mock";
}

function parseExpireMinutes(value: string) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 15;
}

export function getWechatPayConfig(env: NodeJS.ProcessEnv = process.env): WechatPayConfig {
  return {
    mode: parseMode(readEnv(env, "WECHAT_PAY_MODE")),
    nodeEnv: readEnv(env, "NODE_ENV") || "development",
    appId: readEnv(env, "WECHAT_PAY_APPID"),
    mchId: readEnv(env, "WECHAT_PAY_MCH_ID"),
    apiV3Key: readEnv(env, "WECHAT_PAY_API_V3_KEY"),
    merchantSerialNo: readEnv(env, "WECHAT_PAY_MERCHANT_SERIAL_NO"),
    merchantPrivateKey: readEnv(env, "WECHAT_PAY_MERCHANT_PRIVATE_KEY"),
    merchantPrivateKeyPath: readEnv(env, "WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH"),
    platformCertsDir: readEnv(env, "WECHAT_PAY_PLATFORM_CERTS_DIR"),
    notifyUrl: readEnv(env, "WECHAT_PAY_NOTIFY_URL"),
    nativeExpireMinutes: parseExpireMinutes(readEnv(env, "WECHAT_PAY_NATIVE_EXPIRE_MINUTES")),
  };
}

export function validateWechatPayConfig(config: WechatPayConfig) {
  const errors: string[] = [];

  if (config.nodeEnv === "production" && config.mode === "mock") {
    errors.push("WECHAT_PAY_MODE=mock is not allowed in production");
  }

  if (config.mode === "live") {
    const requiredFields: Array<[keyof WechatPayConfig, string]> = [
      ["appId", "WECHAT_PAY_APPID"],
      ["mchId", "WECHAT_PAY_MCH_ID"],
      ["apiV3Key", "WECHAT_PAY_API_V3_KEY"],
      ["merchantSerialNo", "WECHAT_PAY_MERCHANT_SERIAL_NO"],
      ["platformCertsDir", "WECHAT_PAY_PLATFORM_CERTS_DIR"],
      ["notifyUrl", "WECHAT_PAY_NOTIFY_URL"],
    ];

    for (const [field, envName] of requiredFields) {
      if (!config[field]) {
        errors.push(`${envName} is required in live mode`);
      }
    }

    if (!config.merchantPrivateKey && !config.merchantPrivateKeyPath) {
      errors.push(
        "WECHAT_PAY_MERCHANT_PRIVATE_KEY or WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH is required in live mode",
      );
    }
  }

  return errors;
}

export function assertWechatPayConfig(config: WechatPayConfig) {
  const errors = validateWechatPayConfig(config);
  if (errors.length) {
    throw new WechatPayConfigError(errors);
  }
}
