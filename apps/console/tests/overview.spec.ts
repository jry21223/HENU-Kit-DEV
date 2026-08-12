import { expect, test } from "@playwright/test";

const moduleNames = ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"];
const broadPermissionTargets = [
  ["产品运行概览", "/"],
  ["平台运营工作台", "/operations"],
  ["校园通知审核与分发", "/notices"],
  ["资料库运营", "/library"],
  ["美食运营", "/food"],
  ["会员权益运营", "/account/memberships"],
  ["积分账本运营", "/account/points"],
  ["账户工单运营", "/account"],
] as const;

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
  await page.route("**/api/v1/overview", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          modules: [
            {
              id: "portal", status: "ok", status_message: "Portal 部署与只读探测正常", as_of: "2026-07-19T00:00:00Z", request_id: "req_portal_browser",
              metrics: [
                { label: "部署版本", value: "2026.07.19" }, { label: "Commit", value: "0123456789ab" },
                { label: "部署时间", value: "07-19 08:00" }, { label: "Readiness", value: "ready" },
                { label: "关键探测", value: "2/2" }, { label: "入口健康", value: "2/2" },
                { label: "反馈摘要", value: "0 待处理" }, { label: "当前异常", value: "0" },
              ],
            },
            { id: "platform", status: "partial", metrics: [], status_message: "部分来源可用", as_of: "2026-07-19T00:00:00Z", request_id: "req_platform_browser" },
            { id: "notice", status: "empty", metrics: [], status_message: "当前无待办", as_of: "2026-07-19T00:00:00Z", request_id: "req_notice_browser" },
            { id: "library", status: "stale", metrics: [], status_message: "展示最近成功摘要", as_of: "2026-07-19T00:00:00Z", last_success_at: "2026-07-19T00:00:01Z", request_id: "req_library_browser" },
            { id: "quizcraft", status: "unavailable", metrics: [], status_message: "摘要暂不可用", request_id: "req_quizcraft_browser" },
            { id: "food", status: "ok", metrics: [], status_message: "摘要可用", as_of: "2026-07-19T00:00:00Z", request_id: "req_food_browser" },
          ],
          generated_at: "2026-07-19T00:00:01Z",
        },
        request_id: "req_overview_browser",
      }),
    }),
  );
});

test("desktop overview exposes six traced module summaries and degradation states", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "产品运行概览" })).toBeVisible();
  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  for (const name of moduleNames) await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  await expect(page.locator('[data-state="ok"]')).toHaveCount(2);
  for (const state of ["empty", "partial", "stale", "unavailable"]) await expect(page.locator(`[data-state="${state}"]`)).toHaveCount(1);
  await expect(page.getByText("积分", { exact: true })).toHaveCount(0);
  await expect(page.getByText("会员", { exact: true })).toHaveCount(0);
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
  await expect(page.getByText("摘要暂不可用", { exact: true }).first()).toBeVisible();
  const portal = page.locator('[data-module-card="portal"]');
  await expect(portal).toHaveAccessibleName("Portal：正常");
  for (const fact of ["2026.07.19", "0123456789ab", "Readiness", "关键探测", "入口健康", "反馈摘要", "当前异常"]) await expect(portal.getByText(fact, { exact: true })).toBeVisible();
  await expect(portal.getByLabel("Readiness：ready")).toBeVisible();
  await expect(portal.getByLabel("入口健康：2/2")).toBeVisible();
  for (const control of ["编辑内容", "重新部署", "回滚版本", "切换版本"]) await expect(page.getByText(control, { exact: true })).toHaveCount(0);
});

// A static attribute such as description="{{ summaries.length }} …" is literal
// text to Vue, so the placeholder shipped to production unrendered. Assert the
// resolved copy, and fail the whole page on any leaked mustache.
test("desktop overview renders interpolated copy instead of raw template syntax", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page.getByText("6 个运营模块的运行状态与关键指标总览，供运营人员快速了解全站情况。", { exact: true })).toBeVisible();
  await expect(page.locator("body")).not.toContainText("{{");
});

test("390px overview keeps its single real navigation target usable without overflow", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  await page.getByRole("button", { name: "打开运营导航" }).click();
  const navigation = page.getByRole("navigation", { name: "移动端运营导航" });
  await expect(navigation.getByRole("link")).toHaveCount(1);
  const overviewLink = navigation.getByRole("link", { name: "产品运行概览" });
  await expect(overviewLink).toHaveAttribute("href", "/");
  expect(await overviewLink.evaluate((link) => link.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);
  await expect(navigation.locator('a[href^="#"]')).toHaveCount(0);
  await expect(page.getByRole("button", { name: "关闭运营导航" })).toBeVisible();
  await page.getByRole("button", { name: "关闭运营导航" }).click();
  await expect(page.getByRole("dialog")).toBeHidden();

  const portal = page.locator('[data-module-card="portal"]');
  await expect(portal).toHaveAccessibleName("Portal：正常");
  await expect(portal.getByText("Readiness", { exact: true })).toBeVisible();
  await expect(portal.getByText("入口健康", { exact: true })).toBeVisible();
  await expect(portal.getByLabel("Readiness：ready")).toBeVisible();

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
});

