import { expect, test } from "@playwright/test";

/**
 * The board fetches its rows on the client, so the document is still empty at
 * the moment the browser would restore the offset on back navigation. Without
 * explicit restoration the reader is dropped at the top of the board and loses
 * their place in a five-section list.
 */

const TIER_TAGS = ["夯", "顶级", "人上人", "NPC", "拉完了"];

/** Enough rows per tier that the board is several viewports tall. */
const POSTS = TIER_TAGS.flatMap((tag, tierIndex) =>
  Array.from({ length: 8 }, (_, row) => ({
    id: `${tierIndex}-${row}`,
    campus: "minglun",
    title: `${tag} 档第 ${row + 1} 家`,
    excerpt: "用于滚动恢复断言的条目。",
    blocks: [{ type: "p", text: "正文。" }],
    author: "学生编辑部",
    likes: 100 - row,
    stars: 10,
    tags: [tag],
    shop: { name: `${tag}-${row + 1} 号店`, lat: 34.79, lng: 114.3 },
    time: "07-16",
    hidden: false,
    images: [],
  }))
);

test("returning from a venue keeps the reader's place on the board", async ({
  page,
}) => {
  await page.route("**/api/v1/food/posts", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ posts: POSTS, request_id: "req_food_scroll" }),
    });
  });
  await page.route("**/api/v1/food/posts/*", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        post: POSTS[0],
        comments: [],
        request_id: "req_food_scroll_detail",
      }),
    });
  });

  await page.goto("/food", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-food-tier]")).toHaveCount(5);

  // Land somewhere well down the board, then let the recorder observe it.
  await page.evaluate(() => window.scrollTo(0, 1600));
  await expect
    .poll(async () => page.evaluate(() => Math.round(window.scrollY)))
    .toBeGreaterThan(1200);
  const departedFrom = await page.evaluate(() => Math.round(window.scrollY));

  // Click a venue that is already on screen. Playwright's own click would
  // first scroll the target into view, which would move the offset under test.
  await page.evaluate(() => {
    const onScreen = Array.from(
      document.querySelectorAll<HTMLAnchorElement>('a[href^="/food/post/"]')
    ).find((link) => {
      const box = link.getBoundingClientRect();
      return box.top >= 0 && box.bottom <= window.innerHeight;
    });
    if (!onScreen) throw new Error("no venue link on screen to click");
    onScreen.click();
  });
  await page.waitForURL(/\/food\/post\//);

  await page.goBack();
  await expect(page.locator("[data-food-tier]")).toHaveCount(5);

  await expect
    .poll(async () => page.evaluate(() => Math.round(window.scrollY)), {
      timeout: 5000,
    })
    .toBeGreaterThan(departedFrom - 100);
});

test("arriving fresh from another page still starts at the top", async ({
  page,
}) => {
  await page.route("**/api/v1/food/posts", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ posts: POSTS, request_id: "req_food_scroll" }),
    });
  });

  await page.goto("/food", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-food-tier]")).toHaveCount(5);
  await page.evaluate(() => window.scrollTo(0, 1600));
  await expect
    .poll(async () => page.evaluate(() => Math.round(window.scrollY)))
    .toBeGreaterThan(1200);

  // A fresh navigation is not a return, so the recorded offset must not apply.
  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
  await page.goto("/food", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-food-tier]")).toHaveCount(5);

  await expect
    .poll(async () => page.evaluate(() => Math.round(window.scrollY)))
    .toBeLessThan(100);
});
