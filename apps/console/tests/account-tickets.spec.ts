import { expect, test } from "@playwright/test";

const ticketID = "88888888-8888-4888-8888-888888888888";
const missingIdentityTicketID = "77777777-7777-4777-8777-777777777777";
const baseTicket = {
  id: ticketID,
  reference: "HKT-88888888-8888-4888-8888-888888888888",
  display_name: "李同学",
  email: "very.long.support.ticket.owner@henu.edu.cn",
  title: "练习记录问题",
  category: "practice",
  status: "open",
  version: 1,
  created_at: "2026-07-28T00:00:00Z",
  updated_at: "2026-07-28T00:00:00Z",
};

type QueueReference = Pick<typeof baseTicket, "id" | "reference" | "title" | "status" | "version" | "updated_at">;
function queueFacts(items: QueueReference[], url: string) {
  const filter = new URL(url).searchParams.get("status") ?? "all";
  const counts = { open: items.filter((item) => item.status === "open").length, in_progress: items.filter((item) => item.status === "in_progress").length, resolved: items.filter((item) => item.status === "resolved").length, unresolved: items.filter((item) => item.status !== "resolved").length };
  const window = items.filter((item) => filter === "all" || (filter === "unresolved" ? item.status !== "resolved" : item.status === filter)).map(({ id, reference, title, status, version, updated_at }) => ({ id, reference, title, status, version, updated_at }));
  return { scope: "global", as_of: "2026-07-28T00:03:00Z", stale: false, counts, window: { filter, limit: 100, matched_total: window.length, returned: window.length, has_more: false, items: window } };
}

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
  await page.route("**/api/v1/account/tickets/facts?*", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: queueFacts([ticket(), { ...ticket(), id: missingIdentityTicketID, reference: `HKT-${missingIdentityTicketID}`, title: "身份资料缺失" }], route.request().url()), request_id: "req_account_queue" }) })
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
  { name: "375px", width: 375, height: 812 },
  { name: "390px", width: 390, height: 844 },
]) {
  test(`${viewport.name} Account Portfolio operations reply and transition a durable ticket`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/account");
    expect(await page.evaluate(() => window.innerWidth)).toBe(viewport.width);
    await expect(page.getByRole("heading", { name: "账户工单运营" })).toBeVisible();
    await expect(page.getByText("李同学", { exact: true })).toHaveCount(0);
    await expect(page.getByText("very.long.support.ticket.owner@henu.edu.cn", { exact: true })).toHaveCount(0);
    await expect(page.getByText("未设置姓名", { exact: true })).toHaveCount(0);
    await expect(page.getByText("邮箱不可用", { exact: true })).toHaveCount(0);
    await expect(page.getByText(/用户 #[0-9a-f]{4}/)).toHaveCount(0);
    await page.getByRole("button", { name: "练习记录问题" }).click();
    await expect(page.getByText("请帮我核对本次练习记录。", { exact: true })).toBeVisible();
    await expect(page.getByText("李同学", { exact: true })).toBeVisible();
    await expect(page.getByText("very.long.support.ticket.owner@henu.edu.cn", { exact: true })).toBeVisible();
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

test("an unknown reply survives reload using only its original result query", async ({ page }) => {
  let writes = 0;
  let queries = 0;
  let originalKey = "";
  let committed = false;
  const currentTicket = () => ({ ...baseTicket, version: committed ? 2 : 1, status: committed ? "in_progress" : "open" });
  await page.route("**/api/v1/account/tickets/facts?*", (route) => route.fulfill({ json: { data: queueFacts([currentTicket()], route.request().url()), request_id: "req_ticket_recovery_queue" } }));
  await page.route(`**/api/v1/account/tickets/${ticketID}`, (route) => route.fulfill({ json: { data: { ticket: currentTicket(), messages: [], events: [] }, request_id: "req_ticket_recovery_detail" } }));
  await page.route(`**/api/v1/account/tickets/${ticketID}/replies`, async (route) => {
    writes++;
    originalKey = route.request().headers()["idempotency-key"]!;
    committed = true;
    await route.abort("failed");
  });
  await page.route("**/api/v1/account/operations/ticket_reply/*", async (route) => {
    queries++;
    expect(route.request().method()).toBe("GET");
    expect(route.request().url()).toContain(encodeURIComponent(originalKey));
    await route.fulfill({ json: { data: queries === 1 ? { operation: "ticket_reply", status: "unknown" } : { operation: "ticket_reply", status: "succeeded", result: { ticket: currentTicket() } }, request_id: "req_ticket_recovery" } });
  });
  await page.goto("/account");
  await page.getByRole("button", { name: "练习记录问题" }).click();
  await page.getByLabel("客服回复").fill("回复正文不应进入恢复存储");
  await page.getByRole("button", { name: "发送回复" }).click();
  await expect(page.getByRole("status")).toContainText("结果仍未确认");
  const stored = await page.evaluate(() => sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1"));
  expect(stored).not.toContain("回复正文");
  expect(JSON.parse(stored!)).toMatchObject({ operator_id: session.user.id, resource_id: ticketID, idempotency_key: originalKey });
  await expect(page.getByRole("button", { name: "发送回复" })).toBeDisabled();
  await page.reload();
  await expect(page.getByRole("status")).toContainText("回复已写入工单");
  expect(writes).toBe(1);
  expect(queries).toBe(2);
});

test("late A detail cannot replace B or submit B's reply with A's revision", async ({ page }) => {
  let releaseA!: () => void;
  const waitA = new Promise<void>((resolve) => { releaseA = resolve; });
  await page.route(`**/api/v1/account/tickets/${ticketID}`, async (route) => { await waitA; await route.fulfill({ json: { data: { ticket: baseTicket, messages: [], events: [] }, request_id: "req_late_a" } }); });
  const ticketB = { ...baseTicket, id: missingIdentityTicketID, reference: `HKT-${missingIdentityTicketID}`, title: "身份资料缺失", version: 7 };
  await page.route(`**/api/v1/account/tickets/${missingIdentityTicketID}`, (route) => route.fulfill({ json: { data: { ticket: ticketB, messages: [], events: [] }, request_id: "req_fast_b" } }));
  await page.route(`**/api/v1/account/tickets/${missingIdentityTicketID}/replies`, async (route) => {
    expect(await route.request().postDataJSON()).toEqual({ body: "回复工单 B", expected_version: 7 });
    await route.fulfill({ json: { data: { ticket: { ...ticketB, version: 8, status: "in_progress" } }, request_id: "req_b_reply" } });
  });
  await page.goto("/account");
  await page.getByRole("button", { name: "练习记录问题" }).click();
  await page.getByRole("button", { name: "身份资料缺失" }).click();
  await expect(page.locator('[data-account-ticket-detail-state="ready"] h2')).toHaveText("身份资料缺失");
  releaseA();
  await expect(page.locator('[data-account-ticket-detail-state="ready"] h2')).toHaveText("身份资料缺失");
  await page.getByLabel("客服回复").fill("回复工单 B");
  await page.getByRole("button", { name: "发送回复" }).click();
  await expect(page.getByRole("status")).toContainText("回复已写入工单");
});
