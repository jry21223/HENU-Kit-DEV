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

const sessionUserID = "11111111-1111-4111-8111-111111111111";

test("account overview renders the real zero state and never exposes UID as a label", async ({ page }) => {
  await mockSession(page);
  let releaseSummary!: () => void;
  const summaryReleased = new Promise<void>((resolve) => {
    releaseSummary = resolve;
  });
  let markSummaryStarted!: () => void;
  const summaryStarted = new Promise<void>((resolve) => {
    markSummaryStarted = resolve;
  });
  await page.route("**/api/v1/account/summary", async (route) => {
    markSummaryStarted();
    await summaryReleased;
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
  await summaryStarted;
  await expect(page.locator('[data-account-summary-state="loading"]')).toBeVisible();
  releaseSummary();
  await expect(page.getByRole("heading", { name: "小河同学" })).toBeVisible();
  await expect(page.locator('[data-account-summary-state="success"]')).toBeVisible();
  await expect(page.getByText("暂无通知和进行中工单")).toBeVisible();
  await expect(page.getByText(/UID/)).toHaveCount(0);
  await expect(page.getByText(sessionUserID, { exact: true })).toHaveCount(0);
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

test("unimplemented Account Portfolio pages fail closed rather than exposing session mocks", async ({ page }) => {
  await mockSession(page);
  const pages = [
    { path: "/account/wallet", title: "积分钱包", absentAction: "每日签到" },
    { path: "/account/membership", title: "会员", absentAction: "开通" },
    { path: "/account/notifications", title: "系统通知", absentAction: "全部已读" },
    { path: "/account/tickets", title: "工单", absentAction: "新建工单" },
  ];

  for (const item of pages) {
    await page.goto(item.path, { waitUntil: "domcontentloaded" });
    await expect(page.locator('[data-account-capability-state="unavailable"]')).toBeVisible();
    await expect(page.getByRole("heading", { name: item.title })).toBeVisible();
    await expect(page.getByText("不会展示或修改任何会话内数据")).toBeVisible();
    await expect(page.getByText(item.absentAction, { exact: false })).toHaveCount(0);
  }
});
