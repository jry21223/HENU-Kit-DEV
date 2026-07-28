import { expect, test, type Page } from "@playwright/test";

const ticketID = "71717171-7171-4717-8717-717171717171";
const userID = "11111111-1111-4111-8111-111111111111";

type TicketStatus = "open" | "in_progress" | "resolved";

test("user creates a ticket, Console replies and resolves it, then the user reads durable notifications", async ({ page: portalPage }) => {
  let status: TicketStatus = "open";
  let version = 1;
  let created = false;
  let notificationSequence = 0;
  const messages: Array<Record<string, string>> = [];
  const events: Array<Record<string, string>> = [];
  const notifications: Array<Record<string, string>> = [];
  const ticket = () => ({
    id: ticketID,
    reference: `HKT-${ticketID}`,
    title: "练习记录问题",
    category: "practice",
    status,
    version,
    created_at: "2030-01-01T00:00:00Z",
    updated_at: `2030-01-01T00:0${version}:00Z`,
  });
  const detail = () => ({ ticket: ticket(), messages, events });
  const addNotification = (kind: string, title: string, body: string) => {
    notificationSequence += 1;
    notifications.unshift({
      id: `81818181-8181-4818-8818-${notificationSequence.toString().padStart(12, "0")}`,
      kind,
      title,
      body,
      ticket_id: ticketID,
      ticket_reference: `HKT-${ticketID}`,
      created_at: `2030-01-01T00:0${version}:30Z`,
    });
  };

  await installPortalRoutes(portalPage, {
    ticket,
    detail,
    messages,
    notifications,
    create: async (route) => {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^portal-ticket:/);
      expect(await route.request().postDataJSON()).toEqual({
        title: "练习记录问题",
        category: "practice",
        body: "请帮我核对本次练习记录。",
      });
      created = true;
      messages.push({
        id: "72727272-7272-4727-8727-727272727272",
        author_kind: "user",
        body: "请帮我核对本次练习记录。",
        created_at: "2030-01-01T00:00:00Z",
      });
      await fulfill(route, 201, { data: { ticket: ticket() }, request_id: "req_portal_ticket_create" });
    },
  });

  await portalPage.goto("/account/tickets", { waitUntil: "domcontentloaded" });
  await portalPage.getByRole("button", { name: "新建工单" }).click();
  await portalPage.getByLabel("标题").fill("练习记录问题");
  await portalPage.getByLabel("问题说明").fill("请帮我核对本次练习记录。");
  await portalPage.getByRole("button", { name: "提交工单" }).click();
  await expect(portalPage.getByText(`HKT-${ticketID}`).first()).toBeVisible();
  expect(created).toBe(true);

  const consolePage = await portalPage.context().newPage();
  await installConsoleRoutes(consolePage, {
    ticket,
    detail,
    reply: async (route) => {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_reply_/);
      expect(await route.request().postDataJSON()).toEqual({ body: "我们已核对并正在处理。", expected_version: 1 });
      messages.push({
        id: "73737373-7373-4737-8737-737373737373",
        author_kind: "operator",
        body: "我们已核对并正在处理。",
        created_at: "2030-01-01T00:01:00Z",
      });
      events.push({
        id: "74747474-7474-4747-8747-747474747474",
        kind: "operator_reply",
        from_status: "open",
        to_status: "in_progress",
        created_at: "2030-01-01T00:01:00Z",
      });
      status = "in_progress";
      version = 2;
      addNotification("ticket_operator_reply", "工单有新回复", "客服已回复你的问题。");
      await fulfill(route, 200, { data: { ticket: ticket() }, request_id: "req_console_reply" });
    },
    transition: async (route) => {
      expect(route.request().headers()["idempotency-key"]).toMatch(/^idem_account_transition_/);
      expect(await route.request().postDataJSON()).toEqual({ status: "resolved", expected_version: 2 });
      events.push({
        id: "75757575-7575-4757-8757-757575757575",
        kind: "status_transition",
        from_status: "in_progress",
        to_status: "resolved",
        created_at: "2030-01-01T00:02:00Z",
      });
      status = "resolved";
      version = 3;
      addNotification("ticket_status", "工单状态已更新", "工单已标记为已解决。");
      await fulfill(route, 200, { data: { ticket: ticket() }, request_id: "req_console_transition" });
    },
  });

  await consolePage.goto("http://127.0.0.1:4174/account", { waitUntil: "domcontentloaded" });
  await expect(consolePage.getByRole("heading", { name: "账户工单运营" })).toBeVisible();
  await consolePage.getByRole("button", { name: "练习记录问题" }).click();
  await consolePage.getByLabel("客服回复").fill("我们已核对并正在处理。");
  await consolePage.getByRole("button", { name: "发送回复" }).click();
  await expect(consolePage.getByRole("status")).toContainText("回复已写入工单");
  await consolePage.getByRole("button", { name: "标记已解决" }).click();
  await expect(consolePage.getByRole("status")).toContainText("工单已标记为已解决");

  await portalPage.goto("/account/notifications", { waitUntil: "domcontentloaded" });
  await expect(portalPage.getByText("客服已回复你的问题。")).toBeVisible();
  await expect(portalPage.getByText("工单已标记为已解决。")).toBeVisible();
  await portalPage.getByRole("button", { name: "标为已读" }).first().click();
  await expect(portalPage.getByText("已读", { exact: true })).toBeVisible();
});

