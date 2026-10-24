import { expect, test } from "@playwright/test";
import { mockGuestGateway } from "./support/gateway";
import { libraryCountsFor } from "./support/library-counts";

test.describe("asynchronous homepage animation targets", () => {
  test.use({ viewport: { width: 375, height: 812 }, contextOptions: { reducedMotion: "no-preference" } });

  test("waits for real cards and rows before creating their entrance animations", async ({ page }) => {
    const warnings: string[] = [];
    page.on("console", (message) => {
      if (message.type() === "warning" && message.text().includes("GSAP target")) warnings.push(message.text());
    });
    await mockGuestGateway(page);
    let release!: () => void;
    const waiting = new Promise<void>((resolve) => { release = resolve; });
    await page.route("**/api/v1/library/material-counts", async (route) => {
      await waiting;
      await route.fulfill({ json: libraryCountsFor([{ type: "note" }, { type: "exam" }]) });
    });
    await page.route("**/api/v1/library/courses", (route) => route.fulfill({ json: { courses: [], request_id: "animation-courses" } }));
    await page.route("**/api/v1/food/posts", async (route) => {
      await waiting;
      await route.fulfill({ json: { posts: [{
        id: "animation-food", campus: "minglun", title: "校内食堂", excerpt: "日常午餐。",
        blocks: [], author: "学生", likes: 0, stars: 0, tags: ["夯"],
        shop: { name: "校内食堂" }, time: "10-24", hidden: false, images: [],
      }], request_id: "animation-food" } });
    });
    try {
      await page.goto("/");
      await expect(page.locator("html")).toHaveAttribute("data-scroll-memory", "ready");
      await expect(page.getByText("榜单加载中…", { exact: true })).toBeVisible();
      await expect(page.locator("[data-lib-card], [data-rank-row]")).toHaveCount(0);
      expect(warnings).toEqual([]);
      release();
      const cards = page.locator("[data-lib-card]");
      const rows = page.locator("[data-rank-row]");
      await expect(cards).toHaveCount(3);
      await expect(rows).toHaveCount(1);
      await cards.first().scrollIntoViewIfNeeded();
      await expect(cards.first()).toHaveCSS("opacity", "1");
      const track = cards.first().locator("..");
      const transform = await track.evaluate((element) => getComputedStyle(element).transform);
      await expect.poll(() => track.evaluate((element) => getComputedStyle(element).transform)).not.toBe(transform);
      await rows.first().scrollIntoViewIfNeeded();
      await expect(rows.first()).toHaveCSS("opacity", "1");
      await expect(rows.first().getByRole("link", { name: "校内食堂", exact: true })).toBeVisible();
      await page.emulateMedia({ reducedMotion: "reduce" });
      await expect(rows.first()).toHaveCSS("opacity", "1");
      await page.emulateMedia({ reducedMotion: "no-preference" });
      await rows.first().scrollIntoViewIfNeeded();
      await expect(rows.first()).toHaveCSS("opacity", "1");
      expect(warnings).toEqual([]);
    } finally {
      release();
    }
  });

  for (const outcome of ["empty", "unavailable"] as const) {
    test(`creates no empty-target animation for ${outcome} data or motion changes`, async ({ page }) => {
      const warnings: string[] = [];
      page.on("console", (message) => {
        if (message.type() === "warning" && message.text().includes("GSAP target")) warnings.push(message.text());
      });
      await mockGuestGateway(page);
      if (outcome === "empty") {
        await page.route("**/api/v1/library/material-counts", (route) => route.fulfill({ json: libraryCountsFor([]) }));
        await page.route("**/api/v1/library/courses", (route) => route.fulfill({ json: { courses: [], request_id: "empty-courses" } }));
        await page.route("**/api/v1/food/posts", (route) => route.fulfill({ json: { posts: [], request_id: "empty-food" } }));
      }
      await page.goto("/");
      await expect(page.getByText(outcome === "empty" ? "还没有上榜条目。" : "榜单暂时加载不出来，请稍后刷新试试。", { exact: true })).toBeVisible();
      await page.waitForLoadState("networkidle");
      await expect(page.locator("[data-lib-card], [data-rank-row]")).toHaveCount(0);
      await page.emulateMedia({ reducedMotion: "reduce" });
      await page.emulateMedia({ reducedMotion: "no-preference" });
      expect(warnings).toEqual([]);
    });
  }

  test("restores the card sweep after a failed read is retried successfully", async ({ page }) => {
    const warnings: string[] = [];
    page.on("console", (message) => {
      if (message.type() === "warning" && message.text().includes("GSAP target")) warnings.push(message.text());
    });
    await mockGuestGateway(page);
    await page.goto("/");
    const library = page.locator("section").filter({ has: page.getByRole("heading", { level: 2, name: "资料库", exact: true }) });
    await expect(library.getByRole("alert")).toBeVisible();
    let release!: () => void;
    const waiting = new Promise<void>((resolve) => { release = resolve; });
    await page.route("**/api/v1/library/material-counts", async (route) => {
      await waiting;
      await route.fulfill({ json: libraryCountsFor([{ type: "note" }, { type: "exam" }]) });
    });
    await page.route("**/api/v1/library/courses", (route) => route.fulfill({ json: { courses: [], request_id: "retry-courses" } }));
    try {
      await library.getByRole("button", { name: "重试", exact: true }).click();
      await expect(library.getByRole("alert")).toHaveCount(0);
      await expect(library.locator("[data-lib-card]")).toHaveCount(0);
      release();
      const cards = library.locator("[data-lib-card]");
      await expect(cards).toHaveCount(3);
      await cards.first().scrollIntoViewIfNeeded();
      await expect(cards.first()).toHaveCSS("opacity", "1");
      const track = cards.first().locator("..");
      const transform = await track.evaluate((element) => getComputedStyle(element).transform);
      await expect.poll(() => track.evaluate((element) => getComputedStyle(element).transform)).not.toBe(transform);
      await page.emulateMedia({ reducedMotion: "reduce" });
      await expect(cards.first()).toHaveCSS("opacity", "1");
      await page.emulateMedia({ reducedMotion: "no-preference" });
      await cards.first().scrollIntoViewIfNeeded();
      await expect(cards.first()).toHaveCSS("opacity", "1");
      const restoredTransform = await track.evaluate((element) => getComputedStyle(element).transform);
      await expect.poll(() => track.evaluate((element) => getComputedStyle(element).transform)).not.toBe(restoredTransform);
      expect(warnings).toEqual([]);
    } finally {
      release();
    }
  });
});

