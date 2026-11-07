import { expect, test, type Locator, type Page } from "@playwright/test";

// Presentation fixtures only. Owner readiness/auth behavior is separately
// exercised through Library and Gateway HTTP tests with isolated Redis.
const session = { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions: ["library.read", "library.manage", "library.review"], scopes: [{ kind: "product", product_code: "library" }], verified_at: "2026-11-07T18:00:00Z" }, expires_at: "2026-11-07T19:00:00Z" };
const readiness = { management: "not_ready", reason: "management_migration_pending", capabilities: { workspace: false, commands: false, operation_results: false }, checked_at: "2026-11-07T18:00:00Z" };
let readinessStatus = 200;
let readinessData: unknown = readiness;
let libraryRequests: Array<{ method: string; path: string }> = [];

async function tabTo(page: Page, target: Locator) {
  for (let step = 0; step < 30; step += 1) {
    if (await target.evaluate((element) => element === document.activeElement)) return;
    await page.keyboard.press("Tab");
  }
  await expect(target).toBeFocused();
}

test.beforeEach(async ({ page }) => {
  readinessStatus = 200;
  readinessData = readiness;
  libraryRequests = [];
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ data: session, request_id: "req_library_session" }) }));
  await page.route("**/api/v1/library**", (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    libraryRequests.push({ method: request.method(), path });
    if (path === "/api/v1/library/readiness" && request.method() === "GET") return route.fulfill({ status: readinessStatus, contentType: "application/json", body: JSON.stringify({ data: readinessData, request_id: "req_library_readiness" }) });
    return route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: { code: "LIBRARY_COMMANDS_UNAVAILABLE", message: "legacy Study API adapter removed (ADR-0037); catalog migration (T1) pending" }, request_id: "req_no_management" }) });
  });
});

for (const viewport of [{ name: "desktop", width: 1440, height: 1000 }, { name: "375px", width: 375, height: 812 }, { name: "390px", width: 390, height: 844 }]) {
  test(`${viewport.name} Library shows confirmed readiness without forms, fake zeroes or replay`, async ({ page }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.addInitScript(() => sessionStorage.setItem("henukit.library.pending-operation", JSON.stringify({ kind: "course_create", key: "idem_library_old_pending", input: { kind: "course_create", payload: {} }, success: "课程已创建。" })));
    await page.goto("/library");
    await expect(page.getByRole("heading", { name: "资料管理尚未就绪" })).toBeVisible();
    await expect(page.locator("form")).toHaveCount(0);
    await expect(page.locator(".operation-summary-grid")).toHaveCount(0);
    await expect(page.getByText(/同步生效|ADR-0037|\bT1\b|暂无课程|暂无资料|按原请求重试/)).toHaveCount(0);
    await expect(page.getByRole("link", { name: "打开公开资料目录" })).toHaveAttribute("href", "https://henukit.cn/library");
    await expect(page.getByText("状态确认于", { exact: false })).toContainText("北京时间");
    await expect(page.getByRole("button", { name: "重新确认" })).toHaveCount(0);
    const publicLink = page.getByRole("link", { name: "打开公开资料目录" });
    await tabTo(page, publicLink);
    await expect(publicLink).toBeFocused();
    await page.screenshot({ path: testInfo.outputPath("library-public-link-keyboard-focus.png") });
    await page.keyboard.press("Shift+Tab");
    await expect(publicLink).not.toBeFocused();
    await page.keyboard.press("Tab");
    await expect(publicLink).toBeFocused();
    await page.reload();
    await expect(page.getByRole("heading", { name: "资料管理尚未就绪" })).toBeVisible();
    expect(libraryRequests.length).toBeGreaterThanOrEqual(2);
    expect(libraryRequests.every(({ method, path }) => method === "GET" && path === "/api/v1/library/readiness")).toBe(true);
    const width = await page.evaluate(() => ({ inner: window.innerWidth, client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }));
    expect(width.inner).toBe(viewport.width);
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("Library keyboard retry announces loading, keeps failure focus and focuses the available result", async ({ page }) => {
  readinessStatus = 503;
  await page.goto("/library");
  const retry = page.getByRole("button", { name: "重新加载" });
  await expect(retry).toBeVisible();
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => { release = resolve; });
  let retryRequests = 0;
  let nextStatus = 503;
  await page.route("**/api/v1/library/readiness", async (route) => {
    retryRequests += 1;
    libraryRequests.push({ method: route.request().method(), path: new URL(route.request().url()).pathname });
    await gate;
    await route.fulfill({ status: nextStatus, contentType: "application/json", body: JSON.stringify({ data: readiness, request_id: "req_keyboard_retry" }) });
  });
  await tabTo(page, retry);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status")).toContainText("正在确认资料管理能力");
  await expect(retry).toBeFocused();
  await expect(retry).toHaveAttribute("aria-disabled", "true");
  await page.keyboard.press("Enter");
  await expect.poll(() => retryRequests).toBe(1);
  release();
  await expect(page.getByRole("status")).toContainText("暂时无法确认资料管理能力");
  await expect(retry).toBeFocused();
  await expect(retry).toHaveAttribute("aria-disabled", "false");
  nextStatus = 200;
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "资料管理尚未就绪" })).toBeVisible();
  await expect(page.getByRole("link", { name: "打开公开资料目录" })).toBeFocused();
  expect(retryRequests).toBe(2);
  expect(libraryRequests.every(({ method, path }) => method === "GET" && path === "/api/v1/library/readiness")).toBe(true);
});

for (const status of [404, 503]) {
  test(`Library readiness HTTP ${status} remains unconfirmed`, async ({ page }) => {
    readinessStatus = status;
    await page.goto("/library");
    await expect(page.getByText("暂时无法确认资料管理能力，请稍后重新加载。")).toBeVisible();
    await expect(page.getByRole("heading", { name: "资料管理尚未就绪" })).toHaveCount(0);
    await expect(page.locator("form")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "重新加载" })).toBeVisible();
  });
}

test("Library cannot infer readiness from a legacy empty HTTP 200", async ({ page }) => {
  readinessData = { status: "partial", status_message: "legacy Study API adapter removed (ADR-0037); catalog migration (T1) pending", degraded: true, courses: [], materials: [], downloads: [], submissions: [], corrections: [], generated_at: "2026-11-07T18:00:00Z" };
  await page.goto("/library");
  await expect(page.getByText("暂时无法确认资料管理能力，请稍后重新加载。")).toBeVisible();
  await expect(page.locator("form")).toHaveCount(0);
  await expect(page.getByText(/ADR-0037|暂无课程|暂无资料/)).toHaveCount(0);
});

test("Library expiry offers the existing constrained login return", async ({ page }) => {
  readinessStatus = 401;
  await page.goto("/library");
  await expect(page.getByText(/登录状态已过期|重新登录/).first()).toBeVisible();
  await expect(page.getByRole("link", { name: /登录/ })).toHaveAttribute("href", /\/api\/v1\/auth\/login\?return_to=%2Flibrary/);
  await expect(page.locator("form")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "资料管理尚未就绪" })).toHaveCount(0);
});

test("Library permission denial reveals no management or catalog data", async ({ page }) => {
  readinessStatus = 403;
  await page.goto("/library");
  await expect(page.getByText("当前账户没有资料库查看权限，请联系管理员。")).toBeVisible();
  await expect(page.locator("form")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "打开公开资料目录" })).toHaveCount(0);
});
