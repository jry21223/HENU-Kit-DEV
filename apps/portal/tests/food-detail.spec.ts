import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const DETAIL = {
  post: {
    id: "gulou-night-market",
    campus: "minglun",
    title: "鼓楼夜市",
    excerpt: "第一次来开封很适合从这里开始。",
    blocks: [
      { type: "p", text: "选择多、烟火气足，人均 ¥25–50。" },
      { type: "h2", text: "推荐菜品" },
      {
        type: "list",
        items: ["灌汤包：开封代表性面点", "杏仁茶：适合搭配小吃"],
      },
      {
        type: "img",
        src: "https://images.example.com/gulou-environment.jpg",
      },
    ],
    author: "学生编辑部",
    likes: 90,
    stars: 20,
    tags: ["夜市", "夯"],
    shop: {
      name: "鼓楼夜市",
      lat: 34.7972,
      lng: 114.3073,
    },
    time: "2026-07-16",
    hidden: false,
    images: ["https://images.example.com/gulou-cover.jpg"],
  },
  comments: [],
  request_id: "req_food_detail",
};

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} venue dossier shows real details and recommendation entry`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/food/posts/gulou-night-market", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(DETAIL),
      });
    });

    await page.goto("/food/post/gulou-night-market", {
      waitUntil: "domcontentloaded",
    });

    await expect(page.getByRole("heading", { name: "鼓楼夜市" })).toBeVisible();
    await expect(page.getByText("夯", { exact: true })).toBeVisible();
    await expect(page.getByText("34.7972, 114.3073")).toBeVisible();
    await expect(page.getByText("人均 ¥25–50", { exact: true })).toBeVisible();
    await expect(
      page.getByText("待核验 · 出发前请查地图")
    ).toBeVisible();
    await expect(
      page.getByText("学生编辑部 · 社区稿件")
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "推荐菜品" }).first()
    ).toBeVisible();
    await expect(page.getByText("灌汤包", { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "图片与环境" })).toBeVisible();
    await expect(page.getByRole("link", { name: "在高德地图打开" })).toHaveAttribute(
      "href",
      /uri\.amap\.com\/search/
    );
    await expect(page.getByRole("link", { name: "投稿一家好店" })).toBeVisible();

    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}
