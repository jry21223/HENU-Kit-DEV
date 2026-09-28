/**
 * 服务端错误提示的放行规则（#554）。
 *
 * Portal Gateway 的每个错误信封都带一句写给用户的中文 message。下面列出的是 Gateway
 * 自己的错误码，以及它在 /api/v1/account/qq-binding/* 上按 qq-binding 契约原样转发的
 * Platform Core 绑定错误码：命中时原样展示 message。上游透传、不认识的码一律用 Portal
 * 自己的中文兜底。gateway-errors.test.ts 会核对这份名单与 Gateway 源码是否一致。
 */
export const GATEWAY_USER_MESSAGE_CODES: ReadonlySet<string> = new Set([
  // 资料库
  "LIBRARY_TEMPORARILY_UNAVAILABLE",
  "MATERIAL_NOT_AVAILABLE",
  "DOWNLOAD_TEMPORARILY_UNAVAILABLE",
  "INVALID_REQUEST",
  // 刷题
  "practice access denied",
  "practice authorization is temporarily unavailable",
  "practice favorites are not enabled",
  "practice favorites are temporarily unavailable",
  "practice feedback status is not enabled",
  "practice feedback status is temporarily unavailable",
  "practice statistics are not enabled",
  "practice statistics are temporarily unavailable",
  "practice_command_conflict",
  "practice_command_invalid",
  "practice_command_invalid_response",
  "practice_commands_unavailable",
  "practice_idempotency_key_invalid",
  "practice_session_forbidden",
  "practice_session_not_found",
  "quizcraft_catalog_invalid_response",
  "quizcraft_catalog_unavailable",
  "quizcraft_ranking_invalid_response",
  "quizcraft_ranking_unavailable",
  "invalid_bank_id",
  "invalid_ranking_period",
  // 美食
  "food_post_body_too_large",
  "food_post_idempotency_key_invalid",
  "food_post_invalid",
  "food_posts_unavailable",
  "food_service_unavailable",
  // 账户与会员
  "account_command_conflict",
  "account_command_invalid",
  "account_idempotency_key_invalid",
  "account_portfolio_invalid_response",
  "account_portfolio_unavailable",
  "account_resource_not_found",
  "membership_payment_unavailable",
  "membership_unavailable",
  // 求职雷达
  "career_body_too_large",
  "career_idempotency_key_invalid",
  "career_invalid",
  "career_service_unavailable",
  "career_unavailable",
  "lifetime_required",
  // 登录与通用
  "STATE_UNAVAILABLE",
  "session encode error",
  "not found",
  "portal_api_unavailable",
  "proxy_error",
  // QQ 绑定：Gateway 自己的码
  "LOGIN_REQUIRED",
  "ORIGIN_REJECTED",
  "BINDING_UNAVAILABLE",
  // QQ 绑定：Gateway 转发的 Platform Core 绑定错误
  "LINK_EXPIRED",
  "LINK_NOT_FOUND",
  "LINK_ALREADY_AUTHORIZED",
  "INVALID_LINK",
  "ALREADY_BOUND",
  "NOT_BOUND",
  "WEB_APPROVAL_REQUIRED",
  "APPROVAL_EXPIRED",
  "RATE_LIMITED",
  "NOT_FOUND",
]);

/** Gateway 写出、但有意不原样展示的码，以及原因。 */
export const GATEWAY_WITHHELD_CODES: Readonly<Record<string, string>> = {
  "not authenticated": "Gateway 对从没登录过的访客也说“登录已过期”；401 用 Portal 自己的登录提示。",
  CLIENT_AUTH_FAILED: "HENU Bot 与 Platform Core 之间的服务认证失败，用户做不了什么；按绑定服务不可用处理。",
  BINDING_FORBIDDEN: "绑定页把它当作登录失效，引导重新登录，不展示“此操作不可用”。",
};

const CHINESE = /[一-鿿]/;

/** 放行名单里的码、且 message 确实是中文提示时，返回去掉首尾空白的 message；否则 null。 */
export function gatewayUserMessage(code: unknown, message: unknown): string | null {
  if (typeof code !== "string" || typeof message !== "string") return null;
  if (!GATEWAY_USER_MESSAGE_CODES.has(code)) return null;
  const text = message.trim();
  return CHINESE.test(text) ? text : null;
}

/**
 * 同一规则用在自己解析响应体的页面（/bind/qq）：Gateway 的扁平信封是
 * {error: "码", message: "中文"}，它转发的 Platform Core 信封是 {error: {code, message}}。
 */
export function envelopeUserMessage(envelope: unknown): string | null {
  if (typeof envelope !== "object" || envelope === null) return null;
  const { error, message } = envelope as { error?: unknown; message?: unknown };
  if (typeof error === "string") return gatewayUserMessage(error, message);
  if (typeof error === "object" && error !== null) {
    const nested = error as { code?: unknown; message?: unknown };
    return gatewayUserMessage(nested.code, nested.message);
  }
  return null;
}
