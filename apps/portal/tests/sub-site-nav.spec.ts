import { expect, test, type Locator, type Page } from "@playwright/test";

/**
 * 五个子站共用一条页头（SubSiteNav，#538）：页头与正文使用同一个内容框，
 * 所以左上角的返回链接与正文第一个标题左缘对齐；首页页头也用这个内容框，
 * 从首页进入子站时品牌位置不跳。
 */

const quizBankID = "33333333-3333-4333-8333-333333333333";
const quizBankVersionID = "44444444-4444-4444-8444-444444444444";

/**
 * 子站首页，以及刷题里铺满内容框的内页——刷题页头过去用的是另一种宽度，
 * 这几页页头与正文全都错开。
 */
const ALIGNED_PAGES = [
  "/library",
  "/food",
  "/campus",
  "/career",
  "/practice",
  "/practice/stats",
  "/practice/favorites",
  "/practice/favorites/bank-1",
] as const;

/**
 * 刷题里较窄的单栏页：在内容框里限宽，第一块内容（标签或卡片）的左缘仍与
 * 返回链接对齐。标题在带边框的卡片里，所以量的是第一块，不是标题。
 */
const NARROW_PRACTICE_PAGES = [
  // 没带题库参数：「刷题」标签的落点。
  { route: "/practice/quiz", ready: "请先选择题库" },
  // 题库版本确认不了（接口一律不可用）：组卷设置的出错分支。
  {
    route: `/practice/quiz?bank_id=${quizBankID}&bank_version_id=${quizBankVersionID}`,
    ready: "暂时无法确认题库版本",
  },
] as const;

const WIDE_VIEWPORTS = [1440, 1920] as const;

/** 内容框 1440px 居中，md 起左右各留 32px：宽屏下左缘应在这里。 */
const frameLeft = (width: number) => Math.max(0, (width - 1440) / 2) + 32;

/** 未登录，其余接口一律不可用：页头和正文标题不依赖接口数据。 */
async function mockGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
        request_id: "req_nav_unavailable",
      }),
    })
  );
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: "{}" })
  );
}

const backLink = (page: Page) => page.locator("header [data-back-link]").first();

async function leftEdge(locator: Locator): Promise<number> {
  const box = await locator.boundingBox();
  if (!box) throw new Error("元素没有渲染出来");
  return box.x;
}

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGateway(page);
});

for (const width of WIDE_VIEWPORTS) {
  test.describe(`${width}px 宽屏`, () => {
    test.use({ viewport: { width, height: 900 } });

    for (const route of ALIGNED_PAGES) {
      test(`${route} 的返回链接与正文首个标题左缘对齐`, async ({ page }) => {
        await page.goto(route, { waitUntil: "domcontentloaded" });
        const heading = page.locator("main").getByRole("heading").first();
        // 冷启动时这条路由要先编译；超时只用来盖住 dev 的编译时间。
        await expect(heading).toBeVisible({ timeout: 30_000 });

        const backX = await leftEdge(backLink(page));
        const headingX = await leftEdge(heading);
        expect(Math.abs(backX - headingX), `返回链接 x=${backX}，正文标题 x=${headingX}`).toBeLessThanOrEqual(1);
        // 两边都铺满视口也会「对齐」；再确认它们对齐在内容框上。
        expect(Math.abs(backX - frameLeft(width))).toBeLessThanOrEqual(1);
      });
    }

    for (const { route, ready } of NARROW_PRACTICE_PAGES) {
      test(`${route} 的单栏正文左缘与返回链接对齐`, async ({ page }) => {
        await page.goto(route, { waitUntil: "domcontentloaded" });
        await expect(page.getByRole("heading", { name: ready })).toBeVisible({ timeout: 30_000 });

        const firstBlock = page.locator("main [data-enter]").first();
        const backX = await leftEdge(backLink(page));
        const bodyX = await leftEdge(firstBlock);
        expect(Math.abs(backX - bodyX), `返回链接 x=${backX}，正文 x=${bodyX}`).toBeLessThanOrEqual(1);
        expect(Math.abs(backX - frameLeft(width))).toBeLessThanOrEqual(1);
      });
    }

    test("首页品牌与子站返回链接落在同一条左缘上", async ({ page }) => {
      await page.goto("/", { waitUntil: "domcontentloaded" });
      const brand = page.locator("header").getByRole("link", { name: /henukit/ }).first();
      await expect(brand).toBeVisible({ timeout: 30_000 });
      const homeX = await leftEdge(brand);

      await page.goto("/library", { waitUntil: "domcontentloaded" });
      await expect(backLink(page)).toBeVisible({ timeout: 30_000 });
      const subSiteX = await leftEdge(backLink(page));

      expect(Math.abs(homeX - subSiteX), `首页品牌 x=${homeX}，子站返回链接 x=${subSiteX}`).toBeLessThanOrEqual(1);
    });
  });
}

