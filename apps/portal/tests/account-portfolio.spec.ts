import { expect, test, type BrowserContext, type Page } from "@playwright/test";

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

type SharedTicket = {
  id: string;
  reference: string;
  title: string;
  category: string;
  status: "open";
  version: number;
  created_at: string;
  updated_at: string;
};

async function installSharedTicketGateway(
  context: BrowserContext,
  state: { ticket: SharedTicket | null }
) {
  await context.route("**/api/v1/session", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: sessionUserID,
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    });
  });
  await context.route("**/api/v1/account/tickets", async (route) => {
    if (route.request().method() === "POST") {
      const input = await route.request().postDataJSON();
      expect(input).toEqual({
        title: "跨设备持久化验证",
        category: "practice",
        body: "刷新和重新登录后仍应由服务端返回。",
      });
      expect(route.request().headers()["idempotency-key"]).toMatch(/^portal-ticket:/);
      state.ticket = {
        id: "92929292-9292-4929-8929-929292929292",
        reference: "HKT-92929292-9292-4929-8929-929292929292",
        title: input.title,
        category: input.category,
        status: "open",
        version: 1,
        created_at: "2030-01-01T00:00:00Z",
        updated_at: "2030-01-01T00:00:00Z",
      };
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ data: { ticket: state.ticket }, request_id: "req_shared_ticket_create" }),
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: { tickets: state.ticket ? [state.ticket] : [] },
        request_id: "req_shared_ticket_list",
      }),
    });
  });
  await context.route("**/api/v1/account/tickets/*", async (route) => {
    expect(state.ticket).not.toBeNull();
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: { ticket: state.ticket, messages: [], events: [] },
        request_id: "req_shared_ticket_detail",
      }),
    });
  });
}

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

test("account session dependency failure fails closed instead of becoming a local identity", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "portal_session_unavailable", request_id: "req_session_down" }),
    });
  });

  await page.goto("/account", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-session-state="error"]')).toBeVisible();
  await expect(page.getByText("账户信息暂时加载不出来，请稍后重新加载。")).toBeVisible();
  await expect(page.locator('[data-account-summary-state="success"]')).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重新加载" })).toHaveCSS("min-height", "44px");
  await expect(page.getByRole("link", { name: /登录\s*\/\s*注册/ })).toHaveCSS("min-height", "44px");
});

test("an owner 401 requires a fresh Portal login instead of offering a retry loop", async ({ page }) => {
  let ownerUnauthorized = false;
  await page.route("**/api/v1/session", async (route) => {
    await route.fulfill(ownerUnauthorized
      ? {
          status: 401,
          contentType: "application/json",
          body: JSON.stringify({ error: "not_authenticated", request_id: "req_expired_session" }),
        }
      : {
          contentType: "application/json",
          body: JSON.stringify({
            user_id: sessionUserID,
            display_name: "小河同学",
            expires_at: "2030-01-01T00:00:00Z",
          }),
        });
  });
  await page.route("**/api/v1/account/summary", async (route) => {
    ownerUnauthorized = true;
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: "not_authenticated", request_id: "req_owner_unauthorized" }),
    });
  });

  await page.goto("/account", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL(/\/account\/login\?next=%2Faccount/);
  await expect(page.locator('[data-account-summary-state="error"]')).toHaveCount(0);
});

