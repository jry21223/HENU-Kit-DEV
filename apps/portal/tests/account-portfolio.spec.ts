import { expect, test, type Page } from "@playwright/test";

async function mockSession(page: Page) {
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: "11111111-1111-4111-8111-111111111111",
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    });
  });
}

test("account overview renders the real zero state and never exposes UID as a label", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/summary", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 150));
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          points_balance: 0,
          plan: "free",
          lifetime: false,
          unread_notification_count: 0,
          open_ticket_count: 0,
        },
        request_id: "req_account_summary",
      }),
    });
  });

  await page.goto("/account", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-summary-state="loading"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "小河同学" })).toBeVisible();
  await expect(page.locator('[data-account-summary-state="success"]')).toBeVisible();
  await expect(page.getByText("暂无通知和进行中工单")).toBeVisible();
  await expect(page.getByText(/UID/)).toHaveCount(0);
});

test("account overview renders a recoverable error when Account Portfolio is unavailable", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/summary", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "account_portfolio_unavailable", request_id: "req_owner_down" }),
    });
  });

  await page.goto("/account", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-summary-state="error"]')).toBeVisible();
  await expect(page.getByRole("button", { name: "重新加载" })).toBeVisible();
  await expect(page.getByText("账户状态不可用")).toBeVisible();
  await expect(page.getByText("免费会员")).toHaveCount(0);
  await expect(page.getByText(/积分余额/)).toHaveCount(0);
});
