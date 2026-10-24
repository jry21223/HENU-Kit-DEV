import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });
test.beforeEach(async ({ page }) => {
  await page.route("https://images.example.com/**", (route) => route.fulfill({
    contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64"),
  }));
});

test("a short owner post presents each fact and photo once in a compact dossier", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const post = {
    id: "compact", campus: "jinming", title: "同学面馆", shop: { name: "同学面馆" },
    excerpt: "便宜、好吃，晚课后也能吃上热饭。", author: "同学", time: "10-24",
    tags: ["夯"], likes: 0, stars: 0, hidden: false,
    blocks: [
      { type: "p", text: "价格参考：18元" },
      { type: "p", text: "营业时间参考：11:00–21:00" },
      { type: "h2", text: "点什么" },
      { type: "list", items: ["牛肉面：汤浓", "番茄面：清淡", "炸酱面：酱香", "炒面：现炒"] },
    ],
    images: ["/api/v1/food/posts/compact/images/0"],
  };
  await page.route("**/api/v1/food/posts/compact", (route) => route.fulfill({ json: { post, comments: [] } }));
  await page.route("**/api/v1/food/posts/compact/images/0", (route) => route.fulfill({
    contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64"),
  }));
  await page.goto("/food/post/compact");
  await expect(page.getByText(post.excerpt, { exact: true })).toHaveCount(1);
  await expect(page.getByText("18元", { exact: true })).toHaveCount(1);
  await expect(page.getByText("11:00–21:00", { exact: true })).toHaveCount(1);
  await expect(page.locator('img[src="/api/v1/food/posts/compact/images/0"]')).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "为什么是这一档" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "学生锐评" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "图片与环境" })).toHaveCount(0);
  const dishes = page.getByRole("list", { name: "推荐菜品" });
  await expect(dishes.getByRole("listitem")).toHaveCount(4);
  expect((await dishes.boundingBox())!.height).toBeLessThan(360);
  await expect(page.getByText("汤浓", { exact: true })).toHaveCount(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

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
    await expect(page.getByText("34.7972, 114.3073")).toHaveCount(0);
    await expect(page.getByText("0.0000, 0.0000")).toHaveCount(0);
    await expect(page.getByText("人均 ¥25–50", { exact: true })).toBeVisible();
    // 营业时间没填就不占一格，不把空值“未填写”摆出来（#549）。
    await expect(page.getByText("未填写", { exact: true })).toHaveCount(0);
    await expect(page.getByText(/营业参考/)).toHaveCount(0);
    await expect(page.getByText(/地图/)).toHaveCount(0);
    await expect(
      page.getByText("学生编辑部 · 社区稿件")
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "推荐菜品" }).first()
    ).toBeVisible();
    await expect(page.getByText("灌汤包", { exact: true })).toBeVisible();
    // The second photo stays in the article, without another gallery copy.
    await expect(page.locator('img[src="https://images.example.com/gulou-environment.jpg"]')).toHaveCount(1);
    await expect(page.locator('img[src="https://images.example.com/gulou-cover.jpg"]')).toHaveCount(1);
    await expect(page.getByText("选择多、烟火气足，人均 ¥25–50。", { exact: true })).toHaveCount(1);
    await expect(page.getByRole("heading", { name: "推荐菜品", exact: true })).toHaveCount(1);
    await expect(page.getByRole("link", { name: "在高德地图打开" })).toHaveCount(0);
    await expect(page.getByRole("link", { name: "投稿一家好店" })).toBeVisible();

    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("a post without images collapses the gallery to one line and states its source once (#549)", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const post = {
    ...DETAIL.post,
    id: "no-image-post",
    blocks: [{ type: "p", text: "选择多、烟火气足。" }],
    images: [],
  };
  await page.route("**/api/v1/food/posts/no-image-post", (route) =>
    route.fulfill({ json: { post, comments: [], request_id: "req_food_detail_no_image" } })
  );

  await page.goto("/food/post/no-image-post", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { level: 1, name: "鼓楼夜市" })).toBeVisible();

  const note = page.getByText("图片与环境：投稿未附图片", { exact: true });
  await expect(note).toBeVisible();
  const box = await note.boundingBox();
  expect(box?.height).toBeLessThan(60);
  await expect(page.getByRole("heading", { name: "图片与环境" })).toHaveCount(0);
  await expect(page.getByText("图片与环境待补充")).toHaveCount(0);

  // 价格和营业时间都没填：只剩五档定位一格，不展示空值。
  await expect(page.getByText("未填写", { exact: true })).toHaveCount(0);
  await expect(page.getByText(/价格参考|营业参考/)).toHaveCount(0);

  // 侧栏只说一次“社区稿件”。
  const aside = page.locator("aside");
  await expect(aside.getByText("学生编辑部 · 社区稿件")).toBeVisible();
  await expect(aside.getByText(/社区稿件/)).toHaveCount(1);

  // 没有补充时的段落占位随整页一起出现，不是结果变化：看得到，但不作为状态播报。
  const main = page.locator("main");
  await expect(main.getByText("暂无学生补充", { exact: true })).toBeVisible();
  await expect(main.getByRole("status")).toHaveCount(0);
});
