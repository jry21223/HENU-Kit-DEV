import { expect, test } from "@playwright/test";
import { mockGuestGateway, mockSignedInGateway } from "./support/gateway";

/**
 * 互助平台开放前的降级（#568）：保留 /campus 浏览页；二级导航不再给出未开放的「我的交易」
 * 「发布」入口，直接访问这两个路由时说明「即将开放」，而不是空壳页或登录跳转；首页第 04 模块
 * 标明「即将开放」；/campus 取数失败时不重复请求同一批接口。
 */

test.use({ contextOptions: { reducedMotion: "reduce" } });

for (const [path, heading] of [
  ["/campus/deals", "我的交易"],
  ["/campus/publish", "发布单子"],
] as const) {
  test(`${path} says it is coming soon, for guests`, async ({ page }) => {
    await mockGuestGateway(page);
    await page.goto(path);
    await expect(page).toHaveURL(new RegExp(`${path}$`));
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1, name: heading })).toBeVisible();
    await expect(main.getByText("即将开放").first()).toBeVisible();
    await expect(main.getByRole("textbox")).toHaveCount(0);
    await expect(main.getByRole("link", { name: "去浏览互助信息" })).toHaveAttribute("href", "/campus");
  });
}

test("signed-in users see the same notice instead of the empty publish form", async ({ page }) => {
  await mockSignedInGateway(page);
  await page.goto("/campus/publish");
  await expect(page.getByRole("main").getByText("即将开放").first()).toBeVisible();
  await expect(page.getByRole("main").getByRole("textbox")).toHaveCount(0);
  await expect(page.getByRole("main").getByRole("button", { name: /发布/ })).toHaveCount(0);
});

test("the campus sub-site header lists no unopened entries", async ({ page }) => {
  await mockGuestGateway(page);
  await page.goto("/campus");
  const header = page.locator("header").first();
  await expect(header.getByRole("link", { name: /我的交易/ })).toHaveCount(0);
  await expect(header.getByRole("link", { name: /发布/ })).toHaveCount(0);
  await expect(header.getByText(/M-0[23]/)).toHaveCount(0);
  // 浏览页本身仍在。
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: "互助平台" })).toBeVisible();
});

test("the homepage marks module 04 as coming soon", async ({ page }) => {
  await mockGuestGateway(page);
  await page.goto("/");
  const section = page.locator("section").filter({ has: page.getByRole("heading", { level: 2, name: "互助平台" }) });
  await expect(section.getByText("即将开放").first()).toBeAttached();
});

test("a failed campus list is requested once per attempt, not twice", async ({ page }) => {
  const calls: string[] = [];
  await mockGuestGateway(page);
  await page.route("**/api/v1/campus/**", (route) => {
    calls.push(new URL(route.request().url()).pathname);
    return route.fulfill({ status: 503, json: { error: "upstream_unavailable", request_id: "req_campus_once" } });
  });
  await page.goto("/campus");
  await expect(page.getByRole("main").getByRole("alert")).toContainText("互助信息暂时无法加载");
  await page.waitForTimeout(500);
  expect(calls.filter((p) => p === "/api/v1/campus/items")).toHaveLength(1);
  expect(calls.filter((p) => p === "/api/v1/campus/categories")).toHaveLength(1);
});
