import { expect, test, type Page } from "@playwright/test";

test.describe.configure({ mode: "serial" });

const userID = "22222222-2222-4222-8222-222222222222";
const postID = "33333333-3333-4333-8333-333333333333";
const PHOTO = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAADAAAAAkCAIAAABAJy5dAAAAM0lEQVR42u3OAQkAAAgDsMcxhP0xljUUBguwTNcpERISEhISEhISEhISEhISEhISEhISEhISEvoUWoiW6WpoFdbUAAAAAElFTkSuQmCC",
  "base64",
);

const POST = {
  id: postID,
  campus: "minglun",
  title: "西门小馆",
  excerpt: "味道稳，分量足，赶课保底。",
  blocks: [{ type: "p", text: "人均 ¥18" }],
  author: "小河同学",
  likes: 0,
  stars: 0,
  tags: ["夯"],
  shop: { name: "西门小馆" },
  time: "2030-01-01T00:00:00Z",
  hidden: false,
  images: [],
};

async function installSessionRoute(page: Page, signedIn = true) {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill(
      signedIn
        ? {
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              user_id: userID,
              display_name: "小河同学",
              expires_at: "2030-01-01T00:00:00Z",
            }),
          }
        : {
            status: 401,
            contentType: "application/json",
            body: JSON.stringify({
              error: "not_authenticated",
              request_id: "req_session_401",
            }),
          }
    )
  );
}

async function installCreateRoute(
  page: Page,
  images: { content_type: string; data: string }[] = [],
) {
  let createHits = 0;
  await page.route("**/api/v1/food/posts", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ posts: [POST], request_id: "req_food_list" }),
      });
      return;
    }
    createHits += 1;
    expect(route.request().headers()["idempotency-key"]).toMatch(
      /^portal-food-post:/
    );
    expect(route.request().postDataJSON()).toEqual({
      venue_name: "西门小馆",
      campus: "minglun",
      tier: "hang",
      review_text: "味道稳，分量足，赶课保底。",
      price_reference: "",
      hours_reference: "",
      dishes: [],
      images,
    });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ post: POST, request_id: "req_food_create" }),
    });
  });
  await page.route(`**/api/v1/food/posts/${postID}`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        post: POST,
        comments: [],
        request_id: "req_food_detail",
      }),
    })
  );
  return {
    createHitCount: () => createHits,
  };
}

async function fillValidForm(page: Page) {
  await page.getByLabel("店铺名").fill("西门小馆");
  await page.getByRole("button", { name: "明伦校区" }).click();
  await page.getByRole("button", { name: "夯" }).click();
  await page.getByLabel("锐评正文").fill("味道稳，分量足，赶课保底。");
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`${viewport.name} renders the in-site form and submits a public food post`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await installSessionRoute(page);
    const createRoute = await installCreateRoute(page);

    await page.goto("/food/publish", { waitUntil: "domcontentloaded" });

    await expect(
      page.getByRole("heading", { name: "你吃到的好店，投到这里。" })
    ).toBeVisible();
    await expect(page.getByLabel("店铺名")).toBeVisible();
    await expect(page.getByRole("button", { name: "明伦校区" })).toBeVisible();
    await expect(page.getByRole("button", { name: "夯" })).toBeVisible();
    await expect(page.getByLabel("锐评正文")).toBeVisible();
    // 有边框的按钮本身做到 44px 高（DESIGN_SYSTEM §13）。
    await expect(page.getByRole("button", { name: "+ 加一道" })).toHaveCSS("min-height", "44px");
    await expect(page.getByRole("link", { name: "查看我的投稿" })).toHaveAttribute(
      "href",
      "/account/posts"
    );

    // 必填缺失：明确提示且不发请求
    await page.getByRole("button", { name: "提交投稿" }).click();
    await expect(page.getByText("请填写店铺名")).toBeVisible();
    await expect(page.getByText("请选择校区")).toBeVisible();
    await expect(page.getByText("请选择五档定位")).toBeVisible();
    await expect(page.getByText("请填写锐评正文")).toBeVisible();
    await expect(
      page.getByText("必填项还没填完，请先补齐再提交。")
    ).toBeVisible();
    expect(createRoute.createHitCount()).toBe(0);

    const width = await page.evaluate(() => ({
      client: document.documentElement.clientWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    expect(width.scroll).toBeLessThanOrEqual(width.client + 2);

    // 补齐必填项后成功提交，落到新帖详情页
    await fillValidForm(page);
    await page.getByRole("button", { name: "提交投稿" }).click();

    await page.waitForURL(`**/food/post/${postID}`);
    await expect(
      page.getByRole("heading", { name: "西门小馆" })
    ).toBeVisible();
    expect(createRoute.createHitCount()).toBe(1);

    // 新帖立即出现在公开读路径：五档榜
    await page.goto("/food", { waitUntil: "domcontentloaded" });
    await expect(
      page.locator('[data-food-tier="hang"]').getByText("西门小馆").first()
    ).toBeVisible();
  });
}

test("signed-out visitors are redirected to login with a return path", async ({
  page,
}) => {
  await installSessionRoute(page, false);

  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });

  await page.waitForURL("**/account/login?next=*");
  expect(page.url()).toContain(
    `/account/login?next=${encodeURIComponent("/food/publish")}`
  );
});

