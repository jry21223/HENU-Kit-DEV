import { expect, test, type Page } from "@playwright/test";

/**
 * 用户上传图片（#548）：列表缩略图懒加载，首屏以下的图片滚动到附近才请求；详情页主图
 * 立即加载、高优先级，其余图集照样懒加载。图片放在固定尺寸的框里，到达前后尺寸不变；
 * 列表数据到达时也不把首屏里的页脚挤走——手机 390 视口下 /food、/campus 的
 * layout-shift 总和小于 0.1。图片由 page.route 返回一张真实的 48×36 PNG，
 * 没有固定框的图片会按这张图的原始尺寸撑开版面。
 */

const MOBILE = { width: 390, height: 844 };

const PHOTO = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAADAAAAAkCAIAAABAJy5dAAAAM0lEQVR42u3OAQkAAAgDsMcxhP0xljUUBguwTNcpERISEhISEhISEhISEhISEhISEvoUWoiW6WpoFdbUAAAAAElFTkSuQmCC",
  "base64"
);

const TIER_TAGS = ["夯", "顶级", "人上人", "NPC", "拉完了"];

// 五档各四家店，每家一张图：最后一档远在首屏以下。
const FOOD_POSTS = TIER_TAGS.flatMap((tag, tier) =>
  Array.from({ length: 4 }, (_, index) => {
    const id = `food-${tier + 1}-${index + 1}`;
    return {
      id,
      campus: "minglun",
      title: `${tag}档第 ${index + 1} 家`,
      excerpt: "学生实地吃过。",
      blocks: [{ type: "p", text: "味道稳定。" }],
      author: "学生编辑部",
      likes: 0,
      stars: 0,
      tags: [tag],
      shop: { name: `${tag}档店铺 ${index + 1}` },
      time: "2026-09-20",
      hidden: false,
      images: [`/api/v1/food/posts/${id}/images/0`],
    };
  })
);

const CAMPUS_ITEMS = Array.from({ length: 12 }, (_, index) => ({
  id: `campus-${index + 1}`,
  type: index % 2 === 0 ? "sell" : "help",
  category: index % 2 === 0 ? "flea" : "express",
  title: `互助单子 ${index + 1}`,
  desc: "宿舍楼下当面交接。",
  price: 10 + index,
  seller: "同学",
  credit: 0,
  dealsDone: 0,
  wants: 0,
  place: "明伦校区",
  status: "open",
  time: "2026-09-20",
  images: [`https://images.example.com/campus/campus-${index + 1}.png`],
}));

const FOOD_DETAIL = {
  post: {
    id: "food-detail",
    campus: "minglun",
    title: "鼓楼夜市",
    excerpt: "第一次来开封很适合从这里开始。",
    blocks: [
      { type: "p", text: "选择多、烟火气足。" },
      { type: "img", src: "https://images.example.com/food/body.png" },
      { type: "p", text: "晚上八点以后人最多。" },
    ],
    author: "学生编辑部",
    likes: 0,
    stars: 0,
    tags: ["夯"],
    shop: { name: "鼓楼夜市" },
    time: "2026-09-20",
    hidden: false,
    images: ["https://images.example.com/food/cover.png", "https://images.example.com/food/second.png"],
  },
  comments: [],
  request_id: "req_image_food_detail",
};

const CAMPUS_DETAIL = {
  item: {
    ...CAMPUS_ITEMS[0],
    images: [
      "https://images.example.com/campus/detail-1.png",
      "https://images.example.com/campus/detail-2.png",
      "https://images.example.com/campus/detail-3.png",
    ],
  },
  messages: [],
  request_id: "req_image_campus_detail",
};

const FOOD_IMAGE = /^\/api\/v1\/food\/posts\/([^/]+)\/images\/0$/;
const CAMPUS_IMAGE = /^\/campus\/(campus-\d+)\.png$/;

async function mockFood(page: Page) {
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({ json: { posts: FOOD_POSTS, request_id: "req_image_food" } })
  );
  await page.route("**/api/v1/food/posts/*/images/0", (route) =>
    route.fulfill({ contentType: "image/png", body: PHOTO })
  );
}