test("all Account Portfolio views expose loading before their Gateway result", async ({ page }) => {
  const views = [
    {
      path: "/account",
      endpoint: "**/api/v1/account/summary",
      state: "data-account-summary-state",
      body: {
        data: {
          points_balance: 0,
          plan: "free",
          lifetime: false,
          unread_notification_count: 0,
          open_ticket_count: 0,
        },
        request_id: "req_loading_summary",
      },
    },
    {
      path: "/account/wallet",
      endpoint: "**/api/v1/account/points**",
      state: "data-account-points-state",
      body: { data: { balance: 0, entries: [], next_cursor: null }, request_id: "req_loading_points" },
    },
    {
      path: "/account/membership",
      endpoint: "**/api/v1/account/membership",
      state: "data-account-membership-state",
      body: { data: { plan: "free", lifetime: false }, request_id: "req_loading_membership" },
    },
    {
      path: "/account/notifications",
      endpoint: "**/api/v1/account/notifications",
      state: "data-account-notifications-state",
      body: { data: { notifications: [] }, request_id: "req_loading_notifications" },
    },
    {
      path: "/account/tickets",
      endpoint: "**/api/v1/account/tickets",
      state: "data-account-tickets-state",
      body: { data: { tickets: [] }, request_id: "req_loading_tickets" },
    },
  ];

  for (const view of views) {
    await page.unrouteAll();
    await mockSession(page);
    let release!: () => void;
    const resultReleased = new Promise<void>((resolve) => {
      release = resolve;
    });
    let markStarted!: () => void;
    const resultStarted = new Promise<void>((resolve) => {
      markStarted = resolve;
    });
    await page.route(view.endpoint, async (route) => {
      markStarted();
      await resultReleased;
      await route.fulfill({ contentType: "application/json", body: JSON.stringify(view.body) });
    });

    await page.goto(view.path, { waitUntil: "domcontentloaded" });
    await resultStarted;
    await expect(page.locator(`[${view.state}="loading"]`)).toBeVisible();
    release();
    await expect(page.locator(`[${view.state}="success"]`)).toBeVisible();
  }
});

test("new account pages render durable zero and empty responses without session fixtures", async ({ page }) => {
  await mockSession(page);
  await page.route("**/api/v1/account/summary", async (route) => {
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
        request_id: "req_zero_summary",
      }),
    });
  });
  await page.route("**/api/v1/account/points**", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: { balance: 0, entries: [], next_cursor: null },
        request_id: "req_zero_points",
      }),
    });
  });
  await page.route("**/api/v1/account/membership", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { plan: "free", lifetime: false }, request_id: "req_free_membership" }),
    });
  });
  await page.route("**/api/v1/account/notifications", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { notifications: [] }, request_id: "req_empty_notifications" }),
    });
  });
  await page.route("**/api/v1/account/tickets", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { tickets: [] }, request_id: "req_empty_tickets" }),
    });
  });

  await page.goto("/account", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-summary-state="success"]')).toContainText("0");
  await expect(page.getByText("暂无通知和进行中工单")).toBeVisible();

  await page.goto("/account/wallet", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-points-state="success"]')).toContainText("0");
  await expect(page.locator("[data-account-points-empty]")).toBeVisible();

  await page.goto("/account/membership", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-membership-state="success"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "免费会员" })).toBeVisible();

  await page.goto("/account/notifications", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-account-notifications-empty]")).toBeVisible();

  await page.goto("/account/tickets", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-account-tickets-empty]")).toBeVisible();
});

test("account owner failures remain recoverable with 44px controls at 360px", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 });
  const pages = [
    { path: "/account", endpoint: "**/api/v1/account/summary", state: "data-account-summary-state" },
    { path: "/account/membership", endpoint: "**/api/v1/account/membership", state: "data-account-membership-state" },
    { path: "/account/notifications", endpoint: "**/api/v1/account/notifications", state: "data-account-notifications-state" },
    { path: "/account/tickets", endpoint: "**/api/v1/account/tickets", state: "data-account-tickets-state" },
  ];

  for (const item of pages) {
    await page.unrouteAll();
    await mockSession(page);
    await page.route(item.endpoint, async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "account_portfolio_unavailable", request_id: `req_${item.state}` }),
      });
    });
    await page.goto(item.path, { waitUntil: "domcontentloaded" });
    await expect(page.locator(`[${item.state}="error"]`)).toBeVisible();
    await expect(page.getByRole("button", { name: "重新加载" })).toHaveCSS("min-height", "44px");
  }

  await expect(page.getByRole("button", { name: "新建工单" })).toHaveCSS("min-height", "44px");
  await expect(page.getByRole("button", { name: "退出登录" })).toHaveCSS("min-height", "44px");
  await expect(page.getByRole("link", { name: "小河同学的账户概览" })).toHaveCSS("min-height", "44px");
});