async function installPortalRoutes(
  page: Page,
  state: {
    ticket: () => Record<string, unknown>;
    detail: () => Record<string, unknown>;
    messages: Array<Record<string, string>>;
    notifications: Array<Record<string, string>>;
    create: (route: Parameters<Page["route"]>[1] extends (...args: infer Args) => unknown ? Args[0] : never) => Promise<void>;
  }
) {
  await page.route("**/api/v1/session", (route) =>
    fulfill(route, 200, { user_id: userID, display_name: "小河同学", expires_at: "2030-01-01T00:00:00Z" })
  );
  await page.route("**/api/v1/account/tickets", async (route) => {
    if (route.request().method() === "POST") {
      await state.create(route as never);
      return;
    }
    await fulfill(route, 200, { data: { tickets: state.messages.length === 0 ? [] : [state.ticket()] }, request_id: "req_portal_tickets" });
  });
  await page.route(`**/api/v1/account/tickets/${ticketID}`, (route) => fulfill(route, 200, { data: state.detail(), request_id: "req_portal_ticket_detail" }));
  await page.route("**/api/v1/account/notifications", (route) => fulfill(route, 200, { data: { notifications: state.notifications }, request_id: "req_portal_notifications" }));
  await page.route("**/api/v1/account/notifications/*/read", async (route) => {
    expect(route.request().headers()["idempotency-key"]).toMatch(/^portal-notification:/);
    const notification = state.notifications.find((value) => route.request().url().includes(value.id));
    expect(notification).toBeDefined();
    notification!.read_at = "2030-01-01T00:03:00Z";
    await fulfill(route, 200, { data: { notification }, request_id: "req_portal_notification_read" });
  });
}

async function installConsoleRoutes(
  page: Page,
  state: {
    ticket: () => Record<string, unknown>;
    detail: () => Record<string, unknown>;
    reply: (route: Parameters<Page["route"]>[1] extends (...args: infer Args) => unknown ? Args[0] : never) => Promise<void>;
    transition: (route: Parameters<Page["route"]>[1] extends (...args: infer Args) => unknown ? Args[0] : never) => Promise<void>;
  }
) {
  await page.route("**/api/v1/session", (route) =>
    fulfill(route, 200, {
      data: {
        user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
        access_context: {
          permissions: ["account.tickets.read", "account.tickets.reply", "account.tickets.transition"],
          scopes: [{ kind: "product", product_code: "account-portfolio" }],
          verified_at: "2030-01-01T00:00:00Z",
        },
        expires_at: "2030-01-01T01:00:00Z",
      },
      request_id: "req_console_session",
    })
  );
  await page.route("**/api/v1/account/tickets", (route) => fulfill(route, 200, { data: { tickets: [state.ticket()] }, request_id: "req_console_queue" }));
  await page.route(`**/api/v1/account/tickets/${ticketID}`, (route) => fulfill(route, 200, { data: state.detail(), request_id: "req_console_detail" }));
  await page.route(`**/api/v1/account/tickets/${ticketID}/replies`, async (route) => state.reply(route as never));
  await page.route(`**/api/v1/account/tickets/${ticketID}/transitions`, async (route) => state.transition(route as never));
}

async function fulfill(route: Parameters<Page["route"]>[1] extends (...args: infer Args) => unknown ? Args[0] : never, status: number, body: unknown) {
  await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}
