import { expect, test, type Page } from "@playwright/test";
import { expectTouchTargets } from "./support/touch-targets";

test("binding requires an explicit website approval and sends no caller identity", async ({ page }) => {
  const token = "a".repeat(43);
  let approvals = 0;
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: { user_id: "11111111-1111-4111-8111-111111111111", display_name: "小河" } }));
  await page.route("**/api/v1/account/qq-binding/status", (route) => route.fulfill({ json: { data: { bound: false } } }));
  await page.route("**/api/v1/account/qq-binding/authorize", async (route) => {
    approvals++;
    expect(route.request().postDataJSON()).toEqual({ token });
    await route.fulfill({ json: { data: { state: "authorized" } } });
  });
  await page.goto(`/bind/qq#${token}`);
  await expect(page.getByRole("heading", { name: "绑定 HENU Bot" })).toBeVisible();
  await expect(page.locator("#henukit-langbot-widget")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "授权绑定当前账号" })).toBeEnabled();
  expect(approvals).toBe(0);
  await page.getByRole("button", { name: "授权绑定当前账号" }).click();
  await expect(page.getByRole("status")).toContainText("回到原 QQ 私聊");
  expect(approvals).toBe(1);
});

test("expired session offers login instead of repeating authorization", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: { user_id: "11111111-1111-4111-8111-111111111111", display_name: "小河" } }));
  await page.route("**/api/v1/account/qq-binding/status", (route) => route.fulfill({ json: { data: { bound: false } } }));
  await page.route("**/api/v1/account/qq-binding/authorize", (route) => route.fulfill({ status: 403, json: { error: { code: "BINDING_FORBIDDEN" } } }));
  await page.goto(`/bind/qq#${"b".repeat(43)}`);
  await page.getByRole("button", { name: "授权绑定当前账号" }).click();
  await expect(page.getByRole("link", { name: "登录 HENU KIT" })).toBeVisible();
  await expect(page.getByRole("button", { name: "授权绑定当前账号" })).toHaveCount(0);
});

/**
 * 网关错误页、WAF 挑战页或断网时，页面只说中文：发生了什么、可以怎么做；浏览器的原始报错
 * （Failed to fetch、JSON 解析失败）不上屏。绑定服务自己返回的中文 message 照常原样展示。
 */
const SIGNED_IN = { user_id: "11111111-1111-4111-8111-111111111111", display_name: "小河" };
const HTML_502 = "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n<center><h1>502 Bad Gateway</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n";
const RAW_ERROR = /Failed to fetch|Load failed|Unexpected|JSON|<html|nginx|Bad Gateway/;
// Next 自带的路由播报也是 role="alert"，只看正文里的提示。
const pageAlert = (page: Page) => page.locator("main").getByRole("alert");

test("an unreachable session read shows Chinese copy instead of the browser's error", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.abort("internetdisconnected"));
  await page.goto(`/bind/qq#${"c".repeat(43)}`);
  await expect(pageAlert(page)).toHaveText("网络连接失败，请检查网络后重试。");
  await expect(page.locator("main")).not.toContainText(RAW_ERROR);
});

test("a session read answered by an HTML page shows Chinese copy", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ contentType: "text/html", body: "<!DOCTYPE html><html><body>verify you are human</body></html>" }));
  await page.goto(`/bind/qq#${"d".repeat(43)}`);
  await expect(pageAlert(page)).toHaveText("登录状态暂时无法读取，请稍后刷新重试。");
  await expect(page.locator("main")).not.toContainText(RAW_ERROR);
});

test("a binding status answered by a gateway error page shows Chinese copy", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: SIGNED_IN }));
  await page.route("**/api/v1/account/qq-binding/status", (route) => route.fulfill({ status: 502, contentType: "text/html", body: HTML_502 }));
  await page.goto(`/bind/qq#${"e".repeat(43)}`);
  await expect(pageAlert(page)).toHaveText("绑定服务暂时不可用，请稍后重试。");
  await expect(page.locator("main")).not.toContainText(RAW_ERROR);
});

test("a failed authorization says what happened and can be tried again", async ({ page }) => {
  let attempt = 0;
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: SIGNED_IN }));
  await page.route("**/api/v1/account/qq-binding/status", (route) => route.fulfill({ json: { data: { bound: false } } }));
  await page.route("**/api/v1/account/qq-binding/authorize", async (route) => {
    attempt++;
    if (attempt === 1) return route.abort("internetdisconnected");
    if (attempt === 2) return route.fulfill({ status: 502, contentType: "text/html", body: HTML_502 });
    return route.fulfill({ json: { data: { state: "authorized" } } });
  });
  await page.goto(`/bind/qq#${"f".repeat(43)}`);
  const authorize = page.getByRole("button", { name: "授权绑定当前账号" });

  await authorize.click();
  await expect(pageAlert(page)).toHaveText("网络连接失败，请检查网络后重试。");
  await authorize.click();
  await expect(pageAlert(page)).toHaveText("绑定服务暂时不可用，请稍后重试。");
  await expect(page.locator("main")).not.toContainText(RAW_ERROR);

  await authorize.click();
  await expect(page.getByRole("status")).toContainText("回到原 QQ 私聊");
  await expect(pageAlert(page)).toHaveCount(0);
  expect(attempt).toBe(3);
});

test("the binding service's own message is shown as it is", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: SIGNED_IN }));
  await page.route("**/api/v1/account/qq-binding/status", (route) => route.fulfill({ json: { data: { bound: false } } }));
  await page.route("**/api/v1/account/qq-binding/authorize", (route) => route.fulfill({ status: 410, json: { error: { code: "LINK_EXPIRED", message: "绑定链接已失效，请重新发起" }, request_id: "req_qq_expired" } }));
  await page.goto(`/bind/qq#${"g".repeat(43)}`);
  await page.getByRole("button", { name: "授权绑定当前账号" }).click();
  await expect(pageAlert(page)).toHaveText("绑定链接已失效，请重新发起");
});

test("invalid links cannot authorize", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
  await page.goto("/bind/qq#invalid");
  await expect(page.getByText("请在 QQ 私聊 HENU Bot 发送“绑定 HENU KIT”，获取新的绑定链接。")).toBeVisible();
  await expect(page.getByRole("button", { name: "授权绑定当前账号" })).toHaveCount(0);
});

test("390px links and buttons on the binding page are at least 44×44 (#543)", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: SIGNED_IN }));
  await page.route("**/api/v1/account/qq-binding/status", (route) => route.fulfill({ json: { data: { bound: true } } }));
  await page.goto("/bind/qq");
  const unlink = page.getByRole("button", { name: "解除 QQ 绑定" });
  await expect(unlink).toBeVisible();
  await expectTouchTargets(page, "/bind/qq (bound)");

  await unlink.click();
  await expect(page.getByRole("button", { name: "确认解绑" })).toBeVisible();
  await expectTouchTargets(page, "/bind/qq (confirm unlink)");
});
