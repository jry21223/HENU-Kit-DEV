import { expect, test } from "@playwright/test";

const moduleNames = ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"];

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
  await expect(page.getByText("摘要暂不可用", { exact: true })).toBeVisible();
  const portal = page.locator('[data-module-card="portal"]');
  await expect(portal).toHaveAccessibleName("Portal：正常");
  for (const fact of ["2026.07.19", "0123456789ab", "Readiness", "关键探测", "入口健康", "反馈摘要", "当前异常"]) await expect(portal.getByText(fact, { exact: true })).toBeVisible();
  await expect(portal.getByLabel("Readiness：ready")).toBeVisible();
  await expect(portal.getByLabel("入口健康：2/2")).toBeVisible();
  for (const control of ["编辑内容", "重新部署", "回滚版本", "切换版本"]) await expect(page.getByText(control, { exact: true })).toHaveCount(0);
});

test("390px overview keeps every module and mobile navigation usable", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await expect(page.locator("[data-module-card]")).toHaveCount(6);
  await page.getByRole("button", { name: "打开产品导航" }).click();
  const navigation = page.getByRole("navigation", { name: "移动端产品模块" });
  await expect(navigation.getByRole("link")).toHaveCount(6);
  for (const name of moduleNames) await expect(navigation.getByRole("link", { name })).toBeVisible();
  await page.getByRole("button", { name: "关闭产品导航" }).click();

  const portal = page.locator('[data-module-card="portal"]');
  await expect(portal).toHaveAccessibleName("Portal：正常");
  await expect(portal.getByText("Readiness", { exact: true })).toBeVisible();
  await expect(portal.getByText("入口健康", { exact: true })).toBeVisible();
  await expect(portal.getByLabel("Readiness：ready")).toBeVisible();

  const width = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
});

test("loading scenario marks all six modules busy without fake metrics", async ({ page }) => {
  await page.goto("/?scenario=loading");
  await expect(page.locator("section[aria-busy='true']")).toBeVisible();
  await expect(page.locator("[data-state='loading']")).toHaveCount(6);
  await expect(page.locator(".metric-tile")).toHaveCount(0);
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
  await expect(page.locator(".metric-tile")).toHaveCount(0);
  await login.click();

  await expect(page).toHaveURL(/\/\?tab=inbox$/);
  await expect(page.getByText("权限已验证", { exact: true })).toBeVisible();
  await expect(page.locator(".metric-tile")).not.toHaveCount(0);
  expect((await context.cookies()).some((cookie) => cookie.name === "henukit_console_e2e" && cookie.httpOnly)).toBe(true);
});
