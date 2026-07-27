import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

const MODULE_ROUTES = ["/campus", "/food", "/library", "/practice"] as const;
const VIEWPORTS = [360, 390, 430, 768] as const;

async function expectNoPageOverflow(page: Page) {
  const metrics = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(metrics.scrollWidth, `page overflow at ${metrics.clientWidth}px`).toBeLessThanOrEqual(
    metrics.clientWidth + 1,
  );
}

test.describe("Portal mobile layout", () => {
  for (const route of MODULE_ROUTES) {
    test(`${route} stays within the viewport`, async ({ page }) => {
      for (const width of VIEWPORTS) {
        await page.setViewportSize({ width, height: 800 });
        await page.goto(route, { waitUntil: "domcontentloaded" });
        await expect(page.locator("body")).toBeVisible();
        await expectNoPageOverflow(page);

        const header = page.locator("header").first();
        await expect(header).toBeVisible();
        const headerBox = await header.boundingBox();
        expect(headerBox?.x ?? -1).toBeGreaterThanOrEqual(0);
        expect((headerBox?.x ?? 0) + (headerBox?.width ?? width)).toBeLessThanOrEqual(width + 1);

        const navLinks = header.locator("nav a:visible");
        for (let index = 0; index < (await navLinks.count()); index += 1) {
          const linkBox = await navLinks.nth(index).boundingBox();
          expect(linkBox?.width ?? 0).toBeGreaterThan(0);
          expect(linkBox?.y ?? -1).toBeGreaterThanOrEqual((headerBox?.y ?? 0) - 1);
          expect((linkBox?.y ?? 0) + (linkBox?.height ?? 0)).toBeLessThanOrEqual(
            (headerBox?.y ?? 0) + (headerBox?.height ?? 0) + 1,
          );
        }
      }
    });
  }

  for (const route of ["/account/login", "/account/recover"] as const) {
    test(`${route} keeps form actions reachable`, async ({ page }) => {
      for (const width of VIEWPORTS) {
        await page.setViewportSize({ width, height: 800 });
        await page.goto(route, { waitUntil: "domcontentloaded" });
        await expect(page.locator("body")).toBeVisible();
        await expectNoPageOverflow(page);

        const card = page.locator("[data-enter]");
        const cardBox = await card.boundingBox();
        expect(cardBox?.x ?? -1).toBeGreaterThanOrEqual(0);
        expect((cardBox?.x ?? 0) + (cardBox?.width ?? width)).toBeLessThanOrEqual(width + 1);

        if (route === "/account/login") {
          const action = page.getByRole("button", { name: "发送验证码" });
          const actionBox = await action.boundingBox();
          expect(actionBox?.x ?? -1).toBeGreaterThanOrEqual(0);
          expect((actionBox?.x ?? 0) + (actionBox?.width ?? width)).toBeLessThanOrEqual(width + 1);
        }
      }
    });
  }
});

test.describe("Practice hero mastery data states", () => {
  test("renders authenticated practice stats", async ({ page }) => {
    await page.route("**/api/v1/practice/stats", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          totalQuestions: 42,
          accuracy: 75,
          streakDays: 11,
          beatPercent: 60,
          mastery: [{ label: "数据结构", value: 81 }],
          weakTop5: [],
          request_id: "test-success",
        }),
      }),
    );

    await page.goto("/practice");
    await expect(page.getByText("数据结构", { exact: true })).toBeVisible();
    await expect(page.getByText("81%", { exact: true })).toBeVisible();
    await expect(page.getByText("42 题", { exact: true })).toBeVisible();
  });

  test("distinguishes empty stats from an unavailable source", async ({ page }) => {
    await page.route("**/api/v1/practice/stats", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          totalQuestions: 0,
          accuracy: 0,
          streakDays: 0,
          beatPercent: 0,
          mastery: [],
          weakTop5: [],
          request_id: "test-empty",
        }),
      }),
    );
    await page.goto("/practice");
    await expect(page.getByText("暂无掌握度数据", { exact: true })).toBeVisible();

    await page.route("**/api/v1/practice/stats", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: "stats_unavailable",
          request_id: "test-error",
        }),
      }),
    );
    await page.reload();
    await expect(page.getByText("掌握度数据尚未接入", { exact: true })).toBeVisible();
  });
});