test("unshipped posts and deals have no account-console entry or placeholder page", async ({ page }) => {
  await mockSession(page);
  await page.goto("/account", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("link", { name: /我的文章|我的交易/ })).toHaveCount(0);

  for (const path of ["/account/posts", "/account/deals"]) {
    const response = await page.goto(path, { waitUntil: "domcontentloaded" });
    expect(response?.status()).toBe(404);
    await expect(page.locator('[data-account-capability-state="unavailable"]')).toHaveCount(0);
  }
});

test("ticket state survives refresh and a fresh browser context through the Gateway", async ({ browser }) => {
  const state: { ticket: SharedTicket | null } = { ticket: null };
  const firstContext = await browser.newContext();
  const secondContext = await browser.newContext();

  try {
    await installSharedTicketGateway(firstContext, state);
    const firstPage = await firstContext.newPage();
    await firstPage.goto("http://127.0.0.1:3001/account/tickets", { waitUntil: "domcontentloaded" });
    await firstPage.getByRole("button", { name: "新建工单" }).click();
    await firstPage.getByLabel("标题").fill("跨设备持久化验证");
    await firstPage.getByLabel("问题说明").fill("刷新和重新登录后仍应由服务端返回。");
    await firstPage.getByRole("button", { name: "提交工单" }).click();
    await expect(firstPage.getByText("HKT-92929292-9292-4929-8929-929292929292").first()).toBeVisible();

    await firstPage.reload({ waitUntil: "domcontentloaded" });
    await expect(firstPage.getByText("跨设备持久化验证").first()).toBeVisible();

    await installSharedTicketGateway(secondContext, state);
    const secondPage = await secondContext.newPage();
    await secondPage.goto("http://127.0.0.1:3001/account/tickets", { waitUntil: "domcontentloaded" });
    await expect(secondPage.getByText("跨设备持久化验证").first()).toBeVisible();
    await expect(secondPage.getByText("HKT-92929292-9292-4929-8929-929292929292")).toBeVisible();
  } finally {
    await Promise.all([firstContext.close(), secondContext.close()]);
  }
});

test("an acknowledged ticket stays visible when the initial list request failed", async ({ page }) => {
  const ticket = {
    id: "93939393-9393-4939-8939-939393939393",
    reference: "HKT-93939393-9393-4939-8939-939393939393",
    title: "列表故障后的持久化工单",
    category: "practice",
    status: "open",
    version: 1,
    created_at: "2030-01-01T00:00:00Z",
    updated_at: "2030-01-01T00:00:00Z",
  };
  await mockSession(page);
  await page.route("**/api/v1/account/tickets", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "account_portfolio_unavailable", request_id: "req_list_down" }),
      });
      return;
    }
    await route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({ data: { ticket }, request_id: "req_ticket_create_after_list_down" }),
    });
  });
  await page.route(`**/api/v1/account/tickets/${ticket.id}`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { ticket, messages: [], events: [] }, request_id: "req_ticket_detail" }),
    });
  });

  await page.goto("/account/tickets", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-account-tickets-state="error"]')).toBeVisible();
  await page.getByRole("button", { name: "新建工单" }).click();
  await page.getByLabel("标题").fill(ticket.title);
  await page.getByLabel("问题说明").fill("创建命令的真实返回必须立即可见。");
  await page.getByRole("button", { name: "提交工单" }).click();

  await expect(page.locator('[data-account-tickets-state="success"]')).toBeVisible();
  await expect(page.getByText(ticket.reference).first()).toBeVisible();
  await expect(page.locator('[data-account-ticket-detail-state="success"]')).toBeVisible();
});

