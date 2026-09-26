import { expect, test, type Locator, type Page } from "@playwright/test";

/**
 * UI 细节打磨（#549）：首页榜单对齐、档案卡缩写只作装饰、互助筛选分组、求职雷达标题
 * 不拆词且首屏不重复、验证码占位文字不加宽字距、登录卡不重复邮箱后缀。
 */

async function waitForHydration(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
}

/** 标题里每个字所在行的顶边（px）：顶边相同的字排在同一行。 */
async function charLineTops(heading: Locator): Promise<Record<string, number>> {
  return heading.evaluate((element) => {
    const tops: Record<string, number> = {};
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const text = node.textContent ?? "";
      for (let index = 0; index < text.length; index += 1) {
        const range = document.createRange();
        range.setStart(node, index);
        range.setEnd(node, index + 1);
        tops[text[index]] = Math.round(range.getBoundingClientRect().top);
      }
    }
    return tops;
  });
}

const FOOD_POSTS = [
  { id: "hang-1", tags: ["夜市", "夯"], shop: { name: "鼓楼夜市" } },
  { id: "hang-2", tags: ["夜市", "夯"], shop: { name: "西司夜市" } },
  { id: "top-1", tags: ["老字号", "顶级"], shop: { name: "第一楼" } },
  { id: "top-2", tags: ["面馆", "顶级"], shop: { name: "马豫兴" } },
  { id: "elite-1", tags: ["校内", "人上人"], shop: { name: "龙子湖食堂" } },
].map((post) => ({
  campus: "minglun",
  title: post.shop.name,
  excerpt: "学生视角的一句评价。",
  blocks: [{ type: "p", text: "正文。" }],
  author: "学生编辑部",
  likes: 10,
  stars: 5,
  time: "07-16",
  hidden: false,
  images: [],
  ...post,
}));

const LIBRARY_MATERIALS = [
  {
    id: "library-limits", type: "note", subject: "高等数学",
    title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
  },
];

const CAMPUS_ITEMS = [
  {
    id: "campus-express", type: "help", category: "express", title: "代取快递到南门", desc: "两件小包裹",
    price: 3, seller: "同学甲", credit: 0, dealsDone: 0, wants: 0, place: "明伦校区", status: "open", time: "2026-09-20",
  },
];

/** 未登录；美食、资料库、互助给出内容，其余接口不可用。 */
async function mockGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_polish_unavailable" },
    })
  );
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({ json: { posts: FOOD_POSTS, request_id: "req_polish_food" } })
  );
  await page.route("**/api/v1/campus/items", (route) =>
    route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_polish_campus" } })
  );
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_polish_categories" } })
  );
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      json: {
        materials: LIBRARY_MATERIALS,
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: LIBRARY_MATERIALS.length,
          downloadStarts: 12,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_polish_library",
      },
    })
  );
}

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGateway(page);
});

test("home 03 ranking lines every shop name up on one left edge", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/");
  await waitForHydration(page);

  const names = page.locator("[data-rank-row] a");
  await expect(names).toHaveCount(5);
  const lefts: number[] = [];
  for (const name of await names.all()) {
    const box = await name.boundingBox();
    lefts.push(Math.round(box?.x ?? -1));
  }
  expect(new Set(lefts).size, `店名左边缘：${lefts.join(", ")}`).toBe(1);
});

test("home 01 archive cards lead with the Chinese type name, not an abbreviation", async ({ page }) => {
  await page.goto("/");
  await waitForHydration(page);

  const section = page.locator("section").filter({ has: page.getByRole("heading", { name: "资料库", level: 2 }) });
  const card = section.locator("[data-lib-card]").filter({ has: page.getByRole("heading", { name: "笔记总结" }) });
  await expect(card).toHaveCount(1);
  // 缩写只作装饰：读屏不读，视觉上也不再用强调色。
  const code = card.getByText("NO", { exact: true });
  await expect(code).toHaveAttribute("aria-hidden", "true");
  // 查实际渲染的颜色而不是类名：强调色即使从父元素继承下来也能发现。
  const colour = await code.evaluate((element) => getComputedStyle(element).color);
  expect(colour).not.toBe("rgb(255, 77, 0)"); // --color-accent #ff4d00
});

