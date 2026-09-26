import { expect, test } from "@playwright/test";
import type { Locator, Page } from "@playwright/test";

const MODULE_ROUTES = ["/campus", "/career", "/food", "/library", "/practice"] as const;
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

const CAMPUS_ITEMS = [
  {
    id: "campus-express", type: "help", category: "express", title: "代取快递到南门", desc: "两件小包裹",
    price: 3, seller: "同学甲", credit: 0, dealsDone: 0, wants: 0, place: "明伦校区", status: "open", time: "2026-09-20",
  },
];

async function bottomEdge(locator: Locator) {
  const box = await locator.boundingBox();
  return (box?.y ?? Infinity) + (box?.height ?? 0);
}

/** 首页 hero 之后的整屏模块里，min-height 被撑到一整屏的元素。 */
async function forcedScreenHeights(page: Page) {
  return page.locator(".snap-screen").evaluateAll((sections) =>
    sections.slice(1).flatMap((section) =>
      [section, ...section.querySelectorAll("*")]
        .filter((element) => parseFloat(getComputedStyle(element).minHeight) >= window.innerHeight)
        .map((element) => `${section.tagName.toLowerCase()} ${element.className}`)
    )
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

  test("/campus shows search and the first item in the first screen at 390×844 (#542)", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.route("**/api/v1/campus/items", (route) =>
      route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_campus_first_screen" } })
    );
    await page.route("**/api/v1/campus/categories", (route) =>
      route.fulfill({ json: { categories: [], request_id: "req_campus_first_screen_categories" } })
    );
    await page.goto("/campus", { waitUntil: "domcontentloaded" });

    const search = page.getByPlaceholder("搜索：快递 / 键盘 / 占座");
    await expect(search).toBeVisible();
    expect(await bottomEdge(search)).toBeLessThan(844);
    await expect(page.getByRole("heading", { name: "代取快递到南门", exact: true })).toBeInViewport({ ratio: 1 });
  });

  // 目录开关关闭时内容区是「暂无题库」；开关开启时的题库卡片由 quizcraft-catalog.spec.ts 检查。
  test("/practice shows search and the content block in the first screen at 390×844 (#542)", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/practice", { waitUntil: "domcontentloaded" });

    const search = page.getByPlaceholder("如：数据结构 / 高等数学");
    await expect(search).toBeVisible();
    expect(await bottomEdge(search)).toBeLessThan(844);
    await expect(page.getByText("暂无题库", { exact: true })).toBeInViewport({ ratio: 1 });
  });

  // 整屏吸附只在 md+ 启用（snap-scroll.tsx）；手机上模块按内容高度排，不留整屏空白。
  test("homepage modules fill a screen only at md+ (#542)", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect(page.locator(".snap-screen")).toHaveCount(7);
    expect(await forcedScreenHeights(page), "module forced to a full screen at 390px").toEqual([]);

    await page.setViewportSize({ width: 1024, height: 800 });
    const heights = await page.locator(".snap-screen").evaluateAll((sections) =>
      sections.slice(1).map((section) => section.getBoundingClientRect().height)
    );
    for (const height of heights) expect(height).toBeGreaterThanOrEqual(800);
  });

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
