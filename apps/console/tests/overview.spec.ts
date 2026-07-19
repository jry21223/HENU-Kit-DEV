import { expect, test } from "@playwright/test";

const moduleNames = ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"];

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
          access_context: { permissions: ["console.overview.read"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
          expires_at: "2026-07-19T00:05:00Z",
        },
        request_id: "req_browser_console",
      }),
    });
  });
});

test("desktop overview exposes six modules, degradation states, and chart alternative", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "产品运行概览" })).toBeVisible();
  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  for (const name of moduleNames) await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  for (const state of ["empty", "partial", "stale", "unavailable", "denied"]) {
    await expect(page.locator(`[data-state="${state}"]`)).toHaveCount(1);
  }
  await page.getByRole("button", { name: "查看表格数据" }).click();
  await expect(page.getByRole("table", { name: "Portal 探针成功次数表格" })).toBeVisible();
  await expect(page.getByText("积分", { exact: true })).toHaveCount(0);
  await expect(page.getByText("会员", { exact: true })).toHaveCount(0);
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
});

test("390px overview keeps every module and mobile navigation usable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  await page.getByRole("button", { name: "打开产品导航" }).click();
  const navigation = page.getByRole("navigation", { name: "移动端产品模块" });
  await expect(navigation.getByRole("link")).toHaveCount(6);
  for (const name of moduleNames) await expect(navigation.getByRole("link", { name })).toBeVisible();

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
});

test("loading scenario marks all six modules busy without fake metrics", async ({ page }) => {
  await page.goto("/?scenario=loading");
  await expect(page.locator("section[aria-busy='true']")).toBeVisible();
  await expect(page.locator("[data-state='loading']")).toHaveCount(6);
  await expect(page.locator(".metric-tile")).toHaveCount(0);
});

test("expired session offers reauthentication and preserves the intended path", async ({ page }) => {
  await page.unroute("**/api/v1/session");
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, contentType: "application/json", body: "{}" }));
  await page.goto("/operations?tab=inbox");

  const login = page.getByRole("link", { name: "登录 Console" });
  await expect(login).toBeVisible();
  await expect(login).toHaveAttribute("href", /return_to=%2Foperations%3Ftab%3Dinbox/);
  await expect(page.locator("[data-state='denied']")).toHaveCount(6);
  await expect(page.locator(".metric-tile")).toHaveCount(0);
});