test.describe("刷题进行中", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  const bankID = quizBankID;
  const bankVersionID = quizBankVersionID;
  const sessionID = "22222222-2222-4222-8222-222222222222";
  const questionID = "55555555-5555-4555-8555-555555555555";
  const questionVersionID = "66666666-6666-4666-8666-666666666666";

  test("组卷、答题和结算三屏的正文都与页头左缘对齐", async ({ page }) => {
    await page.route("**/api/v1/practice/catalog", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          banks: [{
            bank_id: bankID,
            bank_version_id: bankVersionID,
            name: "计算机基础",
            question_count: 42,
            available: true,
            chapters: [{ id: "chapter-1", name: "真实章节" }],
          }],
          request_id: "req_nav_catalog",
        }),
      })
    );
    await page.route("**/api/v1/practice/sessions", (route) =>
      route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          request_id: "req_nav_session",
          data: {
            session_id: sessionID,
            bank_id: bankID,
            bank_version_id: bankVersionID,
            mode: "random",
            excluded_unavailable_count: 0,
            questions: [{
              question_id: questionID,
              question_version_id: questionVersionID,
              type: "single",
              chapter_id: "chapter-1",
              chapter: "真实章节",
              content: "用于对齐断言的题目。",
              options: ["甲", "乙"],
            }],
          },
        }),
      })
    );
    await page.route(`**/api/v1/practice/sessions/${sessionID}/answers`, (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          request_id: "req_nav_answer",
          data: {
            question_id: questionID,
            question_version_id: questionVersionID,
            correct: true,
            replayed: false,
            expected_answer: 0,
            analysis: "",
          },
        }),
      })
    );

    const expectAlignedWithBackLink = async (body: Locator, what: string) => {
      await expect(body).toBeVisible();
      const backX = await leftEdge(backLink(page));
      const bodyX = await leftEdge(body);
      expect(Math.abs(backX - bodyX), `返回链接 x=${backX}，${what} x=${bodyX}`).toBeLessThanOrEqual(1);
    };

    await page.goto(
      `/practice/quiz?bank_id=${bankID}&bank_version_id=${bankVersionID}&mode=random&question_count=1`,
      { waitUntil: "domcontentloaded" }
    );
    const setupHeading = page.getByRole("heading", { name: "组卷设置" });
    await expect(setupHeading).toBeVisible({ timeout: 30_000 });
    await expect(page.getByTestId("practice-session-setup")).toHaveAttribute("aria-busy", "false");
    await expectAlignedWithBackLink(setupHeading, "组卷设置标题");

    await page.getByLabel("题数 / COUNT").fill("1");
    await page.getByRole("button", { name: "开始练习 →" }).click();
    await expect(page.getByRole("heading", { name: "用于对齐断言的题目。" })).toBeVisible();

    // 答题页的第一块是进度行（第 1 / 1 题），它的左缘就是正文左缘。
    await expectAlignedWithBackLink(page.locator("main > *").first(), "答题进度行");

    // 读者正在这一页答题，「刷题」就是当前标签，不能同时说它「未开放」。
    await expect(page.locator("header").getByText("未开放", { exact: true })).toHaveCount(0);

    await page.getByRole("button", { name: /甲/ }).click();
    await page.getByRole("button", { name: "确认", exact: true }).click();
    await page.getByRole("button", { name: "查看结算 →" }).click();
    await expectAlignedWithBackLink(
      page.locator("main p").filter({ hasText: "本组结算" }).first(),
      "结算页标签"
    );
  });
});

