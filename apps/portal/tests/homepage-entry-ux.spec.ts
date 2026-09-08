import { expect, test } from "@playwright/test";

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
    await page.getByRole("button", { name: "打开菜单" }).click();
    await expect(page.getByText("发布、接单和结算暂未开放。", { exact: true })).toBeVisible();
    await expect(page.getByText(/实名认证|发单有人接|全覆盖|真实订单即将上线|互助接单/)).toHaveCount(0);

    await page.route("**/api/v1/campus/items", (route) => route.fulfill({ json: { items: [], request_id: "campus-empty" } }));
    await page.route("**/api/v1/campus/categories", (route) => route.fulfill({ json: { categories: [], request_id: "campus-categories" } }));
    await page.goto("/campus");
    await expect(page.getByText("可浏览互助与闲置信息；发布、接单和结算暂未开放。", { exact: true })).toBeVisible();
    await expect(page.locator('meta[name="description"]')).toHaveAttribute("content", /发布、接单和结算暂未开放/);
    await expect(page.getByText(/发单有人接|实名认证|即将上线/)).toHaveCount(0);
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
    await expect(page.getByText("加载互助单 / LOADING", { exact: true })).toBeVisible();
    await expect(itemsCounter).toHaveAttribute("aria-busy", "true");
    await expect(itemsCounter).toContainText("加载中");
    await expect(itemsCounter).toContainText("—");
    releaseItems();

    await expect(page.getByText("暂无互助或闲置信息 / EMPTY", { exact: true })).toBeVisible();
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
    await expect(page.getByText(/\/ EMPTY/)).toHaveCount(0);

    unavailable = false;
    await page.getByRole("button", { name: "重试", exact: true }).click();
    await expect(page.getByRole("heading", { name: "校园书架", exact: true })).toBeVisible();
    await expect(itemsCounter).toContainText("1");
    await expect(itemsCounter).not.toContainText("—");
    await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
    await expect(page.getByText(/\/ EMPTY/)).toHaveCount(0);
  });
});