test("daily cap surfaces the explicit Chinese message and stays on the form", async ({
  page,
}) => {
  await installSessionRoute(page);
  await page.route("**/api/v1/food/posts", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ posts: [], request_id: "req_food_list" }),
      });
      return;
    }
    await route.fulfill({
      status: 429,
      contentType: "application/json",
      body: JSON.stringify({
        error: {
          code: "DAILY_POST_CAP_REACHED",
          message: "daily submission cap reached",
        },
        request_id: "req_food_cap",
      }),
    });
  });

  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
  await fillValidForm(page);
  await page.getByRole("button", { name: "提交投稿" }).click();

  await expect(page.getByText("今天已经投满 3 条，明天再来吧")).toBeVisible();
  await expect(page.getByText(/服务暂时不可用/)).toHaveCount(0);
  expect(page.url()).toContain("/food/publish");
});

test("a failed submit keeps the same Idempotency-Key for the retry", async ({
  page,
}) => {
  await installSessionRoute(page);
  const keys: string[] = [];
  let createHits = 0;
  await page.route("**/api/v1/food/posts", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ posts: [], request_id: "req_food_list" }),
      });
      return;
    }
    createHits += 1;
    keys.push(route.request().headers()["idempotency-key"] ?? "");
    if (createHits === 1) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          error: "food_unavailable",
          request_id: "req_food_down",
        }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ post: POST, request_id: "req_food_create" }),
    });
  });
  await page.route(`**/api/v1/food/posts/${postID}`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        post: POST,
        comments: [],
        request_id: "req_food_detail",
      }),
    })
  );

  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
  await fillValidForm(page);
  await page.getByRole("button", { name: "提交投稿" }).click();
  await expect(page.getByText("提交失败，请稍后重试。")).toBeVisible();

  await page.getByRole("button", { name: "提交投稿" }).click();
  await page.waitForURL(`**/food/post/${postID}`);

  expect(createHits).toBe(2);
  expect(keys[0]).toMatch(/^portal-food-post:/);
  expect(keys[1]).toBe(keys[0]);
});