test.describe("Homepage task entry", () => {
  test.use({ viewport: { width: 390, height: 800 }, contextOptions: { reducedMotion: "reduce" } });

  for (const [name, route] of [
    ["找资料", "/library"],
    ["开始刷题", "/practice"],
    ["看岗位", "/career"],
  ] as const) {
    test(`${name} is available without scrolling and opens by keyboard`, async ({ page }) => {
      await page.goto("/");
      const entry = page.getByRole("navigation", { name: "常用功能" }).getByRole("link", { name, exact: true });

      await expect(entry).toBeInViewport({ ratio: 1 });
      await expect(entry).toHaveAttribute("href", route);
      const initialScroll = await page.evaluate(() => window.scrollY);
      expect(initialScroll).toBe(0);

      for (let step = 0; step < 12; step += 1) {
        await page.keyboard.press("Tab");
        if (await entry.evaluate((element) => element === document.activeElement)) break;
      }
      await expect(entry).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(page).toHaveURL(new RegExp(`${route}$`));
    });
  }

  test("five module descriptions and Campus entry explain what is open", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText(/五个模块，陪你处理校园日常/)).toBeVisible();
    await page.getByRole("button", { name: "打开菜单" }).click();
    for (const [name, route] of [
      ["资料库", "/library"],
      ["智能刷题", "/practice"],
      ["美食榜", "/food"],
      ["互助平台", "/campus"],
      ["求职雷达", "/career"],
    ] as const) {
      await expect(page.locator("header").getByRole("link", { name: new RegExp(name) })).toHaveAttribute("href", route);
    }
    await page.getByRole("button", { name: "关闭菜单" }).click();
    await expect(page.getByText("发布、接单和结算暂未开放。", { exact: true })).toBeVisible();
    await expect(page.getByText(/实名认证|发单有人接|全覆盖|真实订单即将上线|互助接单/)).toHaveCount(0);

    await page.route("**/api/v1/campus/items", (route) => route.fulfill({ json: { items: [], request_id: "campus-empty" } }));
    await page.route("**/api/v1/campus/categories", (route) => route.fulfill({ json: { categories: [], request_id: "campus-categories" } }));
    await page.goto("/campus");
    await expect(page.getByText("可浏览互助与闲置信息；发布、接单和结算暂未开放。", { exact: true })).toBeVisible();
    await expect(page.locator('meta[name="description"]')).toHaveAttribute("content", /发布、接单和结算暂未开放/);
    await expect(page.getByText(/发单有人接|实名认证|即将上线/)).toHaveCount(0);
  });

  test("Practice section describes only delivered capabilities", async ({ page }) => {
    await page.goto("/");
    // 首屏 marquee 等其他首页位置同样不宣传 AI 刷题 / 推题。
    await expect(page.locator("main")).not.toContainText(/AI\s*(智能)?(刷题|推题)/);

    const practice = page.locator("section").filter({ has: page.getByRole("heading", { level: 2, name: /智能刷题/ }) });
    await expect(practice).toHaveCount(1);

    // AI 推题尚未上线（#530）：不宣传推题、自动归因、每题讲解或按知识点的掌握度。
    for (const claim of ["AI 按你的薄弱知识点推题", "按知识点智能推题", "错题自动归因", "每道错题都配", "知识点", "曲线", "每周更新"]) {
      await expect(practice).not.toContainText(claim);
    }
    await expect(practice).not.toContainText(/\bAI\b/);

    await expect(practice).toContainText("随机、难题、章节、收藏");
    await expect(practice).toContainText("题库掌握度");
    // 右侧演示对应答题页的解析区，明确标为示例，并说明解析来自题库、部分题目没有。
    await expect(practice).toContainText("解析示例");
    await expect(practice).toContainText("部分题目暂无解析");
  });
});

