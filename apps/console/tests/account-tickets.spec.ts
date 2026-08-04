import { expect, test } from "@playwright/test";

const ticketID = "88888888-8888-4888-8888-888888888888";
const baseTicket = {
  id: ticketID,
  reference: "HKT-88888888-8888-4888-8888-888888888888",
  title: "练习记录问题",
  category: "practice",
  status: "open",
  version: 1,
  created_at: "2026-07-28T00:00:00Z",
  updated_at: "2026-07-28T00:00:00Z",
};

const session = {
  user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
  access_context: {
    permissions: ["account.tickets.read", "account.tickets.reply", "account.tickets.transition"],
    scopes: [{ kind: "product", product_code: "account-portfolio" }],
    verified_at: "2026-07-28T00:00:00Z",
  },
  expires_at: "2026-07-28T01:00:00Z",
};

test.beforeEach(async ({ page }) => {
  let version = 1;
  let status = "open";
  let replied = false;
  const ticket = () => ({ ...baseTicket, version, status, updated_at: `2026-07-28T00:0${version}:00Z` });
  const detail = () => ({
    ticket: ticket(),
    messages: [
      {
        id: "99999999-9999-4999-8999-999999999999",
        author_kind: "user",
        body: "请帮我核对本次练习记录。",
        created_at: "2026-07-28T00:00:00Z",
      },
      ...(replied
        ? [
            {
              id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
              author_kind: "operator",
              body: "我们已核对并正在处理。",
              created_at: "2026-07-28T00:01:00Z",
            },
          ]
        : []),
    ],
    events: [],
  });

  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_account_session" }) })
  );
  await page.route("**/api/v1/account/tickets", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { tickets: [ticket()] }, request_id: "req_account_queue" }) })
  );
  await page.route(`**/api/v1/account/tickets/${ticketID}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: detail(), request_id: "req_account_detail" }) })
  );
  await page.route(`**/api/v1/account/tickets/${ticketID}/replies`, async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_reply_/);
    expect(await route.request().postDataJSON()).toEqual({ body: "我们已核对并正在处理。", expected_version: 1 });
    replied = true;
    version = 2;
    status = "in_progress";
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { ticket: ticket() }, request_id: "req_account_reply" }) });
  });
  await page.route(`**/api/v1/account/tickets/${ticketID}/transitions`, async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_transition_/);
    expect(await route.request().postDataJSON()).toEqual({ status: "resolved", expected_version: 2 });
    version = 3;
    status = "resolved";
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: { ticket: ticket() }, request_id: "req_account_transition" }) });
  });
});

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} Account Portfolio operations reply and transition a durable ticket`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/account");
    await expect(page.getByRole("heading", { name: "账户工单运营" })).toBeVisible();
    await page.getByRole("button", { name: "练习记录问题" }).click();
    await expect(page.getByText("请帮我核对本次练习记录。", { exact: true })).toBeVisible();
    await page.getByLabel("客服回复").fill("我们已核对并正在处理。");
    await page.getByRole("button", { name: "发送回复" }).click();
    await expect(page.getByRole("status")).toContainText("回复已写入工单");
    await expect(page.getByText("我们已核对并正在处理。", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "标记已解决" }).click();
    await expect(page.getByRole("status")).toContainText("工单已标记为已解决");
    await expect(page.locator('[data-account-ticket-detail-state="ready"]').getByText("已解决", { exact: true })).toBeVisible();
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

// The success banner used to be the constant "工单已标记为已解决。" for BOTH
// transitions, so starting work on a ticket told the operator it was resolved
// — they stop working it and the queue silently accumulates. The existing test
// above only ever clicks 标记已解决, where that constant happens to be correct,
// so this path was never covered. 开始处理 only renders while the ticket is
// still open, so this test must not reply first.
test("starting work on a ticket reports that it started, not that it was resolved", async ({ page }) => {
  await page.route(`**/api/v1/account/tickets/${ticketID}/transitions`, async (route) => {
    expect(await route.request().postDataJSON()).toEqual({ status: "in_progress", expected_version: 1 });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: { ticket: { ...baseTicket, status: "in_progress", version: 2, updated_at: "2026-07-28T00:02:00Z" } },
        request_id: "req_account_transition_started",
      }),
    });
  });

  await page.goto("/account");
  await page.getByRole("button", { name: "练习记录问题" }).click();
  await page.getByRole("button", { name: "开始处理" }).click();

  const banner = page.getByRole("status");
  await expect(banner).toContainText("工单已开始处理");
  await expect(banner).not.toContainText("已解决");
});