test("campus filters are two labelled groups", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/campus");
  await waitForHydration(page);
  await expect(page.getByRole("heading", { name: "代取快递到南门" })).toBeVisible();

  const types = page.getByRole("group", { name: "单子类型" });
  const categories = page.getByRole("group", { name: "分类" });
  await expect(types.getByRole("button")).toHaveText(["全部", "求助单", "闲置单"]);
  await expect(categories.getByRole("button", { name: "全部", exact: true })).toHaveCount(1);
  await expect(categories.getByRole("button", { name: "代取快递", exact: true })).toBeVisible();
  // 标签看得见，不只写给读屏。
  await expect(page.getByText("单子类型", { exact: true })).toBeVisible();
  await expect(page.getByText("分类", { exact: true })).toBeVisible();
});

test("career headline keeps 招聘 on one line at 390px and states each point once", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/career");
  await waitForHydration(page);
  const guest = page.locator("[data-career-state='anonymous']");
  await expect(guest).toBeVisible();

  const heading = guest.getByRole("heading", { level: 1 });
  await expect(heading).toHaveText("让雷达替你扫一遍招聘信息");
  const tops = await charLineTops(heading);
  // 标题在 390px 下确实折行，而“招聘”两个字在同一行。
  expect(tops["息"]).toBeGreaterThan(tops["让"]);
  expect(tops["招"]).toBe(tops["聘"]);

  // 描述段不再把下面三条要点逐字说一遍。
  await expect(guest.getByText(/匹配结果与命中原因一目了然/)).toHaveCount(1);
  await expect(guest.getByText(/已验证的账户邮箱/)).toHaveCount(1);
});

test("free-member career headlines keep 终身会员权益 on one line at 390px", async ({ page }) => {
  // 已登录的免费会员：/career 与 /career/history 都显示“……属于终身会员权益”的说明。
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      json: {
        user_id: "11111111-1111-4111-8111-111111111111",
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      },
    })
  );
  await page.route("**/api/v1/account/membership", (route) =>
    route.fulfill({ json: { data: { plan: "free", lifetime: false }, request_id: "req_polish_membership" } })
  );
  await page.setViewportSize({ width: 390, height: 844 });

  for (const { path, state, text } of [
    { path: "/career", state: "[data-career-state='free']", text: "求职雷达属于终身会员权益" },
    { path: "/career/history", state: "[data-career-history-state='free']", text: "扫描历史属于终身会员权益" },
  ]) {
    await page.goto(path);
    await waitForHydration(page);
    const heading = page.locator(state).getByRole("heading", { level: 1 });
    await expect(heading).toHaveText(text);
    const tops = await charLineTops(heading);
    // 标题在 390px 下确实折行，而“终身会员权益”整组在同一行，“会员”不被拆开。
    expect(tops[text[text.length - 1]], path).toBeGreaterThan(tops[text[0]]);
    expect(new Set([..."终身会员权益"].map((char) => tops[char])).size, path).toBe(1);
  }
});

test("login code placeholder reads normally and the email suffix is stated once", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/account/login");
  await waitForHydration(page);

  const code = page.getByPlaceholder("6 位数字");
  await expect(code).toBeVisible();
  const spacing = await code.evaluate((input) => ({
    value: getComputedStyle(input).letterSpacing,
    placeholder: getComputedStyle(input, "::placeholder").letterSpacing,
  }));
  expect(spacing.value).not.toBe("normal");
  expect(spacing.placeholder).toBe("normal");

  // 输入框右侧已固定显示 @henu.edu.cn，卡片底部不再重复。
  await expect(page.locator("#auth-email-suffix")).toHaveText("@henu.edu.cn");
  await expect(page.getByText(/固定后缀/)).toHaveCount(0);
});
