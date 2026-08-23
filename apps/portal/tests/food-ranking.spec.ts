import { expect, test } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const POSTS = [
  {
    id: "hang-1",
    campus: "minglun",
    title: "鼓楼夜市",
    excerpt: "第一次来开封很适合从这里开始。",
    blocks: [{ type: "p", text: "选择多、烟火气足。" }],
    author: "学生编辑部",
    likes: 90,
    stars: 20,
    tags: ["夜市", "夯"],
    shop: { name: "鼓楼夜市" },
    time: "07-16",
    hidden: false,
    images: ["/api/v1/food/posts/hang-1/images/0"],
  },
  {
    id: "top-1",
    campus: "jinming",
    title: "西司夜市",
    excerpt: "稳稳推荐。",
    blocks: [{ type: "p", text: "适合朋友聚餐。" }],
    author: "学生编辑部",
    likes: 80,
    stars: 15,
    tags: ["夜市", "顶级"],
    shop: { name: "西司夜市" },
    time: "07-16",
    hidden: false,
    images: [],
  },
  {
    id: "elite-1",
    campus: "minglun",
    title: "第一楼",
    excerpt: "预算足再冲。",
    blocks: [{ type: "p", text: "开封老字号。" }],
    author: "学生编辑部",
    likes: 70,
    stars: 10,
    tags: ["老字号", "人上人"],
    shop: { name: "第一楼" },
    time: "07-16",
    hidden: false,
    images: [],
  },
  {
    id: "npc-1",
    campus: "longzihu",
    title: "龙子湖食堂",
    excerpt: "日常不出错。",
    blocks: [{ type: "p", text: "赶课保底。" }],
    author: "学生编辑部",
    likes: 60,
    stars: 8,
    tags: ["校内", "NPC"],
    shop: { name: "龙子湖食堂" },
    time: "07-16",
    hidden: false,
    images: [],
  },
  {
    id: "bad-1",
    campus: "longzihu",
    title: "待复核餐饮圈",
    excerpt: "先别急着去。",
    blocks: [{ type: "p", text: "近期评价不稳定。" }],
    author: "学生编辑部",
    likes: 10,
    stars: 1,
    tags: ["待复核", "拉完了"],
    shop: { name: "待复核餐饮圈" },
    time: "07-16",
    hidden: false,
    images: [],
  },
];

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} renders the five-tier food board and recommendation entry`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/food/posts", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ posts: POSTS, request_id: "req_food_ranking" }),
      });
    });
    await page.route("**/api/v1/food/posts/hang-1/images/0", async (route) => {
      await route.fulfill({
        contentType: "image/png",
        body: Buffer.from(
          "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
          "base64"
        ),
      });
    });

    await page.goto("/food", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "从夯到拉" })).toBeVisible();
    await expect(page.locator("[data-food-tier]")).toHaveCount(5);
    await expect(page.locator("[data-food-tier-label]")).toHaveText([
      "夯",
      "顶级",
      "人上人",
      "NPC",
      "拉完了",
    ]);
    await expect(page.getByRole("link", { name: "投稿一家好店" })).toBeVisible();
    const thumbnail = page.locator(
      'img[src="/api/v1/food/posts/hang-1/images/0"]'
    );
    await expect(thumbnail).toBeVisible();
    await expect(thumbnail).toHaveJSProperty("complete", true);
    expect(
      await thumbnail.evaluate((image: HTMLImageElement) => image.naturalWidth)
    ).toBeGreaterThan(0);

    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);
  });
}

test("an empty real response keeps the fixed five-tier frame", async ({
  page,
}) => {
  await page.route("**/api/v1/food/posts", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ posts: [], request_id: "req_food_empty" }),
    });
  });

  await page.goto("/food", { waitUntil: "domcontentloaded" });

  await expect(page.locator("[data-food-tier]")).toHaveCount(5);
  await expect(page.locator("[data-food-tier-label]")).toHaveText([
    "夯",
    "顶级",
    "人上人",
    "NPC",
    "拉完了",
  ]);
  await expect(page.getByText("暂无上榜条目", { exact: true })).toHaveCount(5);
});