test("desktop and 390px overview distinguish owners that are not onboarded", async ({ page }) => {
  await page.unroute("**/api/v1/overview");
  await page.route("**/api/v1/overview", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          modules: [
            { id: "portal", status: "ok", metrics: [], status_message: "Portal 摘要可用", as_of: "2026-08-12T12:00:00Z", request_id: "req_portal_onboarded" },
            { id: "platform", status: "unavailable", unavailable_reason: "not_onboarded", metrics: [], status_message: "Platform Operations 摘要尚未接入，请前往平台运营工作台查看实时数据", request_id: "req_platform_not_onboarded" },
            { id: "notice", status: "empty", metrics: [], status_message: "当前无待办", as_of: "2026-08-12T12:00:00Z", request_id: "req_notice_onboarded" },
            { id: "library", status: "ok", metrics: [], status_message: "Library 摘要可用", as_of: "2026-08-12T12:00:00Z", request_id: "req_library_onboarded" },
            { id: "quizcraft", status: "unavailable", unavailable_reason: "not_onboarded", metrics: [], status_message: "QuizCraft 摘要尚未接入；题库工坊入口尚未配置", request_id: "req_quizcraft_not_onboarded" },
            { id: "food", status: "ok", metrics: [], status_message: "Food 摘要可用", as_of: "2026-08-12T12:00:00Z", request_id: "req_food_onboarded" },
          ],
          generated_at: "2026-08-12T12:00:00Z",
        },
        request_id: "req_overview_not_onboarded",
      }),
    }),
  );

  for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport);
    await page.goto("/");
    const platform = page.locator('[data-module-card="platform"]');
    const quizcraft = page.locator('[data-module-card="quizcraft"]');
    await expect(platform.getByText("尚未接入", { exact: true })).toBeVisible();
    await expect(platform.getByText("Platform Operations 摘要尚未接入，请前往平台运营工作台查看实时数据", { exact: true })).toBeVisible();
    await expect(quizcraft.getByText("尚未接入", { exact: true })).toBeVisible();
    await expect(quizcraft.getByText("QuizCraft 摘要尚未接入；题库工坊入口尚未配置", { exact: true })).toBeVisible();
    await expect(quizcraft.getByRole("link", { name: "打开 QuizCraft 题库工坊" })).toHaveCount(0);
    const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  }
});

test("loading scenario marks all six modules busy without fake metrics", async ({ page }) => {
  await page.goto("/?scenario=loading");
  await expect(page.locator("section[aria-busy='true']")).toBeVisible();
  await expect(page.locator("[data-state='loading']")).toHaveCount(6);
  await expect(page.locator("[data-metric-tile]")).toHaveCount(0);
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
  await expect(page.locator("[data-state='denied']")).toHaveCount(6);
  await expect(page.locator("[data-metric-tile]")).toHaveCount(0);
  await login.click();

  await expect(page).toHaveURL(/\/\?tab=inbox$/);
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
  await expect(page.locator("[data-metric-tile]")).not.toHaveCount(0);
  expect((await context.cookies()).some((cookie) => cookie.name === "henukit_console_e2e" && cookie.httpOnly)).toBe(true);
});

// A broadly authorized operator used to receive both workspace links and
// English module-card anchors. Assert one canonical link for every real route.
test("overview navigation exposes each permitted operational target once", async ({ page }) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
          access_context: {
            permissions: [
              "console.overview.read",
              "platform.operations.read",
              "notice.read",
              "library.read",
              "food.read",
              "account.membership.write",
              "account.points.adjust",
              "account.tickets.read",
            ],
            scopes: [{ kind: "platform" }],
            verified_at: "2026-07-19T00:00:00Z",
          },
          expires_at: "2026-07-19T00:05:00Z",
        },
        request_id: "req_browser_console",
      }),
    }),
  );
  await page.route("**/api/v1/operations", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          accounts: [], sessions: [],
          mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 0, failed: 0, dead_letters: 0 },
          inbox_items: [], audit: [], dependencies: { postgres: "ready", redis: "ready" },
          generated_at: "2026-07-19T00:00:00Z",
        },
        request_id: "req_operations_envelope",
      }),
    }),
  );

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "产品运行概览" })).toBeVisible();
  const navigation = page.getByRole("navigation", { name: "运营导航" });

  await expect(navigation.getByRole("link")).toHaveCount(broadPermissionTargets.length);
  await expect(navigation.locator('a[href^="#"]')).toHaveCount(0);
  for (const [label, href] of broadPermissionTargets) {
    await expect(navigation.getByRole("link", { name: label, exact: true })).toHaveCount(1);
    await expect(navigation.getByRole("link", { name: label, exact: true })).toHaveAttribute("href", href);
  }

  const consoleRouteLinks = page.locator('[data-console-shell] a[href^="/"]');
  await expect(consoleRouteLinks).toHaveCount(broadPermissionTargets.length);
  expect(await consoleRouteLinks.evaluateAll((links) => new Set(links.map((link) => link.getAttribute("href"))).size)).toBe(broadPermissionTargets.length);

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);

  const platform = navigation.getByRole("link", { name: "平台运营工作台", exact: true });
  await platform.focus();
  await expect(platform).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/operations$/);
  await expect(page.getByRole("heading", { name: "平台运营工作台" })).toBeVisible();
});