test.describe("Campus browsing availability", () => {
  test.use({ viewport: { width: 390, height: 800 }, contextOptions: { reducedMotion: "reduce" } });

  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/campus/categories", (route) => route.fulfill({ json: { categories: [], request_id: "campus-categories" } }));
  });

  test("loading stays unknown until an empty response confirms zero items", async ({ page }) => {
    let releaseItems!: () => void;
    const itemsReady = new Promise<void>((resolve) => { releaseItems = resolve; });
    await page.route("**/api/v1/campus/items", async (route) => {
      await itemsReady;
      await route.fulfill({ json: { items: [], request_id: "campus-empty" } });
    });

    await page.goto("/campus");
    const itemsCounter = page.getByText("在架单子", { exact: true }).locator("..");
    await expect(page.getByText("加载互助单…", { exact: true })).toBeVisible();
    await expect(itemsCounter).toHaveAttribute("aria-busy", "true");
    await expect(itemsCounter).toContainText("加载中");
    await expect(itemsCounter).toContainText("—");
    releaseItems();

    await expect(page.getByText("暂无互助或闲置信息", { exact: true })).toBeVisible();
    // 没有筛选条件时，清除筛选帮不上忙，不出现。
    await expect(page.getByRole("button", { name: "清除筛选" })).toHaveCount(0);
    await expect(itemsCounter).toHaveAttribute("aria-busy", "false");
    await expect(itemsCounter).toContainText("0");
    await expect(itemsCounter).not.toContainText("—");
    await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
  });

  test("a failed read offers retry without an empty result, then displays returned items", async ({ page }) => {
    let unavailable = true;
    await page.route("**/api/v1/campus/items", (route) => unavailable
      ? route.fulfill({
          status: 503,
          json: { error: { code: "UPSTREAM_UNAVAILABLE", message: "internal gateway configuration detail" }, request_id: "campus-error" },
        })
      : route.fulfill({
          json: {
            items: [{ id: "campus-bookcase", type: "sell", category: "other", title: "校园书架", desc: "九成新书架", price: 20, seller: "同学", credit: 0, dealsDone: 0, wants: 0, place: "金明校区", status: "open", time: "2026-09-08" }],
            request_id: "campus-recovered",
          },
        }));

    await page.goto("/campus");
    const itemsCounter = page.getByText("在架单子", { exact: true }).locator("..");
    await expect(page.getByRole("main").getByRole("alert")).toContainText("互助信息暂时无法加载，请重试。");
    await expect(page.getByRole("main").getByRole("alert")).not.toContainText("internal gateway");
    await expect(itemsCounter).toContainText("暂不可用");
    await expect(itemsCounter).toContainText("—");
    await expect(page.getByText(/暂无互助或闲置信息|无匹配单子/)).toHaveCount(0);

    unavailable = false;
    await page.getByRole("button", { name: "重试", exact: true }).click();
    await expect(page.getByRole("heading", { name: "校园书架", exact: true })).toBeVisible();
    await expect(itemsCounter).toContainText("1");
    await expect(itemsCounter).not.toContainText("—");
    await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
    await expect(page.getByText(/暂无互助或闲置信息|无匹配单子/)).toHaveCount(0);
  });
});