test("a delayed pre-command ticket list cannot erase an acknowledged ticket", async ({ page }) => {
  const ticket = {
    id: "94949494-9494-4949-8949-949494949494",
    reference: "HKT-94949494-9494-4949-8949-949494949494",
    title: "延迟列表并发验证",
    category: "practice",
    status: "open",
    version: 1,
    created_at: "2030-01-01T00:00:00Z",
    updated_at: "2030-01-01T00:00:00Z",
  };
  let releaseList!: () => void;
  const listReleased = new Promise<void>((resolve) => {
    releaseList = resolve;
  });
  let markListStarted!: () => void;
  const listStarted = new Promise<void>((resolve) => {
    markListStarted = resolve;
  });
  let markListFulfilled!: () => void;
  const listFulfilled = new Promise<void>((resolve) => {
    markListFulfilled = resolve;
  });
  await mockSession(page);
  await page.route("**/api/v1/account/tickets", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ data: { ticket }, request_id: "req_ticket_create_concurrent" }),
      });
      return;
    }
    markListStarted();
    await listReleased;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { tickets: [] }, request_id: "req_stale_ticket_list" }),
    });
    markListFulfilled();
  });
  await page.route(`**/api/v1/account/tickets/${ticket.id}`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { ticket, messages: [], events: [] }, request_id: "req_ticket_detail" }),
    });
  });

  await page.goto("/account/tickets", { waitUntil: "domcontentloaded" });
  await listStarted;
  await page.getByRole("button", { name: "新建工单" }).click();
  await page.getByLabel("标题").fill(ticket.title);
  await page.getByLabel("问题说明").fill("旧列表响应不应覆盖已确认的服务端写入。");
  await page.getByRole("button", { name: "提交工单" }).click();
  await expect(page.getByText(ticket.reference).first()).toBeVisible();

  releaseList();
  await listFulfilled;
  await expect(page.getByText(ticket.reference).first()).toBeVisible();
});

test("a 409 follow-up refreshes the version and retries with a new idempotency key", async ({ page }) => {
  const ticketID = "95959595-9595-4959-8959-959595959595";
  let version = 1;
  let commandCount = 0;
  let firstKey = "";
  const ticket = () => ({
    id: ticketID,
    reference: `HKT-${ticketID}`,
    title: "并发追问验证",
    category: "practice",
    status: "open",
    version,
    created_at: "2030-01-01T00:00:00Z",
    updated_at: `2030-01-01T00:00:0${version}Z`,
  });
  await mockSession(page);
  await page.route("**/api/v1/account/tickets", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { tickets: [ticket()] }, request_id: "req_followup_list" }),
    });
  });
  await page.route(`**/api/v1/account/tickets/${ticketID}`, async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { ticket: ticket(), messages: [], events: [] }, request_id: "req_followup_detail" }),
    });
  });
  await page.route(`**/api/v1/account/tickets/${ticketID}/follow-ups`, async (route) => {
    commandCount += 1;
    const input = await route.request().postDataJSON();
    const key = route.request().headers()["idempotency-key"];
    if (commandCount === 1) {
      firstKey = key;
      expect(input).toEqual({ body: "请保留这段补充说明。", expected_version: 1 });
      version = 2;
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({ error: "ticket_version_conflict", request_id: "req_followup_conflict" }),
      });
      return;
    }
    expect(key).not.toBe(firstKey);
    expect(input).toEqual({ body: "请保留这段补充说明。", expected_version: 2 });
    version = 3;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: { ticket: ticket() }, request_id: "req_followup_success" }),
    });
  });

  await page.goto("/account/tickets", { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "并发追问验证" }).click();
  await expect(page.locator('[data-account-ticket-detail-state="success"]')).toBeVisible();
  await page.getByLabel("补充说明").fill("请保留这段补充说明。");
  await page.getByRole("button", { name: "提交补充" }).click();
  await expect(page.getByText("工单刚刚发生更新，已刷新最新记录后再试。")).toBeVisible();
  await expect(page.getByText(/版本 2/)).toBeVisible();
  await page.getByRole("button", { name: "提交补充" }).click();
  await expect.poll(() => commandCount).toBe(2);
  await expect(page.getByText(/版本 3/)).toBeVisible();
});

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
  { name: "360px", width: 360, height: 800 },
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
    const loadMore = page.getByRole("button", { name: "加载更多记录" });
    const walletNavigation = page.getByRole("link", { name: /A-03.*积分钱包/ });
    await expect(loadMore).toHaveCSS("min-height", "44px");
    await expect(walletNavigation).toHaveCSS("min-height", "44px");
    await loadMore.click();
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
  await page.setViewportSize({ width: 360, height: 800 });
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
  await expect(page.getByText("积分数据暂时加载不出来，请稍后重试。")).toBeVisible();
  await expect(page.getByText("当前积分余额")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重新加载" })).toHaveCSS("min-height", "44px");
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
    await expect(page.getByText("权益已由系统确认，可跨设备读取。", { exact: true })).toBeVisible();
    await expect(page.locator("[data-membership-purchase]")).toHaveCount(0);
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
  await expect(page.getByRole("button", { name: "提交工单" })).toHaveCSS("min-height", "44px");
  await expect(page.getByRole("button", { name: "取消" })).toHaveCSS("min-height", "44px");
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
  const markRead = page.getByRole("button", { name: "标为已读" });
  await expect(markRead).toBeVisible();
  await expect(markRead).toHaveCSS("min-height", "44px");
  await markRead.click();
  await expect(page.getByText("已读", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "标为已读" })).toHaveCount(0);
  expect(markReadCount).toBe(1);
});

