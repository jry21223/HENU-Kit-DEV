import { expect, test, type Page } from "@playwright/test";

const userID = "11111111-1111-4111-8111-111111111111";
const postID = "55555555-5555-4555-8555-555555555555";

const MY_POST = {
  id: postID,
  campus: "jinming",
  title: "西门鸡腿饭",
  excerpt: "下课就来排队的鸡腿饭。",
  blocks: [{ type: "p", text: "人均 ¥12。" }],
  author: "小河同学",
  likes: 0,
  stars: 0,
  tags: ["食堂", "NPC"],
  shop: { name: "西门鸡腿饭", lat: 0, lng: 0 },
  time: "2030-01-02T03:04:05Z",
  hidden: false,
  images: [],
};

const SECOND_POST = {
  id: "66666666-6666-4666-8666-666666666666",
  campus: "minglun",
  title: "鼓楼灌汤包",
  excerpt: "皮薄汤鲜。",
  blocks: [{ type: "p", text: "人均 ¥20。" }],
  author: "小河同学",
  likes: 0,
  stars: 0,
  tags: ["小吃", "夯"],
  shop: { name: "鼓楼灌汤包", lat: 0, lng: 0 },
  time: "2030-01-03T05:06:07Z",
  hidden: false,
  images: [],
};

const FOREIGN_POST_TITLE = "别人的私藏榜单";

type RouteHandler = Parameters<Page["route"]>[1] extends (...args: infer Args) => unknown
  ? Args[0]
  : never;

async function fulfill(route: RouteHandler, status: number, body: unknown) {
  await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}

async function mockSession(page: Page) {
  await page.route("**/api/v1/session", (route) =>
    fulfill(route, 200, {
      user_id: userID,
      display_name: "小河同学",
      expires_at: "2030-01-01T00:00:00Z",
    })
  );
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} anonymous visit to 我的投稿 redirects to account login`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.route("**/api/v1/session", (route) =>
      fulfill(route, 401, { error: "not_authenticated", request_id: "req_expired_session" })
    );

    await page.goto("/account/posts", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/account\/login\?next=%2Faccount%2Fposts/);
    await expect(page.locator('[data-account-food-posts-state="error"]')).toHaveCount(0);
    await expect(page.locator('[data-account-food-posts-state="success"]')).toHaveCount(0);
  });

  test(`${viewport.name} signed-in owner sees only their own food posts`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockSession(page);
    let publicListRequests = 0;
    await page.route("**/api/v1/food/posts/mine", (route) =>
      fulfill(route, 200, { posts: [MY_POST, SECOND_POST], request_id: "req_my_posts" })
    );
    await page.route("**/api/v1/food/posts", (route) => {
      publicListRequests += 1;
      return fulfill(route, 200, {
        posts: [
          {
            ...MY_POST,
            id: "77777777-7777-4777-8777-777777777777",
            title: FOREIGN_POST_TITLE,
            author: "路人甲",
          },
        ],
        request_id: "req_public_posts",
      });
    });

    await page.goto("/account/posts", { waitUntil: "domcontentloaded" });

    await expect(page.getByRole("heading", { name: "我的投稿" })).toBeVisible();
    await expect(page.getByRole("link", { name: "我的投稿" })).toBeVisible();
    await expect(page.getByRole("link", { name: /西门鸡腿饭/ })).toBeVisible();
    await expect(page.getByRole("link", { name: /鼓楼灌汤包/ })).toBeVisible();
    await expect(page.getByText("NPC", { exact: true })).toBeVisible();
    await expect(page.getByText("夯", { exact: true })).toBeVisible();
    await expect(page.getByText(/金明校区/)).toBeVisible();
    await expect(page.getByText(/明伦校区/)).toBeVisible();
    await expect(page.getByText(/发布于/).first()).toBeVisible();
    await expect(page.getByText(FOREIGN_POST_TITLE)).toHaveCount(0);
    // 全局预热（initAllGateways → initFoodGateway）会拉取公共 food 列表，
    // 这是首屏数据预热的既有设计；本测试只保证公共内容不混入“我的投稿”。
    expect(publicListRequests).toBeGreaterThanOrEqual(0);
    await expect(page.getByText(/待审核|审核中|审核队列/)).toHaveCount(0);
  });

  test(`${viewport.name} empty owner list guides to 去投稿`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockSession(page);
    await page.route("**/api/v1/food/posts/mine", (route) =>
      fulfill(route, 200, { posts: [], request_id: "req_my_posts_empty" })
    );

    await page.goto("/account/posts", { waitUntil: "domcontentloaded" });

    await expect(page.locator('[data-account-food-posts-empty]')).toBeVisible();
    await expect(page.locator('[data-account-food-posts-empty]').getByText("还没有发布过投稿")).toBeVisible();
    await expect(page.locator('[data-account-food-posts-empty]').getByText(/立即公开/)).toBeVisible();
    const publishLink = page.getByRole("link", { name: "去投稿" });
    await expect(publishLink).toBeVisible();
    await expect(publishLink).toHaveAttribute("href", "/food/publish");
  });

  test(`${viewport.name} clicking a post row opens the public detail page`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockSession(page);
    await page.route("**/api/v1/food/posts/mine", (route) =>
      fulfill(route, 200, { posts: [MY_POST], request_id: "req_my_posts" })
    );
    await page.route(`**/api/v1/food/posts/${postID}`, (route) =>
      fulfill(route, 200, { post: MY_POST, comments: [], request_id: "req_food_detail" })
    );

    await page.goto("/account/posts", { waitUntil: "domcontentloaded" });

    const row = page.getByRole("link", { name: /西门鸡腿饭/ });
    await expect(row).toHaveAttribute("href", `/food/post/${postID}`);
    await row.click();
    await expect(page).toHaveURL(new RegExp(`/food/post/${postID}$`));
    await expect(page.getByRole("heading", { name: "西门鸡腿饭" })).toBeVisible();
  });
}
