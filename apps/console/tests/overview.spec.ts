import { expect, test } from "@playwright/test";

// Controlled test data only. Listing these cases is not browser execution.
test.use({ timezoneId: "Asia/Shanghai" });
const asOf = "2026-11-07T10:01:00Z";
const actor = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const permissions = ["console.overview.read", "platform.operations.read", "notice.read", "library.read", "food.read", "account.membership.write", "account.points.adjust", "account.tickets.read"];
const broadPermissionTargets = [["全局平台概览", "/"], ["平台运营工作台", "/operations"], ["校园通知审核与分发", "/notices"], ["资料库运营", "/library"], ["美食运营", "/food"], ["会员权益运营", "/account/memberships"], ["积分调整", "/account/points"], ["账户工单运营", "/account"]] as const;
function session(grants = permissions) { return { user: { id: actor }, access_context: { permissions: grants, scopes: [{ kind: "platform" }, ...["food", "account-portfolio", "notice", "library"].map((product_code) => ({ kind: "product", product_code }))], verified_at: asOf }, expires_at: "2026-11-07T11:00:00Z" }; }
function foodFacts(total = 2) { return { scope: "global", as_of: asOf, stale: false, counts: { pending_submissions: total, open_anomalies: 0, pending_tier_adjustments: 0 }, windows: { submissions: { filter: "pending", limit: 200, returned: total, has_more: false, items: Array.from({ length: total }, (_, index) => ({ id: `${index}1111111-1111-4111-8111-111111111111`, status: "pending", version: 1, updated_at: asOf })) }, anomalies: { filter: "open", limit: 200, returned: 0, has_more: false, items: [] }, tier_adjustments: { filter: "pending", limit: 200, returned: 0, has_more: false, items: [] } } }; }
function tickets(total = 3) { return { scope: "global", as_of: asOf, stale: false, counts: { open: total, in_progress: 0, resolved: 0, unresolved: total }, window: { filter: "unresolved", limit: 100, matched_total: total, returned: total, has_more: false, items: Array.from({ length: total }, (_, index) => { const id = `${index}2222222-2222-4222-8222-222222222222`; return { id, reference: `HKT-${id}`, title: "受控工单", status: "open", version: 1, updated_at: asOf }; }) } }; }
const platform = { access_context: { permissions: ["platform.operations.read"], scopes: [{ kind: "platform" }], verified_at: asOf }, accounts: [], sessions: [], audit: [], inbox_items: [], mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 0, failed: 0, dead_letters: 0 }, pagination: Object.fromEntries(["accounts", "sessions", "inbox_items", "audit"].map((key) => [key, { page: 1, next_page: null, next_cursor: null }])), dependencies: { postgres: "ready", redis: "ready" }, generated_at: asOf };
const readiness = { management: "not_ready", reason: "management_migration_pending", capabilities: { workspace: false, commands: false, operation_results: false }, checked_at: asOf };

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/**", (route) => route.fulfill({ status: 503, json: { error: { code: "FIXTURE_UNAVAILABLE", message: "受控测试未提供该路径" }, request_id: "req_browser_unavailable" } }));
  for (const [path, data] of [["session", session()], ["food/facts", foodFacts()], ["account/tickets/facts?status=unresolved", tickets()], ["library/readiness", readiness]] as const) await page.route((url) => url.pathname + url.search === `/api/v1/${path}`, (route) => route.fulfill({ json: { data, request_id: "req_browser_fact" } }));
  await page.route("**/api/v1/operations?*", (route) => route.fulfill({ json: { data: platform, request_id: "req_browser_platform" } }));
  await page.route("**/api/v1/notices", (route) => route.fulfill({ status: 503, json: { error: { code: "NOTICE_NOT_CONFIGURED", message: "通知工作台尚未接入" }, request_id: "req_browser_notice_unconfigured" } }));
});

