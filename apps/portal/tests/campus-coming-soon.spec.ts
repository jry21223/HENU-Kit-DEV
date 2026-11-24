import { expect, test } from "@playwright/test";
import { mockGuestGateway, mockSignedInGateway } from "./support/gateway";

/**
 * 互助平台二级入口路由（/campus/publish、/campus/deals）：承接旧链接与书签，
 * 如实说明已在互助广场与单子详情页提供发单、联系方式簿与双方同意后的联系方式交换，
 * 不再提示「即将开放」，并明确不提供在线资金托管/结算；同时校验 /campus 取数失败时不重复请求。
 */

test.use({ contextOptions: { reducedMotion: "reduce" } });

for (const entry of [
  {
    path: "/campus/deals",
    heading: "我的交易",
    ctaLabel: "前往互助平台",
    expectedTexts: ["联系方式簿", "单子详情页", "不提供在线资金托管或支付结算"],
  },
  {
    path: "/campus/publish",
    heading: "发布单子",
    ctaLabel: "前往互助平台发布",
    expectedTexts: ["发布新单子", "最多 6 张图片", "联系方式簿", "不提供在线资金托管或支付结算"],
  },
] as const) {
  test(`${entry.path} guides guests to /campus without contradictory coming-soon copy`, async ({ page }) => {
    await mockGuestGateway(page);
    await page.goto(entry.path);
    await expect(page).toHaveURL(new RegExp(`${entry.path}$`));
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1, name: entry.heading })).toBeVisible();
    await expect(main).not.toContainText("即将开放");
    for (const text of entry.expectedTexts) {
      await expect(main).toContainText(text);
    }
    await expect(main.getByRole("textbox")).toHaveCount(0);
    await expect(main.getByRole("link", { name: entry.ctaLabel })).toHaveAttribute("href", "/campus");
  });
}

test("signed-in users on /campus/publish see the entry guidance to /campus", async ({ page }) => {
  await mockSignedInGateway(page);
  await page.goto("/campus/publish");
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name: "发布单子" })).toBeVisible();
  await expect(main).not.toContainText("即将开放");
  await expect(main).toContainText("发布新单子");
  await expect(main.getByRole("textbox")).toHaveCount(0);
  await expect(main.getByRole("link", { name: "前往互助平台发布" })).toHaveAttribute("href", "/campus");
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

test("the homepage module 04 reflects Campus publishing, two-way contact exchange, and no online custody", async ({ page }) => {
  await mockGuestGateway(page);
  await page.goto("/");
  const section = page.locator("section").filter({ has: page.getByRole("heading", { level: 2, name: "互助平台" }) });
  await expect(section).not.toContainText("即将开放");
  await expect(section).not.toContainText("暂未开放");
  await expect(section).toContainText("双向同意后换联系方式 · 无在线结算");
  await expect(section).toContainText("不提供在线支付与资金托管，金额仅供沟通参考。");
  await expect(section.getByRole("link", { name: "进入互助平台" })).toHaveAttribute("href", "/campus");
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
