import type { Page } from "@playwright/test";

/**
 * 共享的网关 mock（#553）。各 spec 先装访客网关，再 route 自己要给内容的接口：
 * Playwright 按注册的倒序匹配，后注册的优先。
 */

/** 登录后的会话：账户中心、发布页等要登录的页面用它。 */
export const SIGNED_IN_SESSION = {
  user_id: "11111111-1111-4111-8111-111111111111",
  display_name: "小河同学",
  expires_at: "2030-01-01T00:00:00Z",
};

/**
 * 未登录访客：/api/v1/** 一律 503（上游透传的嵌套信封，带请求编号），/api/v1/session 回 401。
 * 页面落在各自的出错或未登录状态。
 */
export async function mockGuestGateway(page: Page, requestId = "req_gateway_unavailable") {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: requestId },
    })
  );
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
}

/** 已登录，其余接口一律不可用。 */
export async function mockSignedInGateway(page: Page, session = SIGNED_IN_SESSION) {
  await mockGuestGateway(page);
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: session }));
}