for (const viewport of [{ width: 1440, height: 1000 }, { width: 375, height: 812 }, { width: 390, height: 844 }]) {
  test(`${viewport.width}px overview leads with scoped tasks, exact entries and one diagnostic disclosure without overflow`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/");
    expect(await page.evaluate(() => innerWidth)).toBe(viewport.width);
    await expect(page.getByRole("heading", { name: "全局总览", exact: true })).toBeVisible();
    await expect(page.locator("h1")).toHaveCount(1);
    await expect(page.locator("[data-overview-task='food']")).toContainText("2 条投稿待审核");
    await expect(page.locator("[data-overview-task='food'] a")).toHaveAttribute("href", "/food");
    await expect(page.locator("[data-overview-task='tickets']")).toContainText("3 条未解决");
    await expect(page.locator("[data-overview-task='tickets'] a")).toHaveAttribute("href", "/account?status=unresolved");
    await expect(page.locator("[data-overview-platform]")).toContainText("本页未结项 0 条");
    await expect(page.locator("[data-overview-notice]")).toContainText("通知工作台尚未接入");
    await expect(page.locator("[data-overview-library]")).toContainText("资料管理尚未就绪");
    await expect(page.locator("details")).toHaveCount(1);
    await expect(page.locator("details")).not.toHaveAttribute("open", "");
    await page.getByText("连接与诊断", { exact: true }).click();
    await expect(page.locator("details")).toContainText("本次摘要诊断未能完成");
    await expect(page.locator("[data-module-card], [data-module-evidence]")).toHaveCount(0);
    await expect(page.locator("body")).not.toContainText("{{");
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("true owner zero keeps destinations and does not turn a failed module into an empty queue", async ({ page }) => {
  await page.route("**/api/v1/food/facts", (route) => route.fulfill({ json: { data: foodFacts(0), request_id: "req_zero_food" } }));
  await page.route("**/api/v1/account/tickets/facts?*", (route) => route.fulfill({ json: { data: tickets(0), request_id: "req_zero_tickets" } }));
  await page.goto("/");
  await expect(page.getByText("美食与工单当前没有待处理事项。", { exact: true })).toBeVisible();
  await expect(page.locator("[data-overview-task='tickets'] a")).toBeVisible();
  await page.route("**/api/v1/food/facts", (route) => route.fulfill({ status: 503, body: "{}" }));
  await page.getByRole("button", { name: "刷新总览", exact: true }).click();
  await expect(page.locator("[data-overview-task='food']")).toContainText("本次未能确认");
  await expect(page.getByText("美食与工单当前没有待处理事项。", { exact: true })).toHaveCount(0);
});

test("summary403 revokes only the combined Overview and leaves other authorized destinations", async ({ page }) => {
  await page.route("**/api/v1/overview", (route) => route.fulfill({ status: 403, body: "{}" }));
  await page.goto("/");
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
  await expect(page.getByText(/当前账户没有总览查看权限/)).toBeVisible();
  await expect(page.getByRole("navigation", { name: "可用工作台" }).getByRole("link", { name: "美食运营" })).toHaveAttribute("href", "/food");
  await expect(page.locator("[data-overview-task]")).toHaveCount(0);
});

test("Session503 stays unverified with a working retry and no private tasks or fake deny", async ({ page }) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 503, body: "{}" }));
  await page.goto("/");
  await expect(page.getByText("暂时无法验证访问权限，请重试连接。", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "重试连接", exact: true })).toBeVisible();
  await expect(page.locator("body")).not.toContainText("无权读取");
  await expect(page.locator("[data-overview-task]")).toHaveCount(0);
});

for (const width of [1440, 390]) test(`${width}px navigation exposes each permitted destination once inside the real navigation`, async ({ page }) => {
  await page.setViewportSize({ width, height: 1000 }); await page.goto("/");
  if (width < 1024) await page.getByRole("button", { name: "打开运营导航" }).click();
  const navigation = page.getByRole("navigation", { name: width < 1024 ? "移动端运营导航" : "运营导航", exact: true });
  await expect(navigation.getByRole("link")).toHaveCount(broadPermissionTargets.length);
  for (const [label, href] of broadPermissionTargets) { const link = navigation.getByRole("link", { name: label, exact: true }); await expect(link).toHaveCount(1); await expect(link).toHaveAttribute("href", href); expect(await link.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44); }
  await expect(navigation.locator('a[href^="#"]')).toHaveCount(0);
});

