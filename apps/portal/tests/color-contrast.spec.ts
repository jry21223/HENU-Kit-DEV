import { expect, test, type Locator, type Page } from "@playwright/test";
import { contrastViolations, revealTextBackgrounds } from "./support/color-contrast";

/**
 * 文字对比度达到 WCAG AA（#536；DESIGN_SYSTEM.md 第 3 节“文字配色”、第 13 节）：axe 的
 * color-contrast 规则在首页、五个子站首页和登录页上为 0，桌面 1440 与手机 390 两种宽度都算。
 * 检查范围和扫描前的准备见 tests/support/color-contrast.ts。
 *
 * - 网关按有内容的回应 mock，列表、筛选、卡片和档位标签都渲染出来；减少动态设置下没有播到一半的动画。
 * - 首页逐屏滚到每个模块后再扫整页：固定页头是半透明纸白，压在墨色的刷题模块上时底色会变深，
 *   每个滚动位置都要算。
 */

/** 加载失败的投稿照片：列表里换成回退图块，图块上的小字也要检查。 */
const BROKEN_PHOTO = "/contrast-broken-photo.jpg";

const FOOD_POSTS = [
  { id: "hang-1", campus: "minglun", tags: ["夜市", "夯"], shop: { name: "鼓楼夜市" }, images: [BROKEN_PHOTO] },
  { id: "top-1", campus: "jinming", tags: ["夜市", "顶级"], shop: { name: "西司夜市" } },
  { id: "elite-1", campus: "minglun", tags: ["老字号", "人上人"], shop: { name: "第一楼" } },
  { id: "npc-1", campus: "longzihu", tags: ["校内", "NPC"], shop: { name: "龙子湖食堂" } },
  { id: "bad-1", campus: "longzihu", tags: ["待复核", "拉完了"], shop: { name: "待复核餐饮圈" } },
].map((post) => ({
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

const CAMPUS_ITEMS = [
  {
    id: "campus-express", type: "help", category: "express", title: "代取快递到南门", desc: "两件小包裹",
    price: 3, seller: "同学甲", credit: 0, dealsDone: 0, wants: 0, place: "明伦校区", status: "open", time: "2026-09-20",
  },
  {
    id: "campus-bookcase", type: "sell", category: "flea", title: "九成新书架", desc: "宿舍搬家出",
    price: 20, seller: "同学乙", credit: 0, dealsDone: 0, wants: 0, place: "金明校区", status: "open", time: "2026-09-21",
  },
];

const LIBRARY_MATERIALS = [
  {
    id: "library-limits", type: "note", subject: "高等数学",
    title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 12, downloadAvailable: true, fileSize: 4096,
  },
  {
    id: "library-final", type: "exam", subject: "高等数学",
    title: "高数期末真题", author: "资料库收录", intro: "", toc: [], pages: [],
    price: 0, previewPages: 0, downloads: 30, downloadAvailable: true, fileSize: 8192,
  },
];

/** 未登录；美食、互助、资料库给出有内容的回应，其余接口不可用。 */
async function mockGatewayWithContent(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_contrast_unavailable" },
    })
  );
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, json: {} }));
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({ json: { posts: FOOD_POSTS, request_id: "req_contrast_food" } })
  );
  await page.route("**/api/v1/campus/items", (route) =>
    route.fulfill({ json: { items: CAMPUS_ITEMS, request_id: "req_contrast_campus" } })
  );
  await page.route("**/api/v1/campus/categories", (route) =>
    route.fulfill({ json: { categories: [], request_id: "req_contrast_categories" } })
  );
  await page.route(`**${BROKEN_PHOTO}`, (route) => route.fulfill({ status: 404 }));
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      json: {
        materials: LIBRARY_MATERIALS,
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: LIBRARY_MATERIALS.length,
          downloadStarts: 42,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_contrast_library",
      },
    })
  );
}

async function waitForHydration(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
}

const VIEWPORTS = [
  { label: "1440", width: 1440, height: 900 },
  { label: "390", width: 390, height: 844 },
] as const;