test.describe("390px 手机", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test("/library 只有一个标签，不渲染标签行", async ({ page }) => {
    await page.goto("/library", { waitUntil: "domcontentloaded" });
    const header = page.locator("header").first();
    await expect(header.locator("[data-back-link]")).toBeVisible({ timeout: 30_000 });

    await expect(header.locator("nav")).toHaveCount(0);
    await expect(header.getByRole("link", { name: /L-01/ })).toHaveCount(0);
    // 账户入口仍然在页头那一行里；它和首页菜单的账户行一样是 flex 链接，读出的名字相同。
    await expect(header.getByRole("link", { name: "登录 / 注册", exact: true })).toBeVisible();
    const box = await header.boundingBox();
    // 一行页头：56px 行高 + 1px 底边。
    expect(box?.height ?? 0).toBeLessThanOrEqual(57);
  });

  test("标签行放不下时，当前标签被滑进可见范围", async ({ page }) => {
    // 刷题四个标签加上「未开放」标注，390px 放不下，最右边的「数据」原本会被裁掉。
    await page.goto("/practice/stats", { waitUntil: "domcontentloaded" });
    const nav = page.locator("header nav");
    const current = nav.getByRole("link", { name: /P-04/ });
    await expect(current).toBeVisible({ timeout: 30_000 });

    await expect
      .poll(async () => {
        const [navBox, tabBox] = await Promise.all([nav.boundingBox(), current.boundingBox()]);
        if (!navBox || !tabBox) return Number.POSITIVE_INFINITY;
        return tabBox.x + tabBox.width - (navBox.x + navBox.width);
      })
      .toBeLessThanOrEqual(0);
    // 标签行是自己横向滑动的，页面本身不横向溢出。
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth
    );
    expect(overflow).toBeLessThanOrEqual(1);
  });

  for (const width of [360, 390] as const) {
    test(`${width}px 从「数据」回到题库，当前标签不被裁在左缘`, async ({ page }) => {
      // 刷题页头在 /practice/* 之间一直挂着，标签行的横向位置会跟着带过来。
      await page.setViewportSize({ width, height: 844 });
      await page.goto("/practice/stats", { waitUntil: "domcontentloaded" });
      await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
      const nav = page.locator("header nav");
      // 先确认标签行确实为「数据」往右滑过，否则测不到左缘。
      await expect.poll(() => nav.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);

      await backLink(page).click();
      await expect(page).toHaveURL(/\/practice$/);
      const current = nav.getByRole("link", { name: /P-01/ });
      await expect
        .poll(async () => {
          const [navBox, tabBox] = await Promise.all([nav.boundingBox(), current.boundingBox()]);
          if (!navBox || !tabBox) return Number.NEGATIVE_INFINITY;
          return tabBox.x - navBox.x;
        })
        .toBeGreaterThanOrEqual(0);
    });
  }

  test("有多个标签的子站仍保留标签行", async ({ page }) => {
    await page.goto("/food", { waitUntil: "domcontentloaded" });
    const nav = page.locator("header nav");
    await expect(nav).toBeVisible({ timeout: 30_000 });
    await expect(nav.getByRole("link", { name: /F-01/ })).toBeVisible();
    await expect(nav.getByRole("link", { name: /F-02/ })).toBeVisible();
  });
});

/**
 * 题库目录未开放时，「刷题」标签不能点；原因过去只写在 title 属性里，
 * 触屏和键盘都看不到。现在标签旁直接写「未开放」。
 */
for (const width of [390, 1440] as const) {
  test(`${width}px 下刷题标签可见地标注未开放`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/practice/stats", { waitUntil: "domcontentloaded" });
    const nav = page.locator("header nav");
    await expect(nav).toBeVisible({ timeout: 30_000 });

    const quizTab = nav.locator("[data-tab-unavailable]").filter({ hasText: "刷题" });
    await expect(quizTab).toHaveCount(1);
    await expect(quizTab.getByText("未开放", { exact: true })).toBeVisible();
    await expect(nav.locator('a[href="/practice/quiz"]')).toHaveCount(0);
  });
}
