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

test("unimplemented Account Portfolio pages still fail closed rather than exposing session mocks", async ({ page }) => {
  await mockSession(page);
  const pages = [
    { path: "/account/posts", title: "我的文章", absentAction: "去发布" },
    { path: "/account/deals", title: "我的交易", absentAction: "完整管理" },
  ];

  for (const item of pages) {
    await page.goto(item.path, { waitUntil: "domcontentloaded" });
    await expect(page.locator('[data-account-capability-state="unavailable"]')).toBeVisible();
    await expect(page.getByRole("heading", { name: item.title })).toBeVisible();
    await expect(page.getByText("不会展示或修改任何会话内数据")).toBeVisible();
    await expect(page.getByText(item.absentAction, { exact: false })).toHaveCount(0);
  }
});

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} wallet renders the real paged immutable ledger without point consumption features`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockSession(page);
    const nextCursor = "plc1.b9Nl4wX2vJm9_0DK-cW1H3s9pQm8aXoZr2LtE5yYv7g";
    let initialRequests = 0;
    let pageRequests = 0;
    await page.route("**/api/v1/account/points**", async (route) => {
      const url = new URL(route.request().url());
      expect(url.searchParams.get("limit")).toBe("20");
      const cursor = url.searchParams.get("cursor");
      if (cursor === null) initialRequests += 1;
      else {
        expect(cursor).toBe(nextCursor);
        pageRequests += 1;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: cursor === null
            ? {
                balance: 100,
                entries: [{ id: "11111111-1111-4111-8111-111111111111", amount: 120, reason: "人工核验后的补偿积分", created_at: "2026-07-28T00:00:00Z" }],
                next_cursor: nextCursor,
              }
            : {
                balance: 100,
                entries: [{ id: "22222222-2222-4222-8222-222222222222", amount: -20, reason: "重复记账复核扣减", created_at: "2026-07-27T00:00:00Z" }],
                next_cursor: null,
              },
          request_id: `req_wallet_${initialRequests + pageRequests}`,
        }),
      });
    });

    await page.goto("/account/wallet", { waitUntil: "domcontentloaded" });
    await expect(page.locator('[data-account-points-state="success"]')).toBeVisible();
    await expect(page.getByRole("heading", { name: "积分钱包" })).toBeVisible();
    await expect(page.locator('[data-account-points-state="success"]')).toContainText("100");
    await expect(page.getByText("人工核验后的补偿积分")).toBeVisible();
    await page.getByRole("button", { name: "加载更多记录" }).click();
    await expect(page.getByText("重复记账复核扣减")).toBeVisible();
    await expect(page.getByRole("button", { name: "加载更多记录" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /签到|购买|消费/ })).toHaveCount(0);
    expect(initialRequests).toBeGreaterThanOrEqual(1);
    expect(pageRequests).toBe(1);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("wallet exposes a recoverable owner failure instead of a local balance", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/points**", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "account_portfolio_unavailable", request_id: "req_wallet_down" }),
    });
  });

  await page.goto("/account/wallet", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-points-state="error"]')).toBeVisible();
  await expect(page.getByText("账户服务不可用时，不会以本地余额或会话数据替代真实积分账本。")).toBeVisible();
  await expect(page.getByText("当前积分余额")).toHaveCount(0);
});

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} membership page renders the durable entitlement without a payment or session fallback`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockSession(page);
    await page.route("**/api/v1/account/membership", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: { plan: "lifetime", lifetime: true },
          request_id: "req_membership_lifetime",
        }),
      });
    });

    await page.goto("/account/membership", { waitUntil: "domcontentloaded" });
    await expect(page.locator('[data-account-membership-state="success"]')).toBeVisible();
    await expect(page.getByRole("heading", { name: "终身会员" })).toBeVisible();
    await expect(page.getByText("权益已由 Account Portfolio 持久化确认，可跨设备读取。", { exact: true })).toBeVisible();
    await expect(page.getByText("支付 Provider 尚未启用", { exact: false })).toBeVisible();
    await expect(page.getByRole("button", { name: /开通|支付/ })).toHaveCount(0);
    await expect(page.getByText(sessionUserID, { exact: true })).toHaveCount(0);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("account tickets create and show durable ticket history without a session fallback", async ({ page }) => {
  await mockSession(page);
  const ticketID = "33333333-3333-4333-8333-333333333333";
  const ticket = {
    id: ticketID,
    reference: "HKT-33333333-3333-4333-8333-333333333333",
    title: "练习记录问题",
    category: "practice",
    status: "open",
    version: 1,
    created_at: "2030-01-01T00:00:00Z",
    updated_at: "2030-01-01T00:00:00Z",
  };
  let createCount = 0;
  await page.route("**/api/v1/account/tickets", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { tickets: [] }, request_id: "req_tickets_empty" }),
      });
      return;
    }
    createCount += 1;
    expect(route.request().headers()["idempotency-key"]).toMatch(/^portal-ticket:/);
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({ data: { ticket }, request_id: "req_ticket_create" }),
    });
  });
  await page.route(`**/api/v1/account/tickets/${ticketID}`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          ticket,
          messages: [
            {
              id: "44444444-4444-4444-8444-444444444444",
              author_kind: "user",
              body: "请帮我核对这次作答。",
              created_at: "2030-01-01T00:00:00Z",
            },
          ],
          events: [],
        },
        request_id: "req_ticket_detail",
      }),
    });
  });

  await page.goto("/account/tickets", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-tickets-state="success"]')).toBeVisible();
  await expect(page.locator("[data-account-tickets-empty]")).toBeVisible();
  await page.getByRole("button", { name: "新建工单" }).click();
  await page.getByLabel("标题").fill("练习记录问题");
  await page.getByLabel("问题说明").fill("请帮我核对这次作答。");
  await page.getByRole("button", { name: "提交工单" }).click();
  await expect(page.locator('[data-account-ticket-detail-state="success"]')).toBeVisible();
  const detail = page.locator('[data-account-ticket-detail-state="success"]');
  await expect(detail.getByText("HKT-33333333-3333-4333-8333-333333333333")).toBeVisible();
  await expect(detail.getByText("请帮我核对这次作答。")).toBeVisible();
  expect(createCount).toBe(1);
});