const SUB_SITES: Array<{ route: string; ready: (page: Page) => Locator }> = [
  { route: "/library", ready: (page) => page.getByRole("link", { name: /极限复习笔记/ }) },
  // 默认构建里题库目录关闭，正文落在空状态。生产构建开着目录：题库卡片和加载失败提示
  // 由 quizcraft-catalog.spec.ts 在目录开启的 dev server 上检查（test:e2e:quizcraft-catalog）。
  { route: "/practice", ready: (page) => page.getByText("暂无题库") },
  // 等到第一条的照片加载失败、换成回退图块：图块的小字压在缩略图框的 4% 墨色底上。
  { route: "/food", ready: (page) => page.getByText("T-01 / 暂无图片") },
  { route: "/campus", ready: (page) => page.getByRole("heading", { name: "代取快递到南门" }) },
  // 等到落定的未登录介绍页，而不是先出现的加载块（它也带 data-career-state）。
  { route: "/career", ready: (page) => page.locator("[data-career-state='anonymous']") },
  { route: "/account/login", ready: (page) => page.getByRole("button", { name: "发送验证码" }) },
];

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGatewayWithContent(page);
});

for (const viewport of VIEWPORTS) {
  test.describe(`${viewport.label}px`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    test("首页每一屏的文字对比度都达到 AA", async ({ page }) => {
      await page.goto("/");
      await waitForHydration(page);
      await expect(page.locator("[data-rank-row]")).toHaveCount(5);
      await expect(page.locator("section").getByRole("heading", { name: "笔记总结" })).toBeVisible();

      await revealTextBackgrounds(page);

      const screens = page.locator(".snap-screen");
      const count = await screens.count();
      expect(count, "首屏、五个模块和页脚").toBe(7);

      const violations = new Set<string>();
      for (let index = 0; index < count; index += 1) {
        // 滚动后等一帧：页头的滚动状态在下一帧才更新。
        await screens.nth(index).evaluate(async (element) => {
          element.scrollIntoView({ block: "start" });
          await new Promise(requestAnimationFrame);
        });
        for (const violation of await contrastViolations(page)) violations.add(violation);
      }
      expect([...violations], "首页：以下文字的对比度低于 WCAG AA").toEqual([]);
    });

    for (const { route, ready } of SUB_SITES) {
      test(`${route} 的文字对比度达到 AA`, async ({ page }) => {
        await page.goto(route);
        await waitForHydration(page);
        await expect(ready(page)).toBeVisible();
        await revealTextBackgrounds(page);
        expect(await contrastViolations(page), `${route}：以下文字的对比度低于 WCAG AA`).toEqual([]);
      });
    }
  });
}

/**
 * 悬停中的控件只扫它自己。它的底色可能正是读屏隐藏的色块（磁吸按钮滑入的橙色填充），
 * 所以这里不隐藏装饰；过渡直接跳到终态，悬停后的颜色立刻可测。
 */
async function hoverViolations(page: Page, target: Locator): Promise<string[]> {
  await target.scrollIntoViewIfNeeded();
  await target.hover();
  await target.evaluate((element) => element.setAttribute("data-contrast-hover", ""));
  const violations = await contrastViolations(page, "[data-contrast-hover]");
  await target.evaluate((element) => element.removeAttribute("data-contrast-hover"));
  return violations;
}

test.describe("悬停", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test("悬停后的按钮和链接文字同样达到 AA：强调橙底上是墨色字，浅色底上的橙字更深", async ({ page }) => {
    const violations: string[] = [];
    const section = (title: string) =>
      page.locator("section").filter({ has: page.getByRole("heading", { name: title, level: 2 }) });

    await page.goto("/");
    await waitForHydration(page);
    await expect(page.locator("[data-rank-row]")).toHaveCount(5);
    await page.addStyleTag({ content: "*, *::before, *::after { transition: none !important; }" });
    for (const target of [
      // 磁吸按钮悬停时橙色填充滑入：墨色模块里原本是纸白字，纸白模块里原本是墨色字。
      section("智能刷题").getByRole("link", { name: "进入模块" }),
      section("美食排行榜").getByRole("link", { name: "进入模块" }),
      // 纸白底上的链接悬停变橙。
      section("美食排行榜").getByRole("link", { name: "鼓楼夜市" }),
    ]) {
      violations.push(...(await hoverViolations(page, target)));
    }

    await page.goto("/food");
    await waitForHydration(page);
    await expect(page.getByRole("link", { name: /鼓楼夜市/ }).first()).toBeVisible();
    await page.addStyleTag({ content: "*, *::before, *::after { transition: none !important; }" });
    for (const target of [
      // 墨色主按钮悬停转强调橙底。
      page.getByRole("link", { name: "提交推荐 →" }),
      // 五档导览的格子悬停整格变强调橙。
      page.getByRole("navigation", { name: "五档榜单导览" }).getByRole("link").first(),
    ]) {
      violations.push(...(await hoverViolations(page, target)));
    }

    expect(violations, "悬停后以下文字的对比度低于 WCAG AA").toEqual([]);
  });
});
