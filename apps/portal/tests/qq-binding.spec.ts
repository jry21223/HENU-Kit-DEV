import { expect, test } from "@playwright/test";

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

test("invalid links cannot authorize", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
  await page.goto("/bind/qq#invalid");
  await expect(page.getByText("请在 QQ 私聊 HENU Bot 发送“绑定 HENU KIT”，获取新的绑定链接。")).toBeVisible();
  await expect(page.getByRole("button", { name: "授权绑定当前账号" })).toHaveCount(0);
});
