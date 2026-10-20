import { expect, test, type Page } from "@playwright/test";

const id = "33333333-3333-4333-8333-333333333333";
const path = `/api/v1/food/posts/${id}`;
const snapshot = {
  id,
  version: 3,
  venue_name: "西门小馆",
  campus: "minglun",
  tier: "hang",
  review_text: "汤鲜料足，赶课保底。",
  price_reference: "18元",
  hours_reference: "11:00–21:00",
  dishes: [{ name: "牛肉面", price: "18元", reason: "汤鲜" }],
  images: [0, 1].map((position) => ({
    position,
    url: `${path}/images/${position}?v=original`,
  })),
};
const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=",
  "base64",
);
const publicPost = {
  id,
  campus: "minglun",
  title: "西门小馆",
  excerpt: "汤鲜料足，赶课保底。",
  blocks: [],
  author: "小河同学",
  likes: 0,
  stars: 0,
  tags: ["夯"],
  shop: { name: "西门小馆" },
  time: "2030-01-01T00:00:00Z",
  hidden: false,
  images: [],
};

async function setup(page: Page, status = 200) {
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      json: {
        user_id: "22222222-2222-4222-8222-222222222222",
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      },
    }),
  );
  await page.route(`**${path}/edit`, (route) =>
    route.fulfill({
      status,
      json:
        status === 200
          ? { post: snapshot, request_id: "req_edit_read" }
          : {
              error: { code: "POST_NOT_FOUND", message: "missing" },
              request_id: "req_edit_missing",
            },
    }),
  );
  await page.route(`**${path}/images/**`, (route) =>
    route.fulfill({ contentType: "image/png", body: png }),
  );
  await page.route(`**${path}`, (route) =>
    route.fulfill({
      json: { post: publicPost, comments: [], request_id: "req_detail" },
    }),
  );
}

for (const width of [360, 1440]) {
  test(`${width}px author prefills text/dishes, keeps/removes/adds photos and saves once`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await setup(page);
    let writes = 0;
    let release!: () => void;
    const saved = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**${path}`, async (route) => {
      if (route.request().method() !== "PUT")
        return route.fulfill({
          json: { post: publicPost, comments: [], request_id: "req_detail" },
        });
      writes++;
      const input = route.request().postDataJSON();
      expect(input).toMatchObject({
        expected_version: 3,
        venue_name: "西门小馆",
        campus: "minglun",
        tier: "hang",
        review_text: "牛肉面也值得来。",
        price_reference: "18元",
        hours_reference: "11:00–21:00",
        retained_image_positions: [1],
        dishes: snapshot.dishes,
      });
      expect(input.images).toHaveLength(1);
      expect(input.images[0].content_type).toBe("image/png");
      expect(input).not.toHaveProperty("author_user_id");
      await saved;
      await route.fulfill({
        json: {
          post: { ...publicPost, excerpt: input.review_text },
          request_id: "req_edit_saved",
        },
      });
    });
    await page.goto(`/food/post/${id}/edit`);
    await expect(page.getByLabel("店铺名")).toHaveValue("西门小馆");
    await expect(page.getByPlaceholder("菜名")).toHaveValue("牛肉面");
    await page.getByLabel("锐评正文").fill("牛肉面也值得来。");
    await page.getByRole("button", { name: "删除图 1" }).click();
    await page
      .locator('input[type="file"]')
      .setInputFiles({ name: "meal.png", mimeType: "image/png", buffer: png });
    await expect(
      page.getByRole("button", { name: "保存修改 →" }),
    ).toBeEnabled();
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`food-edit-${width}.png`),
      fullPage: true,
    });
    await page.getByRole("button", { name: "保存修改 →" }).click();
    await expect(page.getByLabel("店铺名")).toBeDisabled();
    await expect(page.getByRole("button", { name: "提交中…" })).toBeDisabled();
    expect(writes).toBe(1);
    release();
    await expect(page).toHaveURL(`/food/post/${id}`);
  });
}

test("failed save keeps the draft and reuses its exact payload/key on retry", async ({
  page,
}) => {
  await setup(page);
  const attempts: { key: string; body: string | null }[] = [];
  await page.route(`**${path}`, (route) => {
    if (route.request().method() !== "PUT")
      return route.fulfill({
        json: { post: publicPost, comments: [], request_id: "req_detail" },
      });
    attempts.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postData(),
    });
    return route.fulfill(
      attempts.length === 1
        ? {
            status: 503,
            json: {
              error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
              request_id: "req_failure",
            },
          }
        : { json: { post: publicPost, request_id: "req_recovered" } },
    );
  });
  await page.goto(`/food/post/${id}/edit`);
  await page.getByLabel("锐评正文").fill("失败后保留的修改。");
  await page.getByRole("button", { name: "保存修改 →" }).click();
  await expect(page.locator("main").getByRole("alert")).toContainText(
    "提交失败",
  );
  await expect(page.getByLabel("锐评正文")).toHaveValue("失败后保留的修改。");
  await expect(page.getByAltText("图 1", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "保存修改 →" }).click();
  await expect(page).toHaveURL(`/food/post/${id}`);
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
});

test("version conflict retains draft, blocks overwrite and explicitly reloads current fields", async ({
  page,
}) => {
  await setup(page);
  await page.route(`**${path}`, (route) =>
    route.fulfill({
      status: 409,
      json: {
        error: { code: "VERSION_CONFLICT", message: "reload" },
        request_id: "req_conflict",
      },
    }),
  );
  await page.goto(`/food/post/${id}/edit`);
  await page.getByLabel("锐评正文").fill("尚未保存的修改。");
  await page.getByRole("button", { name: "保存修改 →" }).click();
  await expect(page.locator("main").getByRole("alert")).toContainText(
    "投稿已被更新",
  );
  await expect(page.getByLabel("锐评正文")).toHaveValue("尚未保存的修改。");
  await expect(page.getByRole("button", { name: "保存修改 →" })).toBeDisabled();
  await page.getByRole("button", { name: "放弃未保存修改并重新加载" }).click();
  await expect(page.getByLabel("锐评正文")).toHaveValue(snapshot.review_text);
  await expect(page.getByRole("button", { name: "保存修改 →" })).toBeEnabled();
});

test("foreign/not-found and hidden Post do not mount an editable form", async ({
  page,
}) => {
  await setup(page, 404);
  await page.goto(`/food/post/${id}/edit`);
  await expect(page.locator("main").getByRole("alert")).toContainText(
    "无法编辑",
  );
  await expect(page.getByLabel("店铺名")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "返回我的投稿" })).toBeVisible();
});

test("missing session redirects before reading author data", async ({
  page,
}) => {
  await setup(page);
  let reads = 0;
  await page.route(`**${path}/edit`, (route) => {
    reads++;
    return route.abort();
  });
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, json: { error: "not_authenticated" } }),
  );
  await page.goto(`/food/post/${id}/edit`);
  await expect(page).toHaveURL(/\/account\/login\?next=/);
  expect(reads).toBe(0);
});
