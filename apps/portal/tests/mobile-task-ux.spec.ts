import { expect, test } from "@playwright/test";
import { mockGuestGateway } from "./support/gateway";
import { libraryCountsFor } from "./support/library-counts";

test.use({ viewport: { width: 375, height: 812 }, contextOptions: { reducedMotion: "reduce" } });

test("资料目录分批显示，筛选全目录，详情返回保留筛选与阅读位置", async ({ page }) => {
  const materials = Array.from({ length: 60 }, (_, index) => ({
    id: `mobile-material-${index + 1}`,
    type: index < 50 ? "note" : "exam",
    subject: index < 50 ? "高等数学" : "线性代数",
    title: `${index < 50 ? "复习笔记" : "往年试卷"}${index + 1}`,
    author: "资料库收录", intro: "", toc: [], pages: [], price: 0,
    previewPages: 0, downloads: index + 1, downloadAvailable: true, fileSize: 1024,
  }));
  let catalogReads = 0;
  await page.route("**/api/v1/library/materials", async (route) => {
    catalogReads += 1;
    // 超过旧上级返回的 1.5 秒窗口，覆盖慢网下目录到达后的恢复。
    if (catalogReads > 1) await new Promise((resolve) => setTimeout(resolve, 2000));
    await route.fulfill({ json: {
      materials,
      statistics: { materialCount: 60, downloadStarts: 200, releaseId: "mobile-catalog", countingSince: "2026-08-11T00:00:00Z", asOf: "2026-10-23T00:00:00Z" },
      request_id: "mobile-catalog",
    } });
  });
  await page.route("**/api/v1/library/materials/*", (route) => route.fulfill({
    json: { material: materials.find((material) => route.request().url().endsWith(`/${material.id}`)), request_id: "mobile-detail" },
  }));
  await page.goto("/library");
  const cards = page.locator('main a[href^="/library/item/"]');
  await expect(cards).toHaveCount(24);
  await expect(page.getByText("已显示 24 / 60 份", { exact: true })).toBeVisible();
  const secondTitle = cards.nth(1).getByRole("heading");
  await expect(secondTitle).toBeInViewport({ ratio: 1 });

  await page.getByLabel("搜索资料").fill("复习");
  await page.getByRole("combobox", { name: "按科目筛选" }).selectOption("高等数学");
  await page.getByRole("button", { name: "笔记总结", exact: true }).click();
  await expect(page.getByText("已显示 24 / 50 份", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "加载更多资料" }).click();
  await expect(cards).toHaveCount(48);
  const chosen = cards.nth(35);
  await chosen.scrollIntoViewIfNeeded();
  const offset = await page.evaluate(() => window.scrollY);
  await chosen.click();
  await expect(page.getByRole("heading", { level: 1, name: "复习笔记36", exact: true })).toBeVisible();
  await page.locator('header a[href="/library"]').click();
  await expect(page.getByLabel("搜索资料")).toHaveValue("复习");
  await expect(page.getByRole("combobox", { name: "按科目筛选" })).toHaveValue("高等数学");
  await expect(page.getByRole("button", { name: "笔记总结", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(cards).toHaveCount(48);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(offset - 100);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeLessThan(offset + 100);

  // 历史返回的数据仍在加载时，主动操作搜索后，稍后的补位不能把人拉回旧位置。
  await cards.nth(35).click();
  await expect(page.getByRole("heading", { level: 1, name: "复习笔记36", exact: true })).toBeVisible();
  await page.goBack({ waitUntil: "domcontentloaded" });
  await page.getByLabel("搜索资料").press("End");
  await expect(cards).toHaveCount(48);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeLessThan(1000);

  await page.getByLabel("搜索资料").fill("不存在的课程");
  await expect(page.getByText("无匹配资料", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选" }).click();
  await expect(cards).toHaveCount(24);
  await expect(page.getByText("已显示 24 / 60 份", { exact: true })).toBeVisible();
  await expect(page.getByText("累计下载", { exact: true }).locator("..")).toContainText("200");
  await page.getByLabel("搜索资料").fill("往年试卷60");
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("往年试卷60");
});

test("首页解析正文在滚入前后与减少动态模式切换后始终保留", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await mockGuestGateway(page);
  await page.goto("/");
  await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");
  const explanation = page.locator("[data-typewriter]");
  await expect(explanation).toContainText("【参考答案】1");
  await explanation.scrollIntoViewIfNeeded();
  await expect(explanation).toBeVisible();
  await expect(explanation).toContainText("注意 x 需以弧度计");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(explanation).toContainText("【参考答案】1");
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await expect(explanation).toContainText("注意 x 需以弧度计");
});

test("减少动态效果时，等待数据的骨架不再循环闪动", async ({ page }) => {
  await mockGuestGateway(page);
  let release!: () => void;
  const waiting = new Promise<void>((resolve) => { release = resolve; });
  await page.route("**/api/v1/library/material-counts", async (route) => {
    await waiting;
    await route.fulfill({ json: libraryCountsFor([]) });
  });
  try {
    await page.goto("/");
    const skeletons = page.locator(".animate-pulse");
    await expect(skeletons.first()).toBeAttached();
    const animations = await skeletons.evaluateAll((elements) => elements.map((element) => getComputedStyle(element).animationName));
    expect(animations.every((name) => name === "none")).toBe(true);
    await page.emulateMedia({ reducedMotion: "no-preference" });
    await expect(skeletons.first()).not.toHaveCSS("animation-name", "none");
    await page.emulateMedia({ reducedMotion: "reduce" });
    await expect(skeletons.first()).toHaveCSS("animation-name", "none");
  } finally {
    release();
  }
});

test("美食首屏可选校区并看到完整首店，同店名不重复且筛选有效", async ({ page }) => {
  await mockGuestGateway(page);
  const posts = ["minglun", "jinming"].map((campus, index) => ({
    id: `mobile-food-${index}`, campus, title: index ? "金明米饭" : "明伦面馆",
    shop: { name: index ? "金明米饭" : "明伦面馆" },
    excerpt: "同学推荐", blocks: [{ type: "p", text: "用餐体验" }], author: "同学",
    likes: 1, stars: 0, tags: ["夯"], time: "10-23", hidden: false, images: [],
  }));
  await page.route("**/api/v1/food/posts", (route) => route.fulfill({ json: { posts, request_id: "mobile-food" } }));
  await page.goto("/food");
  await expect(page.getByRole("button", { name: "明伦校区", exact: true })).toBeInViewport({ ratio: 1 });
  const firstShop = page.locator('a[href="/food/post/mobile-food-0"]');
  await expect(firstShop.getByText("明伦面馆", { exact: true })).toBeVisible();
  await expect(firstShop).toBeInViewport({ ratio: 1 });
  expect((await firstShop.innerText()).match(/明伦面馆/g)).toHaveLength(1);
  await page.getByRole("button", { name: "金明校区", exact: true }).click();
  await expect(firstShop).toHaveCount(0);
  await expect(page.locator('a[href="/food/post/mobile-food-1"]')).toBeVisible();
  await expect(page.getByRole("button", { name: "金明校区", exact: true })).toHaveAttribute("aria-pressed", "true");
});

test("五个刷题中文任务在手机导航中完整可见", async ({ page }) => {
  test.skip(process.env.PLAYWRIGHT_ENABLE_QUIZCRAFT_V2_READS !== "1", "五项导航在 V2 读取开启的测试组验证");
  await mockGuestGateway(page);
  for (const width of [375, 390]) {
    await page.setViewportSize({ width, height: 812 });
    for (const route of ["/practice", "/practice/quiz", "/practice/favorites", "/practice/stats", "/practice/leaderboard"]) {
      await page.goto(route);
      const nav = page.locator("header nav");
      for (const [href, label] of [
        ["/practice", "题库"], ["/practice/quiz", "刷题"], ["/practice/favorites", "收藏夹"],
        ["/practice/stats", "数据"], ["/practice/leaderboard", "排行榜"],
      ]) {
        const link = href === "/practice/quiz"
          ? nav.locator('a[href="/practice/quiz"], [data-tab-unavailable], span[aria-current="page"]').filter({ hasText: "刷题" })
          : nav.locator(`a[href="${href}"]`);
        await expect(link).toContainText(label);
        const bounds = await link.boundingBox();
        expect(bounds?.x).toBeGreaterThanOrEqual(0);
        expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
        expect(bounds!.height).toBeGreaterThanOrEqual(44);
      }
    }
  }
});
