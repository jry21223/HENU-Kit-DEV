import { expect, test, type Page } from "@playwright/test";

const operatorId = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";

const fullSession = {
  user: { id: operatorId },
  access_context: {
    permissions: [
      "console.overview.read",
      "platform.operations.read",
      "platform.operations.write",
      "notice.read",
      "notice.manage",
      "notice.review",
      "notice.distribute",
      "library.read",
      "food.read",
      "food.review",
      "food.anomaly",
      "food.tier_adjust",
      "quizcraft.workshop.read",
      "quizcraft.workshop.write",
      "quizcraft.workshop.publish",
      "account.tickets.read",
      "account.tickets.reply",
      "account.tickets.transition",
      "account.membership.write",
      "account.orders.read",
      "account.orders.close",
      "account.orders.refund",
      "account.points.adjust",
    ],
    scopes: [
      { kind: "platform" },
      { kind: "product", product_code: "notice" },
      { kind: "product", product_code: "library" },
      { kind: "product", product_code: "food" },
      { kind: "product", product_code: "quizcraft" },
      { kind: "product", product_code: "account-portfolio" },
    ],
    verified_at: "2026-11-22T04:00:00Z",
  },
  expires_at: "2026-11-22T08:00:00Z",
};

async function installMockGateway(page: Page) {
  await page.route("**/api/v1/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const pathname = url.pathname;
    const json = (data: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify({ data, request_id: "req_e2e_console" }),
      });

    if (pathname === "/api/v1/session") return json(fullSession);
    if (pathname === "/api/v1/session/logout") return json({ ok: true });
    if (pathname === "/api/v1/overview") {
      return json({
        generated_at: "2026-11-22T04:00:00Z",
        modules: [
          {
            id: "portal",
            status: "ok",
            status_message: "主站访问正常",
            metrics: [{ label: "当前版本", value: "v2.5.0" }],
            request_id: "req_portal",
          },
          {
            id: "food",
            status: "ok",
            status_message: "美食服务正常",
            metrics: [],
            request_id: "req_food",
          },
        ],
      });
    }
    if (pathname === "/api/v1/food/facts") {
      return json({
        as_of: "2026-11-22T04:00:00Z",
        scope: "global",
        stale: false,
        counts: { pending_submissions: 2, open_anomalies: 1, pending_tier_adjustments: 1 },
        windows: {
          submissions: { filter: "pending", has_more: false, items: [], limit: 100, returned: 0 },
          anomalies: { filter: "open", has_more: false, items: [], limit: 100, returned: 0 },
          tier_adjustments: { filter: "pending", has_more: false, items: [], limit: 100, returned: 0 },
        },
      });
    }
    if (pathname === "/api/v1/food") {
      return json({
        as_of: "2026-11-22T04:00:00Z",
        stale: false,
        status: "ok",
        status_message: "ok",
        submissions: [
          {
            id: "11111111-1111-1111-1111-111111111111",
            venue_name: "南门牛肉面",
            item_name: "红烧牛肉面",
            description: "汤鲜肉厚，性价比高",
            campus: "minglun",
            status: "pending",
            version: 1,
            submitted_at: "2026-11-22T03:00:00Z",
            updated_at: "2026-11-22T03:00:00Z",
          },
        ],
        posts: [
          {
            id: "11111111-1111-1111-1111-111111111112",
            venue_name: "西门烤冷面",
            campus: "jinming",
            tier: "hang",
            review_text: "酱香十足，夜宵首选",
            price_reference: "10元",
            hours_reference: "17:00-23:00",
            author_display_name: "陈同学",
            hidden: false,
            version: 1,
            created_at: "2026-11-22T02:00:00Z",
            updated_at: "2026-11-22T02:00:00Z",
          },
        ],
        anomaly_tickets: [],
        tier_adjustments: [],
      });
    }
    if (pathname === "/api/v1/food/commands") {
      expect(req.headers()["idempotency-key"]).toMatch(/^idem_food_/);
      const body = (await req.postDataJSON()) as { kind: string; resource_id: string };
      return json({
        operation: body.kind,
        resource_id: body.resource_id,
        state: "succeeded",
        version: 2,
      });
    }
    if (pathname === "/api/v1/notices/sources") {
      return json({
        sources: [
          {
            id: "66666666-6666-4666-8666-666666666666",
            code: "henu-jwc",
            name: "教务处",
            canonical_url: "https://jwc.henu.edu.cn",
            created_at: "2026-11-20T00:00:00Z",
          },
        ],
      });
    }
    if (pathname === "/api/v1/notices") {
      return json({
        generated_at: "2026-11-22T04:00:00Z",
        items: [
          {
            id: "55555555-5555-4555-8555-555555555555",
            source: { id: "66666666-6666-4666-8666-666666666666", code: "henu-jwc", name: "教务处" },
            version: 1,
            revision: 1,
            title: "2026秋季期末考试时间安排",
            body: "请各位同学按时登录教务系统确认考场。",
            content_hash: "a".repeat(64),
            source_url: "https://jwc.henu.edu.cn/notice/1",
            state: "pending_review",
            distribution_count: 0,
            created_at: "2026-11-22T03:00:00Z",
          },
        ],
      });
    }
    if (pathname === "/api/v1/library/readiness") {
      return json({
        management: "not_ready",
        reason: "management_migration_pending",
        capabilities: { workspace: false, commands: false, operation_results: false },
        checked_at: "2026-11-22T04:00:00Z",
      });
    }
    if (pathname === "/api/v1/quizcraft/workshop/catalog") {
      return json({
        banks: [
          {
            id: "22222222-2222-4222-8222-222222222222",
            slug: "computer-networks",
            name: "计算机网络题库",
            version: 1,
            active_version_number: 1,
            updated_at: "2026-11-22T03:00:00Z",
            versions: [
              {
                id: "33333333-3333-4333-8333-333333333333",
                bank_id: "22222222-2222-4222-8222-222222222222",
                version_number: 1,
                state: "published",
                question_count: 150,
                created_at: "2026-11-22T01:00:00Z",
              },
            ],
          },
        ],
      });
    }
    if (pathname === "/api/v1/account/tickets/facts") {
      return json({
        as_of: "2026-11-22T04:00:00Z",
        scope: "global",
        stale: false,
        counts: { unresolved: 1, open: 1, in_progress: 0, resolved: 3 },
        window: {
          filter: url.searchParams.get("status") ?? "unresolved",
          limit: 100,
          matched_total: 1,
          returned: 1,
          has_more: false,
          items: [
            {
              id: "88888888-8888-4888-8888-888888888888",
              reference: "HKT-88888888-8888-4888-8888-888888888888",
              title: "练习记录同步异常",
              status: "open",
              version: 1,
              updated_at: "2026-11-22T03:00:00Z",
            },
          ],
        },
      });
    }
    if (pathname === "/api/v1/account/tickets/88888888-8888-4888-8888-888888888888") {
      return json({
        ticket: {
          id: "88888888-8888-4888-8888-888888888888",
          reference: "HKT-88888888-8888-4888-8888-888888888888",
          display_name: "王同学",
          email: "wang@stu.henu.edu.cn",
          title: "练习记录同步异常",
          category: "practice",
          status: "open",
          version: 1,
          created_at: "2026-11-22T03:00:00Z",
          updated_at: "2026-11-22T03:00:00Z",
        },
        messages: [
          {
            id: "99999999-9999-4999-8999-999999999999",
            author_kind: "user",
            body: "昨天刷的题目没有计入统计，麻烦看一下。",
            created_at: "2026-11-22T03:00:00Z",
          },
        ],
        events: [],
      });
    }
    if (pathname === "/api/v1/account/memberships/search") {
      return json({
        accounts: [
          {
            id: "77777777-7777-4777-8777-777777777777",
            display_name: "赵同学",
            email: "zhao@stu.henu.edu.cn",
            status: "active",
            membership: { plan: "free", lifetime: false, version: 1 },
          },
        ],
        next_page: null,
      });
    }
    if (pathname === "/api/v1/operations/account-lookups") {
      return json({
        account: {
          id: "44444444-4444-4444-8444-444444444444",
          display_name: "李同学",
          email: "li@stu.henu.edu.cn",
          status: "active",
        },
      });
    }
    if (pathname === "/api/v1/account/membership-orders") {
      return json({
        orders: [
          {
            id: "9f1c2a44-6f3d-4c2b-9a71-2b6d5e8c4a10",
            user_id: "77777777-7777-4777-8777-777777777777",
            display_name: "赵同学",
            email: "zhao@stu.henu.edu.cn",
            plan: "lifetime",
            amount_cents: 1990,
            status: "paid",
            version: 2,
            created_at: "2026-11-22T01:00:00Z",
            updated_at: "2026-11-22T01:01:00Z",
          },
        ],
      });
    }
    if (pathname.startsWith("/api/v1/account/points/")) {
      return json({
        balance: 200,
        entries: [],
      });
    }
    if (pathname === "/api/v1/operations/roles") {
      return json({
        roles: [
          {
            code: "operations-operator",
            display_name: "平台运营",
            permissions: ["platform.operations.read", "platform.operations.write"],
          },
        ],
      });
    }
    if (pathname === "/api/v1/operations") {
      return json({
        access_context: fullSession.access_context,
        accounts: [
          {
            id: operatorId,
            display_name: "张老师",
            email: "zhang@henu.edu.cn",
            email_verified: true,
            status: "active",
            authorization_revision: 1,
            created_at: "2026-11-22T00:00:00Z",
            grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }],
          },
        ],
        sessions: [
          {
            id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302",
            user_id: operatorId,
            display_name: "张老师",
            email: "zhang@henu.edu.cn",
            kind: "core",
            last_seen_at: "2026-11-22T03:30:00Z",
            expires_at: "2026-11-22T08:00:00Z",
          },
        ],
        inbox_items: [],
        audit: [],
        mail: {
          pending: 1,
          processing: 0,
          retry_due: 0,
          accepted: 0,
          delivered: 12,
          failed: 0,
          dead_letters: 0,
        },
        pagination: {
          accounts: { page: 1, next_page: null, next_cursor: null },
          sessions: { page: 1, next_page: null, next_cursor: null },
          inbox_items: { page: 1, next_page: null, next_cursor: null },
          audit: { page: 1, next_page: null, next_cursor: null },
        },
        dependencies: { postgres: "ready", redis: "ready" },
        generated_at: "2026-11-22T04:00:00Z",
      });
    }
    return json({});
  });
}