test("the diagnostic disclosure is keyboard accessible and source timestamps retain their provenance", async ({ page }) => {
  await page.goto("/");
  const diagnostic = page.locator("details > summary");
  await diagnostic.focus(); await expect(diagnostic).toBeFocused(); await page.keyboard.press("Enter");
  await expect(page.locator("details")).toHaveAttribute("open", "");
  await expect(page.locator("[data-overview-task='food'] time")).toHaveAttribute("datetime", asOf);
  await expect(page.locator("[data-overview-task='food']")).toContainText("北京时间");
  await expect(page.locator("details")).toContainText(asOf);
  await page.keyboard.press("Enter"); await expect(diagnostic).toBeFocused();
});

test("expired session completes sign-in callback and returns to the intended path", async ({ page, context }) => {
  await page.unroute("**/api/v1/session");
  await page.route("**/api/v1/session", (route) => {
    const authenticated = route.request().headers().cookie?.includes("henukit_console_e2e=bound");
    return authenticated
      ? route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
              access_context: { permissions: ["console.overview.read"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
              expires_at: "2026-07-19T00:05:00Z",
            },
            request_id: "req_browser_return",
          }),
        })
      : route.fulfill({ status: 401, contentType: "application/json", body: "{}" });
  });
  let storedReturn = "/";
  await context.route(/\/api\/v1\/auth\/login(?:\?|$)/, (route) => {
    storedReturn = new URL(route.request().url()).searchParams.get("return_to") ?? "/";
    return route.fulfill({
      status: 200,
      contentType: "text/html",
      body: `<script>location.replace('/api/v1/auth/callback?code=authorization_code_123456&state=browser_bound_state_123456789012345')</script>`,
    });
  });
  await context.route(/\/api\/v1\/auth\/callback(?:\?|$)/, async (route) => {
    await context.addCookies([{ name: "henukit_console_e2e", value: "bound", url: "http://127.0.0.1:4174", httpOnly: true, sameSite: "Lax" }]);
    return route.fulfill({ status: 200, contentType: "text/html", body: `<script>location.replace(${JSON.stringify(storedReturn)})</script>` });
  });
  await page.goto("/?tab=inbox");

  const login = page.getByRole("link", { name: "登录 Console" });
  await expect(login).toBeVisible();
  await expect(login).toHaveAttribute("href", /return_to=%2F%3Ftab%3Dinbox/);
  await expect(page.locator("[data-overview-task]")).toHaveCount(0);
  await login.click();

  await expect(page).toHaveURL(/\/\?tab=inbox$/);
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "全局总览", exact: true })).toBeVisible();
  await expect(page.locator("details")).toHaveCount(1);
  expect((await context.cookies()).some((cookie) => cookie.name === "henukit_console_e2e" && cookie.httpOnly)).toBe(true);
});

test("signing out asks for confirmation and cancel keeps the session", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.route("**/api/v1/session/logout", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  await page.goto("/");

  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "退出登录" }).click();
  const confirm = page.getByRole("alertdialog");
  await expect(confirm).toBeVisible();
  await expect(confirm.getByText("退出登录？", { exact: true })).toBeVisible();
  await confirm.getByRole("button", { name: "取消" }).click();
  await expect(confirm).toBeHidden();
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "退出登录" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "确认退出" }).click();
  await expect(page.getByRole("link", { name: "登录 Console" })).toBeVisible();
  await expect(page.getByText("权限已验证", { exact: true })).toHaveCount(0);
});

test("switching accounts from the denied state also asks for confirmation", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 403, contentType: "application/json", body: "{}" }));
  await page.route("**/api/v1/session/logout", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  await page.goto("/");

  await expect(page.getByText("权限不足", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "换个账户登录" }).click();
  const confirm = page.getByRole("alertdialog");
  await expect(confirm.getByText("换个账户登录？", { exact: true })).toBeVisible();
  await confirm.getByRole("button", { name: "取消" }).click();
  await expect(page.getByText("权限不足", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "换个账户登录" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "换个账户登录", exact: true }).click();
  await expect(page.getByRole("link", { name: "登录 Console" })).toBeVisible();
});

test("mobile navigation exposes a confirming sign-out entry", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/api/v1/session/logout", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  await page.goto("/");

  await page.getByRole("button", { name: "打开运营导航" }).click();
  const navigation = page.getByRole("dialog");
  await navigation.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  await page.getByRole("alertdialog").getByRole("button", { name: "确认退出" }).click();

  await expect(page.getByRole("link", { name: "登录 Console" })).toBeVisible();
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(page.locator("[data-overview-task]")).toHaveCount(0);
});