test("an image above 2MiB is rejected client-side with a clear message", async ({
  page,
}) => {
  await installSessionRoute(page);
  let createHits = 0;
  await page.route("**/api/v1/food/posts", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ posts: [], request_id: "req_food_list" }),
      });
      return;
    }
    createHits += 1;
    expect(route.request().postDataJSON().images).toEqual([]);
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ post: POST, request_id: "req_food_create" }),
    });
  });
  await page.route(`**/api/v1/food/posts/${postID}`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        post: POST,
        comments: [],
        request_id: "req_food_detail",
      }),
    })
  );

  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
  await page.locator('input[type="file"]').setInputFiles({
    name: "too-big.jpg",
    mimeType: "image/jpeg",
    buffer: Buffer.alloc(2 * 1024 * 1024 + 1, 1),
  });

  await expect(page.getByText("图片大小不能超过 2MB")).toBeVisible();
  await expect(page.getByText("FIG.1")).toHaveCount(0);

  await fillValidForm(page);
  await page.getByRole("button", { name: "提交投稿" }).click();
  await page.waitForURL(`**/food/post/${postID}`);
  expect(createHits).toBe(1);
});

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 360, height: 800 },
]) {
  test(`${viewport.name} rejects an unreadable photo before submission`, async ({ page }, testInfo) => {
    await page.setViewportSize(viewport);
    await installSessionRoute(page);
    const createRoute = await installCreateRoute(page);
    await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
    await page.locator('input[type="file"]').setInputFiles({
      name: "damaged.jpg",
      mimeType: "image/jpeg",
      buffer: Buffer.from("not an image"),
    });

    await expect(page.getByRole("alert").filter({ hasText: "图片无法读取" })).toHaveText("图片无法读取，请重新选择 JPG / PNG / WebP 图片");
    await expect(page.getByText("图片（0/6，单张 ≤2MB，可选）")).toBeVisible();
    await expect(page.getByRole("img", { name: "图 1", exact: true })).toHaveCount(0);
    await page.getByRole("alert").filter({ hasText: "图片无法读取" }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath(`${viewport.name}.png`) });
    await fillValidForm(page);
    await page.getByRole("button", { name: "提交投稿" }).click();
    await page.waitForURL(`**/food/post/${postID}`);
    expect(createRoute.createHitCount()).toBe(1);
  });
}

test("a photo with a mismatched file type is rejected before submission", async ({ page }) => {
  await installSessionRoute(page);
  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
  await page.locator('input[type="file"]').setInputFiles({
    name: "renamed.jpg",
    mimeType: "image/jpeg",
    buffer: PHOTO,
  });

  await expect(page.getByText("图片格式与文件类型不一致，请重新导出 JPG / PNG / WebP 图片")).toBeVisible();
  await expect(page.getByText("图片（0/6，单张 ≤2MB，可选）")).toBeVisible();
});

for (const contentType of ["image/jpeg", "image/png", "image/webp"]) {
  test(`a valid ${contentType} photo previews and enters the submission`, async ({ page }) => {
    await installSessionRoute(page);
    await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
    const data = await page.evaluate((type) => {
      const canvas = document.createElement("canvas");
      canvas.width = 48;
      canvas.height = 36;
      const context = canvas.getContext("2d")!;
      context.fillStyle = "#cc6633";
      context.fillRect(0, 0, 48, 36);
      return canvas.toDataURL(type).split(",")[1];
    }, contentType);
    const createRoute = await installCreateRoute(page, [{ content_type: contentType, data }]);
    await page.locator('input[type="file"]').setInputFiles({
      name: `meal.${contentType.split("/")[1]}`,
      mimeType: contentType,
      buffer: Buffer.from(data, "base64"),
    });

    const preview = page.getByRole("img", { name: "图 1", exact: true });
    await preview.scrollIntoViewIfNeeded();
    await expect(preview).toHaveJSProperty("naturalWidth", 48);
    await expect(preview).toHaveJSProperty("naturalHeight", 36);
    await fillValidForm(page);
    await page.getByRole("button", { name: "提交投稿" }).click();
    await page.waitForURL(`**/food/post/${postID}`);
    expect(createRoute.createHitCount()).toBe(1);
  });
}

test("only six images can be attached; the upload slot disappears", async ({
  page,
}) => {
  await installSessionRoute(page);

  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });
  const input = page.locator('input[type="file"]');
  for (let i = 0; i < 6; i += 1) {
    await input.setInputFiles({
      name: `img-${i + 1}.png`,
      mimeType: "image/png",
      buffer: PHOTO,
    });
    await expect(
      page.getByText(`图片（${i + 1}/6，单张 ≤2MB，可选）`)
    ).toBeVisible();
  }
  await expect(page.locator('input[type="file"]')).toHaveCount(0);
  await expect(page.getByText("+ 上传")).toHaveCount(0);
});

test("publish page copy describes immediate publish without review-queue wording", async ({
  page,
}) => {
  await installSessionRoute(page);

  await page.goto("/food/publish", { waitUntil: "domcontentloaded" });

  await expect(
    page.getByText("提交后立即公开到五档榜，同校区同学马上能看到。")
  ).toBeVisible();
  await expect(page.getByText("没有审核环节，帖子会立即进入五档榜。")).toBeVisible();
  await expect(page.getByText(/维护者核验|待审核|审核中/)).toHaveCount(0);
});