test("paid Library materials never use Account session mocks as purchase, preview, or shelf results", async ({ page }) => {
  // Detail fetches the owner material by id from the real contract instead
  // of reading bundled static entries. Paid material remains unavailable for
  // purchase and all online-reader routes return to the OSS-download detail
  // boundary; Account session mocks must not manufacture either capability.
  const paidMaterial = {
    id: "paid-math-exam25",
    type: "exam",
    subject: "高等数学A",
    title: "高等数学A · 2025 期末试卷 + 逐题详解",
    author: "刘助教",
    intro: "2025 春季期末真题，每题附完整解题过程与评分点标注。",
    toc: ["一、填空题", "二、计算题", "三、应用题", "四、证明题", "逐题详解"],
    pages: Array.from({ length: 8 }, (_, pageIndex) => [
      `第 ${pageIndex + 1} 页正文段落`,
    ]),
    price: 60,
    previewPages: 2,
    rating: 4.9,
    downloads: 2876,
    favs: 924,
  };
  await page.route("**/api/v1/library/materials/paid-math-exam25", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        material: paidMaterial,
        request_id: "req_library_paid_material",
      }),
    });
  });

  await page.goto("/library/item/paid-math-exam25", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-library-purchase-state="unavailable"]')).toBeVisible();
  await expect(page.locator('[data-library-favorite-state="unavailable"]')).toBeVisible();
  await expect(page.getByRole("button", { name: /积分购买|登录后购买/ })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /立即阅读|免费试读/ })).toHaveCount(0);

  await page.goto("/library/read/paid-math-exam25", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveURL(/\/library\/item\/paid-math-exam25$/);
  await expect(page.locator('[data-library-purchase-state="unavailable"]')).toBeVisible();
  await expect(page.getByRole("button", { name: "下一页 →" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /立即阅读|免费试读/ })).toHaveCount(0);

  await page.goto("/library/shelf", { waitUntil: "domcontentloaded" });
  await expect(page.locator('[data-library-shelf-state="unavailable"]')).toBeVisible();
  await expect(page.getByText("书架功能即将上线，敬请期待。")).toBeVisible();
});