async function mockCampus(page: Page) {
  await page.route("**/api/v1/campus/items", (route) =>
    route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_image_campus" } })
  );
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_image_categories" } })
  );
  await page.route("https://images.example.com/**", (route) =>
    route.fulfill({ contentType: "image/png", body: PHOTO })
  );
}

async function mockDetails(page: Page) {
  await page.route(`**/api/v1/food/posts/${FOOD_DETAIL.post.id}`, (route) =>
    route.fulfill({ json: FOOD_DETAIL })
  );
  await page.route(`**/api/v1/campus/items/${CAMPUS_DETAIL.item.id}`, (route) =>
    route.fulfill({ json: CAMPUS_DETAIL })
  );
  await page.route("https://images.example.com/**", (route) =>
    route.fulfill({ contentType: "image/png", body: PHOTO })
  );
}

/** 记录页面请求过的图片，按 id 记：食物投稿 id 或互助单子 id。 */
function recordImageRequests(page: Page, pattern: RegExp): Set<string> {
  const requested = new Set<string>();
  page.on("request", (request) => {
    const match = new URL(request.url()).pathname.match(pattern);
    if (match) requested.add(match[1]);
  });
  return requested;
}

/** 等水合完成、页面发起的请求都落地。 */
async function settle(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
  await page.waitForLoadState("networkidle");
}

type LayoutShiftRecord = {
  total: number;
  add: (entries: PerformanceEntryList) => void;
  observer: PerformanceObserver;
};

/** 从导航开始累计 layout-shift（不算紧跟用户输入的偏移）。 */
async function observeLayoutShift(page: Page) {
  await page.addInitScript(() => {
    const record = {
      total: 0,
      add(entries: PerformanceEntryList) {
        for (const entry of entries as Array<PerformanceEntry & { value: number; hadRecentInput: boolean }>) {
          if (!entry.hadRecentInput) record.total += entry.value;
        }
      },
    } as LayoutShiftRecord;
    record.observer = new PerformanceObserver((list) => record.add(list.getEntries()));
    record.observer.observe({ type: "layout-shift", buffered: true });
    (window as unknown as { __layoutShift: LayoutShiftRecord }).__layoutShift = record;
  });
}

/** 一屏一屏往下滚到底，每屏等屏内图片加载完，让懒加载的图也在测量范围内落位。 */
async function scrollThrough(page: Page) {
  for (;;) {
    const atBottom = await page.evaluate(() => {
      window.scrollBy(0, window.innerHeight);
      return window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 1;
    });
    await page.waitForFunction(() =>
      Array.from(document.images).every((image) => {
        const { top, bottom } = image.getBoundingClientRect();
        return image.complete || bottom < 0 || top > window.innerHeight;
      })
    );
    if (atBottom) break;
  }
}

/**
 * 读累计值前先等两帧：最后一屏的图片 complete 之后，要到下一帧布局时才产生偏移记录；
 * 观察者的回调又排在之后的任务里，所以再用 takeRecords() 取走还没送达的记录。
 */
async function layoutShiftTotal(page: Page): Promise<number> {
  return page.evaluate(async () => {
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const record = (window as unknown as { __layoutShift: LayoutShiftRecord }).__layoutShift;
    record.add(record.observer.takeRecords());
    return record.total;
  });
}

test.use({ viewport: MOBILE, contextOptions: { reducedMotion: "reduce" } });

test("/food requests a below-the-fold thumbnail only once it is scrolled near", async ({ page }) => {
  await mockFood(page);
  const requested = recordImageRequests(page, FOOD_IMAGE);

  await page.goto("/food");
  await settle(page);

  const first = FOOD_POSTS[0].id;
  const last = FOOD_POSTS[FOOD_POSTS.length - 1].id;
  await expect(page.locator(`img[src="/api/v1/food/posts/${first}/images/0"]`)).toHaveJSProperty("complete", true);
  expect(requested.has(first)).toBe(true);
  expect(requested.has(last)).toBe(false);
  expect(requested.size).toBeLessThan(FOOD_POSTS.length);

  const lastImage = page.locator(`img[src="/api/v1/food/posts/${last}/images/0"]`);
  await lastImage.scrollIntoViewIfNeeded();
  await expect.poll(() => requested.has(last)).toBe(true);
  await expect(lastImage).toHaveJSProperty("complete", true);
});