test("signed-out route preserves OAuth login CTA and safe return_to without false expired notice", async ({
  page,
}) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "UNAUTHORIZED", message: "unauthorized" } }),
    }),
  );

  await page.goto("/food");
  await expect(page.getByText("请先登录管理后台")).toBeVisible();
  await expect(page.getByText("登录已过期，请重新登录")).toHaveCount(0);
  const loginLink = page.getByRole("link", { name: "登录 Console" }).first();
  await expect(loginLink).toBeVisible();
  await expect(loginLink).toHaveAttribute("href", "/api/v1/auth/login?return_to=%2Ffood");
});

for (const viewport of [
  { name: "desktop-1440", width: 1440, height: 960 },
  { name: "mobile-390", width: 390, height: 844 },
]) {
  test(`${viewport.name}: all 9 Console routes render cleanly with zero console errors and zero overflow`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));
    page.on("console", (msg) => {
      if (msg.type() === "error") errors.push(msg.text());
    });

    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await installMockGateway(page);

    const routes = [
      { path: "/", headingId: "overview-heading", expectedText: "美食内容审核" },
      { path: "/food", headingId: "food-heading", expectedText: "南门牛肉面" },
      { path: "/notices", headingId: "notice-heading", expectedText: "2026秋季期末考试时间安排" },
      { path: "/practice", headingId: "practice-heading", expectedText: "计算机网络题库" },
      { path: "/library", headingId: "library-heading", expectedText: "资料在线管理功能尚未就绪" },
      { path: "/account", headingId: "account-tickets-heading", expectedText: "练习记录同步异常" },
      { path: "/account/memberships", headingId: "account-membership-heading", expectedText: "赵同学" },
      { path: "/account/points", headingId: "account-points-heading", expectedText: "查找用户并填写变动" },
      { path: "/operations", headingId: "operations-heading", expectedText: "张老师" },
    ];

    for (const r of routes) {
      await page.goto(r.path);
      await expect(page.locator(`#${r.headingId}`)).toBeVisible();
      await expect(page.getByText(r.expectedText).first()).toBeVisible();

      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(overflow, `horizontal overflow on ${r.path} (${viewport.name})`).toBeLessThanOrEqual(2);

      const headerClipped = await page.evaluate(() => {
        const header = document.querySelector("header");
        if (!header) return 0;
        return header.scrollHeight - header.clientHeight;
      });
      expect(headerClipped, `header vertical clipping on ${r.path} (${viewport.name})`).toBeLessThanOrEqual(2);
    }

    expect(errors).toEqual([]);
  });
}

test("desktop interactive flow: Food modal approve & Platform access drawer", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 960 });
  await installMockGateway(page);

  await page.goto("/food");
  await expect(page.getByText("南门牛肉面")).toBeVisible();
  await page.getByRole("button", { name: /批\s*准/ }).click();
  await expect(page.getByText("批准投稿上架")).toBeVisible();
  await page.getByPlaceholder(/信息核实无误/).fill("实地核实无误，同意上架");
  await page.getByRole("button", { name: "确认批准" }).click();
  await expect(page.getByText("投稿已审核通过。")).toBeVisible();

  // SPA navigate to /operations via sidebar menu
  await page.getByRole("menuitem", { name: /平台与安全/ }).click();
  await expect(page.locator("#operations-heading")).toBeVisible();
  await expect(page.getByText("张老师")).toBeVisible();
  await page.getByRole("button", { name: /管理权限/ }).click();
  await expect(page.getByText("管理权限：张老师")).toBeVisible();
});