test("390px navigation exposes distinct permitted operational targets without horizontal overflow", async ({ page }) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
          access_context: {
            permissions: [
              "console.overview.read",
              "platform.operations.read",
              "notice.read",
              "library.read",
              "food.read",
              "account.membership.write",
              "account.points.adjust",
              "account.tickets.read",
            ],
            scopes: [{ kind: "platform" }],
            verified_at: "2026-07-19T00:00:00Z",
          },
          expires_at: "2026-07-19T00:05:00Z",
        },
        request_id: "req_browser_console_mobile",
      }),
    }),
  );
  await page.route("**/api/v1/operations", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          accounts: [], sessions: [],
          mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 0, failed: 0, dead_letters: 0 },
          inbox_items: [], audit: [], dependencies: { postgres: "ready", redis: "ready" },
          generated_at: "2026-07-19T00:00:00Z",
        },
        request_id: "req_operations_mobile_envelope",
      }),
    }),
  );

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await page.getByRole("button", { name: "打开运营导航" }).click();

  const navigation = page.getByRole("navigation", { name: "移动端运营导航" });
  await expect(navigation.getByRole("link")).toHaveCount(broadPermissionTargets.length);
  await expect(navigation.locator('a[href^="#"]')).toHaveCount(0);
  for (const [label, href] of broadPermissionTargets) {
    const link = navigation.getByRole("link", { name: label, exact: true });
    await expect(link).toHaveCount(1);
    await expect(link).toHaveAttribute("href", href);
    expect(await link.evaluate((element) => element.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);
  }

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);

  await navigation.getByRole("link", { name: "平台运营工作台", exact: true }).click();
  await expect(page).toHaveURL(/\/operations$/);
  await expect(page.getByRole("heading", { name: "平台运营工作台" })).toBeVisible();
});

test("overview navigation does not expose an unverified workspace", async ({ page }) => {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
          access_context: {
            permissions: ["console.overview.read", "food.read"],
            scopes: [{ kind: "product", product_code: "food" }],
            verified_at: "2026-07-19T00:00:00Z",
          },
          expires_at: "2026-07-19T00:05:00Z",
        },
        request_id: "req_browser_limited_console",
      }),
    }),
  );

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  const navigation = page.getByRole("navigation", { name: "运营导航" });
  await expect(navigation.getByRole("link")).toHaveCount(2);
  await expect(navigation.getByRole("link", { name: "产品运行概览", exact: true })).toBeVisible();
  await expect(navigation.getByRole("link", { name: "美食运营", exact: true })).toBeVisible();
  for (const hiddenWorkspace of ["平台运营工作台", "校园通知审核与分发", "资料库运营", "会员权益运营", "积分账本运营", "账户工单运营"]) {
    await expect(navigation.getByRole("link", { name: hiddenWorkspace, exact: true })).toHaveCount(0);
  }
});

test("overview count drops to 0/6 when the overview feed is unavailable", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.route("**/api/v1/overview", (route) => route.fulfill({ status: 500, contentType: "application/json", body: "{}" }));
  await page.goto("/");

  await expect(page.locator("[data-state='unavailable']")).toHaveCount(6);
  await expect(page.getByText("0/6 可见", { exact: true })).toBeVisible();
  await expect(page.getByText("概览数据暂时不可用，请稍后刷新页面。", { exact: true })).toHaveCount(6);
});

test("degraded status messages render exactly once per card", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  // Degraded card explanations must not appear both in the state panel and
  // the footer; metric-bearing cards keep theirs in the footer only.
  await expect(page.getByText("部分来源可用", { exact: true })).toHaveCount(1);
  await expect(page.getByText("摘要暂不可用", { exact: true })).toHaveCount(1);
  await expect(page.getByText("Portal 部署与只读探测正常", { exact: true })).toHaveCount(1);
});

test("module cards are informational while Console navigation remains canonical", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  for (const moduleID of ["portal", "platform", "notice", "library", "quizcraft", "food"]) {
    const card = page.locator(`[data-module-card="${moduleID}"]`);
    await expect(card).toHaveJSProperty("tagName", "ARTICLE");
    expect(await card.getAttribute("href")).toBeNull();
  }
});

test("overview timestamps render in local time instead of raw UTC", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");

  await expect(page.getByText(/^截至 /).first()).toBeVisible();
  const library = page.locator('[data-module-card="library"]');
  await expect(library.getByText(/^最近成功 /)).toBeVisible();
  await expect(library).not.toContainText("2026-07-19T00:00:01Z");
  await expect(page.locator('[data-module-card="portal"]')).not.toContainText("2026-07-19T00:00:00Z");
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
  await expect(page.locator("[data-module-card]")).toHaveCount(6);
});