test("/campus requests a below-the-fold card image only once it is scrolled near", async ({ page }) => {
  await mockCampus(page);
  const requested = recordImageRequests(page, CAMPUS_IMAGE);

  await page.goto("/campus");
  await settle(page);

  const first = CAMPUS_ITEMS[0];
  const last = CAMPUS_ITEMS[CAMPUS_ITEMS.length - 1];
  await expect(page.locator(`img[src="${first.images[0]}"]`)).toHaveJSProperty("complete", true);
  expect(requested.has(first.id)).toBe(true);
  expect(requested.has(last.id)).toBe(false);
  expect(requested.size).toBeLessThan(CAMPUS_ITEMS.length);

  const lastImage = page.locator(`img[src="${last.images[0]}"]`);
  await lastImage.scrollIntoViewIfNeeded();
  await expect.poll(() => requested.has(last.id)).toBe(true);
  await expect(lastImage).toHaveJSProperty("complete", true);
});

test("detail pages load the lead photo right away and leave the rest of the gallery lazy", async ({ page }) => {
  await mockDetails(page);

  await page.goto(`/food/post/${FOOD_DETAIL.post.id}`);
  await settle(page);
  const cover = page.getByRole("img", { name: "鼓楼夜市参考图", exact: true });
  await expect(cover).toHaveAttribute("loading", "eager");
  await expect(cover).toHaveAttribute("fetchpriority", "high");
  const gallery = page.getByRole("img", { name: /^鼓楼夜市稿件参考图 \d+$/ });
  await expect(gallery).toHaveCount(3);
  for (const photo of await gallery.all()) {
    await expect(photo).toHaveAttribute("loading", "lazy");
  }
  await expect(page.getByRole("img", { name: "正文插图" })).toHaveAttribute("loading", "lazy");

  await page.goto(`/campus/item/${CAMPUS_DETAIL.item.id}`);
  await settle(page);
  const lead = page.getByRole("img", { name: CAMPUS_DETAIL.item.title, exact: true });
  await expect(lead).toHaveAttribute("loading", "eager");
  await expect(lead).toHaveAttribute("fetchpriority", "high");
  const rest = page.getByRole("img", { name: /^互助单子 1 图 \d+$/ });
  await expect(rest).toHaveCount(2);
  for (const photo of await rest.all()) {
    await expect(photo).toHaveAttribute("loading", "lazy");
  }
});

test("a food article photo holds its box before the image arrives", async ({ page }) => {
  await mockDetails(page);
  let release = () => {};
  const arrived = new Promise<void>((resolve) => (release = resolve));
  await page.route("https://images.example.com/food/body.png", async (route) => {
    await arrived;
    await route.fulfill({ contentType: "image/png", body: PHOTO });
  });

  await page.goto(`/food/post/${FOOD_DETAIL.post.id}`);
  // 不等 networkidle：正文图的请求一直挂着。
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
  const photo = page.getByRole("img", { name: "正文插图" });
  await photo.scrollIntoViewIfNeeded();
  await expect(photo).toHaveJSProperty("complete", false);
  const before = await photo.boundingBox();

  release();
  await expect(photo).toHaveJSProperty("complete", true);
  const after = await photo.boundingBox();

  expect(before?.height).toBeGreaterThan(100);
  expect(after?.height).toBe(before?.height);
});

for (const { path, mock } of [
  { path: "/food", mock: mockFood },
  { path: "/campus", mock: mockCampus },
]) {
  test(`${path} keeps the mobile layout-shift total under 0.1 while the list and its images load`, async ({ page }) => {
    await mock(page);
    await observeLayoutShift(page);

    await page.goto(path);
    await settle(page);
    await scrollThrough(page);

    expect(await layoutShiftTotal(page)).toBeLessThan(0.1);
  });
}
