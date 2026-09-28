/**
 * 服务端错误提示的放行规则（#554）。
 *
 * Portal Gateway 自己写出的错误信封是扁平的 {error: "码", message: "中文"}，message 是写给用户的。
 * 下面名单里的码原样展示 message；上游透传的信封（Food、Career 的 {error: {code, message}}）和
 * 不认识的码一律用 Portal 自己的中文兜底。gateway-errors.test.ts 核对 Gateway 源码里以字面量
 * 写出的每个码，要么在名单里，要么在 GATEWAY_WITHHELD_CODES 写明不放行的原因。
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
  "food_post_invalid",
  "food_posts_unavailable",
  "food_service_unavailable",
  // 账户与会员
  "account_command_conflict",
  "account_command_invalid",
  "account_portfolio_invalid_response",
  "account_portfolio_unavailable",
  "membership_payment_unavailable",
  "membership_unavailable",
  // 求职雷达
  "career_body_too_large",
  "career_invalid",
  "career_service_unavailable",
  "career_unavailable",
  "lifetime_required",
  // 通用与 QQ 绑定页
  "portal_api_unavailable",
  "proxy_error",
  "LOGIN_REQUIRED",
  "ORIGIN_REJECTED",
  "BINDING_UNAVAILABLE",
]);

const OAUTH_NAVIGATION = "登录跳转和 OAuth 回调是浏览器整页导航，Portal 不解析这些响应。";
const PORTAL_IDEMPOTENCY_KEY = "幂等键由 Portal 生成，用户无从检查；按服务不可用处理。";
const GENERIC_NOT_FOUND = "通用的“不存在”，多用于只说“内容不存在或已下架”、没有下一步的地方；用 Portal 的 404 提示。";

/** Gateway 写出、但有意不原样展示的码，以及原因。 */
export const GATEWAY_WITHHELD_CODES: Readonly<Record<string, string>> = {
  "not authenticated": "Gateway 对从没登录过的访客也说“登录已过期”；401 用 Portal 自己的登录提示。",
  "not found": GENERIC_NOT_FOUND,
  account_resource_not_found: GENERIC_NOT_FOUND,
  account_idempotency_key_invalid: PORTAL_IDEMPOTENCY_KEY,
  career_idempotency_key_invalid: PORTAL_IDEMPOTENCY_KEY,
  food_post_idempotency_key_invalid: PORTAL_IDEMPOTENCY_KEY,
  practice_idempotency_key_invalid: PORTAL_IDEMPOTENCY_KEY,
  STATE_UNAVAILABLE: OAUTH_NAVIGATION,
  "session encode error": OAUTH_NAVIGATION,
  exchange_error: OAUTH_NAVIGATION,
  "exchange failed": OAUTH_NAVIGATION,
};

/**
 * 只用在 /bind/qq：Gateway 在 /api/v1/account/qq-binding/{authorize,status,unlink} 上按 qq-binding 契约
 * 原样转发 Platform Core 的嵌套信封 {error: {code, message}}，这些绑定错误码的 message 原样展示。
 * 别的路由上的嵌套信封都是上游透传，不走这份名单。
 */
export const BINDING_USER_MESSAGE_CODES: ReadonlySet<string> = new Set([
  "INVALID_REQUEST",
  "INVALID_LINK",
  "LINK_NOT_FOUND",
  "LINK_EXPIRED",
  "LINK_ALREADY_AUTHORIZED",
  "ALREADY_BOUND",
  "RATE_LIMITED",
]);

const BOT_ONLY = "只在 HENU Bot 调用的动作上返回；Gateway 只转发网页的 authorize、status、unlink。";

/** Platform Core 的绑定错误码里有意不展示的，以及原因。 */
export const BINDING_WITHHELD_CODES: Readonly<Record<string, string>> = {
  NOT_BOUND: BOT_ONLY,
  NOT_FOUND: BOT_ONLY,
  WEB_APPROVAL_REQUIRED: BOT_ONLY,
  APPROVAL_EXPIRED: BOT_ONLY,
  CLIENT_AUTH_FAILED: "Platform Core 拒绝了调用方的服务身份；Gateway 不转发 401，改回 503 BINDING_UNAVAILABLE。",
  BINDING_FORBIDDEN: "绑定页把它当作登录失效，引导重新登录，不展示“此操作不可用”。",
};

const CHINESE = /[一-鿿]/;

function chineseMessage(codes: ReadonlySet<string>, code: unknown, message: unknown): string | null {
  if (typeof code !== "string" || typeof message !== "string") return null;
  if (!codes.has(code)) return null;
  const text = message.trim();
  return CHINESE.test(text) ? text : null;
}

/** Gateway 扁平信封的码在名单里、且 message 确实是中文提示时，返回去掉首尾空白的 message；否则 null。 */
export function gatewayUserMessage(code: unknown, message: unknown): string | null {
  return chineseMessage(GATEWAY_USER_MESSAGE_CODES, code, message);
}

/**
 * /bind/qq 自己解析响应体，用同一规则：扁平信封是 Gateway 自己的错误，嵌套信封是它在绑定接口上
 * 转发的 Platform Core 绑定错误。
 */
export function envelopeUserMessage(envelope: unknown): string | null {
  if (typeof envelope !== "object" || envelope === null) return null;
  const { error, message } = envelope as { error?: unknown; message?: unknown };
  if (typeof error === "string") return gatewayUserMessage(error, message);
  if (typeof error === "object" && error !== null) {
    const nested = error as { code?: unknown; message?: unknown };
    return chineseMessage(BINDING_USER_MESSAGE_CODES, nested.code, nested.message);
  }
  return null;
}