test("notifications render and mark the server-returned notification as read at a narrow width", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockSession(page);
  const notificationID = "55555555-5555-4555-8555-555555555555";
  const notification = {
    id: notificationID,
    title: "工单有新回复",
    body: "客服已回复你的问题。",
    kind: "ticket_operator_reply",
    ticket_id: "33333333-3333-4333-8333-333333333333",
    ticket_reference: "HKT-33333333-3333-4333-8333-333333333333",
    created_at: "2030-01-01T00:00:00Z",
  };
  let markReadCount = 0;
  await page.route("**/api/v1/account/notifications", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { notifications: [notification] }, request_id: "req_notifications" }),
    });
  });
  await page.route(`**/api/v1/account/notifications/${notificationID}/read`, async (route) => {
    markReadCount += 1;
    expect(route.request().headers()["idempotency-key"]).toMatch(/^portal-notification:/);
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          notification: { ...notification, read_at: "2030-01-01T00:05:00Z" },
        },
        request_id: "req_notification_read",
      }),
    });
  });

  await page.goto("/account/notifications", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-notifications-state="success"]')).toBeVisible();
  await expect(page.getByRole("button", { name: "标为已读" })).toBeVisible();
  await page.getByRole("button", { name: "标为已读" }).click();
  await expect(page.getByText("已读", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "标为已读" })).toHaveCount(0);
  expect(markReadCount).toBe(1);
});

test("paid Library materials never use Account session mocks as a purchase or shelf result", async ({ page }) => {
  await page.goto("/library/item/paid-math-exam25", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-library-purchase-state="unavailable"]')).toBeVisible();
  await expect(page.locator('[data-library-favorite-state="unavailable"]')).toBeVisible();
  await expect(page.getByRole("button", { name: /积分购买|登录后购买/ })).toHaveCount(0);

  await page.goto("/library/read/paid-math-exam25", { waitUntil: "domcontentloaded" });
  const nextPage = page.getByRole("button", { name: "下一页 →" });
  await expect(nextPage).toBeEnabled();
  await nextPage.click();
  await nextPage.click();
  await expect(page.getByText("3 / 8", { exact: true })).toBeVisible();
  await expect(page.locator('[data-library-purchase-state="unavailable"]')).toBeVisible();
  await expect(page.getByText("不会通过本地余额或会话状态解锁全文")).toBeVisible();
  await expect(page.getByRole("button", { name: /积分购买|登录后购买/ })).toHaveCount(0);

  await page.goto("/library/shelf", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-library-shelf-state="unavailable"]')).toBeVisible();
  await expect(page.getByText("不会展示任何个人书架内容")).toBeVisible();
});
