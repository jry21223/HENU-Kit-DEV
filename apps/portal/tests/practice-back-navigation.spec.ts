import { expect, test, type Page } from "@playwright/test";

/**
 * 左上角的箭头是「返回上一级」：模块内页回到该模块首页，模块首页回到平台首页。
 * 落点页面还要回到读者上次离开时的滚动位置——浏览器只在历史前进/后退时自己恢复，
 * 而这是一次新的客户端导航，所以由 ScrollMemory 补上。
 */

const TIER_TAGS = ["夯", "顶级", "人上人", "NPC", "拉完了"];

/** 五档各若干条，让榜单有好几屏高。 */
const FOOD_POSTS = TIER_TAGS.flatMap((tag, tierIndex) =>
  Array.from({ length: 8 }, (_, row) => ({
    id: `post-${tierIndex}-${row}`,
    campus: "minglun",
    title: `${tag} 档第 ${row + 1} 家`,
    excerpt: "用于返回位置断言的条目。",
    blocks: [{ type: "p", text: "正文。" }],
    author: "学生编辑部",
    likes: 100 - row,
    stars: 10,
    tags: [tag],
    shop: { name: `${tag}-${row + 1} 号店` },
    time: "07-16",
    hidden: false,
    images: [],
  }))
);

async function mockFood(page: Page) {
  await page.route("**/api/v1/food/posts", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ posts: FOOD_POSTS, request_id: "req_back_food" }),
    })
  );
  await page.route("**/api/v1/food/posts/*", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        post: FOOD_POSTS[0],
        comments: [],
        request_id: "req_back_food_detail",
      }),
    })
  );
}

function scrollY(page: Page) {
  return page.evaluate(() => Math.round(window.scrollY));
}

/** 书库目录：够多张卡片，列表有好几屏高。 */
const LIBRARY_MATERIALS = Array.from({ length: 36 }, (_, index) => ({
  id: `library-back-${index}`,
  type: "note",
  subject: "高等数学",
  title: `返回位置用资料 ${index}`,
  author: "资料库收录",
  intro: "",
  toc: [],
  pages: [],
  price: 0,
  previewPages: 0,
  downloads: 1,
  downloadAvailable: true,
  fileSize: 1024,
}));

const materialCards = (page: Page) => page.locator('a[href^="/library/item/"]');

/** 点一张已经在屏幕里的资料卡。理由同 clickOnScreenVenue。 */
async function clickOnScreenMaterial(page: Page) {
  await page.evaluate(() => {
    const onScreen = Array.from(
      document.querySelectorAll<HTMLAnchorElement>('a[href^="/library/item/"]')
    ).find((link) => {
      const box = link.getBoundingClientRect();
      return box.top >= 0 && box.bottom <= window.innerHeight;
    });
    if (!onScreen) throw new Error("屏幕上没有可点的资料卡");
    onScreen.click();
  });
  await page.waitForURL(/\/library\/item\//);
}

async function scrollTo(page: Page, offset: number) {
  await page.evaluate((value) => window.scrollTo(0, value), offset);
  await expect.poll(() => scrollY(page)).toBeGreaterThan(offset - 40);
}

const backLink = (page: Page) => page.locator("header [data-back-link]").first();

/**
 * 等客户端外壳水合完成（ScrollMemory 挂上记录器才会打这个标记）。还没水合时
 * 点击会走整页刷新，阅读位置也还没被记下来——那是测试抢跑，不是读者行为。
 */
async function waitForClientShell(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
}

/**
 * 点一条已经在屏幕里的条目。理由同 food-scroll-restoration：Playwright 自己的
 * click 会先把目标滚进视口，那会改动被测的 offset。
 */
async function clickOnScreenVenue(page: Page): Promise<string> {
  const href = await page.evaluate(() => {
    const onScreen = Array.from(
      document.querySelectorAll<HTMLAnchorElement>('a[href^="/food/post/"]')
    ).find((link) => {
      const box = link.getBoundingClientRect();
      return box.top >= 0 && box.bottom <= window.innerHeight;
    });
    if (!onScreen) throw new Error("屏幕上没有可点的条目");
    onScreen.click();
    return onScreen.getAttribute("href") ?? "";
  });
  await page.waitForURL(/\/food\/post\//);
  return href;
}

/** 等页面长到滚得动这么多，再断言，免得把「还没渲染完」当成「恢复失败」。 */
async function waitForRoomToScroll(page: Page, needed: number) {
  await expect
    .poll(() =>
      page.evaluate(() =>
        Math.round(document.documentElement.scrollHeight - window.innerHeight)
      )
    )
    .toBeGreaterThan(needed);
}

/** 点导航里那个标签，而不是 Playwright 的 click（它可能先滚动，改动被测的 offset）。 */
async function clickNavTab(page: Page, href: string) {
  await page.evaluate((target) => {
    const tab = Array.from(
      document.querySelectorAll<HTMLAnchorElement>(`nav a[href="${target}"]`)
    )[0];
    if (!tab) throw new Error(`导航里没有 ${target}`);
    tab.click();
  }, href);
}

/**
 * 让返回列表的那一趟请求慢下来：落回列表时行还在路上，文档撑不下那个位置，
 * 恢复窗口开着。首屏那趟保持快，读者才能先滚到一个位置再离开。
 *
 * 用书库列表而不是美食榜单：榜单的资料在模块级缓存里，回程不再请求，行不会晚到。
 */
async function returnToSlowLibrary(page: Page, delayMs: number): Promise<number> {
  const state = { slow: false };
  await page.route("**/api/v1/library/materials", async (route) => {
    if (state.slow) {
      await new Promise((resolve) => setTimeout(resolve, delayMs));
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: LIBRARY_MATERIALS,
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: LIBRARY_MATERIALS.length,
          downloadStarts: 99,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_back_library",
      }),
    });
  });
  await page.route("**/api/v1/library/materials/*", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        material: LIBRARY_MATERIALS[0],
        request_id: "req_back_library_detail",
      }),
    })
  );

  await page.goto("/library", { waitUntil: "domcontentloaded" });
  await expect(materialCards(page).first()).toBeVisible();
  await waitForClientShell(page);

  await scrollTo(page, 1600);
  const departedFrom = await scrollY(page);
  state.slow = true;

  await clickOnScreenMaterial(page);
  await backLink(page).click();
  await expect(page).toHaveURL(/\/library$/);
  return departedFrom;
}

test.use({ viewport: { width: 390, height: 844 } });

/**
 * 每个子站的二次页：上一级是该子站首页，标签点名它要回的那一层。
 * 直接 goto 而不是从站内点进来——深链接进来和刷新后落点必须一样。
 */
const SUB_SITE_INNER_PAGES = [
  { route: "/library/item/free-ds-graph-note", label: "← 书库", destination: "/library" },
  { route: "/food/post/post-0-0", label: "← 榜单", destination: "/food" },
  { route: "/campus/item/h-01", label: "← 市集", destination: "/campus" },
  { route: "/career/history", label: "← 求职雷达", destination: "/career" },
  { route: "/practice/quiz", label: "← 题库", destination: "/practice" },
  { route: "/practice/favorites/bank-1", label: "← 收藏夹概览", destination: "/practice/favorites" },
] as const;

/**
 * 深层页的上一级是该子站首页，而不是平台首页；也就是平台首页自己的一级。
 * 深链接直达和站内进来走同一条路径规则，箭头和落点因此一致。
 */
const SUB_SITE_HOMES = [
  { route: "/library", label: "← henukit" },
  { route: "/food", label: "← henukit" },
  { route: "/campus", label: "← henukit" },
  { route: "/career", label: "← henukit" },
  { route: "/practice", label: "← henukit" },
] as const;

test("没走成的点击不会让读者之后的位移被丢掉", async ({ page }) => {
  await mockFood(page);
  await page.goto("/food", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-food-tier]")).toHaveCount(TIER_TAGS.length);
  await waitForClientShell(page);
  await scrollTo(page, 1600);

  // 拦下一次点击：导航没有发生（下载链接、别的处理函数 preventDefault 都长这样）。
  // 记录器如果据此认定"这一页要走了"并一直等一个 0，读者后续的真实位移就会被丢掉。
  await page.evaluate(() => {
    const swallow = (event: Event) => {
      event.preventDefault();
      event.stopPropagation();
    };
    document.addEventListener("click", swallow, true);
    window.setTimeout(() => document.removeEventListener("click", swallow, true), 1000);
  });
  await page.evaluate(() => {
    const onScreen = Array.from(
      document.querySelectorAll<HTMLAnchorElement>('a[href^="/food/post/"]')
    ).find((link) => {
      const box = link.getBoundingClientRect();
      return box.top >= 0 && box.bottom <= window.innerHeight;
    });
    if (!onScreen) throw new Error("屏幕上没有可点的条目");
    onScreen.click();
  });
  await page.waitForTimeout(400);
  expect(new URL(page.url()).pathname).toBe("/food");

  // 读者自己滚回顶部：这一次位移必须被记下来。
  // 这里断言记录值而不是界面结果：要把这份记录"带出去"需要一次没有点击的站内导航，
  // 页面上没有可点的入口；键与格式由 scroll-memory 的单测钉住。
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect
    .poll(() => page.evaluate(() => window.sessionStorage.getItem("henukit.scroll.v1:/food")))
    .toBe("0");
});

/**
 * 新标签（题库/书库/榜单/市集/求职雷达/收藏夹概览）只出现在这些内页上，
 * 所以"移动端无横向溢出"必须在这里量，而不是只量五个模块首页。
 */
test.describe("内页在窄屏下不横向溢出", () => {
  for (const { route } of SUB_SITE_INNER_PAGES) {
    test(`${route} 在 360px 下不溢出`, async ({ page }) => {
      await page.setViewportSize({ width: 360, height: 844 });
      await page.goto(route, { waitUntil: "domcontentloaded" });
      await expect(page.locator("header [data-back-link]").first()).toBeVisible();

      const metrics = await page.evaluate(() => ({
        clientWidth: document.documentElement.clientWidth,
        scrollWidth: document.documentElement.scrollWidth,
        headerOverflow: (() => {
          const header = document.querySelector("header");
          return header ? header.scrollWidth - header.clientWidth : 0;
        })(),
      }));
      expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth + 1);
      expect(metrics.headerOverflow).toBeLessThanOrEqual(1);
    });
  }
});

test("模块首页的返回箭头回平台首页，并落回读者离开时的位置", async ({ page }) => {
  await page.goto("/", { waitUntil: "domcontentloaded" });
  await waitForClientShell(page);
  await scrollTo(page, 1600);

  // 点首页上已经在屏幕里的入口。Playwright 自己的 click 会先把目标滚进视口，
  // 那会改动被测的 offset（读者点得到的东西本来就在屏幕上）。
  await page.evaluate(() => {
    const entry = Array.from(
      document.querySelectorAll<HTMLAnchorElement>('a[href="/practice"]')
    ).find((link) => link.closest('nav[aria-label="常用功能"]'));
    if (!entry) throw new Error("首页没有刷题入口");
    entry.click();
  });
  await expect(page).toHaveURL(/\/practice$/);
  // 新进入的页面从顶部开始，不能继承上一页的位置。
  expect(await scrollY(page)).toBeLessThan(40);

  await expect(backLink(page)).toHaveText("← henukit");
  await backLink(page).click();

  await expect(page).toHaveURL(/\/$/);
  await expect.poll(() => scrollY(page)).toBeGreaterThan(1400);
});

test("列表页的返回箭头回模块首页，并落回读者离开时的位置", async ({ page }) => {
  await mockFood(page);
  await page.goto("/food", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-food-tier]")).toHaveCount(TIER_TAGS.length);
  await waitForClientShell(page);
  await scrollTo(page, 1600);
  const departedFrom = await scrollY(page);

  // 点一条已经在屏幕里的条目，理由同上。
  await page.evaluate(() => {
    const onScreen = Array.from(
      document.querySelectorAll<HTMLAnchorElement>('a[href^="/food/post/"]')
    ).find((link) => {
      const box = link.getBoundingClientRect();
      return box.top >= 0 && box.bottom <= window.innerHeight;
    });
    if (!onScreen) throw new Error("屏幕上没有可点的条目");
    onScreen.click();
  });
  await page.waitForURL(/\/food\/post\//);
  expect(await scrollY(page)).toBeLessThan(40);

  await expect(backLink(page)).toHaveText("← 榜单");
  await backLink(page).click();

  await expect(page).toHaveURL(/\/food$/);
  // 榜单的行是客户端拉取的，恢复要等文档重新长到够高。
  await expect.poll(() => scrollY(page)).toBeGreaterThan(departedFrom - 100);
});

test("列表的行晚到，返回箭头仍然把读者放回离开时的位置", async ({ page }) => {
  const departedFrom = await returnToSlowLibrary(page, 800);

  // 返回的这一趟请求还在路上：文档现在根本滚不到读者离开的位置，
  // 恢复只能在行到了以后落地——快服务器证明不了这一段。
  await expect(materialCards(page)).toHaveCount(0);
  const roomWhileWaiting = await page.evaluate(() =>
    Math.round(document.documentElement.scrollHeight - window.innerHeight)
  );
  expect(roomWhileWaiting).toBeLessThan(40);
  expect(roomWhileWaiting).toBeLessThan(departedFrom);

  await expect(materialCards(page)).toHaveCount(LIBRARY_MATERIALS.length);
  await expect.poll(() => scrollY(page)).toBeGreaterThan(departedFrom - 60);
});

test("恢复期间读者一滚，位置立刻交给读者", async ({ page }) => {
  const departedFrom = await returnToSlowLibrary(page, 900);
  await expect(materialCards(page)).toHaveCount(0);

  // 恢复窗口还开着，行还在路上，读者自己动手。
  await page.mouse.move(195, 500);
  await page.mouse.wheel(0, 600);

  await expect(materialCards(page)).toHaveCount(LIBRARY_MATERIALS.length);
  await page.waitForTimeout(400);
  // 行到了以后也不许把读者拖回那个记住的位置。
  expect(await scrollY(page)).toBeLessThan(departedFrom - 400);
});

test("恢复期间读者一按键，位置立刻交给读者", async ({ page }) => {
  const departedFrom = await returnToSlowLibrary(page, 900);
  await expect(materialCards(page)).toHaveCount(0);

  await page.keyboard.press("End");

  await expect(materialCards(page)).toHaveCount(LIBRARY_MATERIALS.length);
  await page.waitForTimeout(400);
  expect(await scrollY(page)).toBeLessThan(departedFrom - 400);
});

test.describe("触摸接管", () => {
  test.use({ hasTouch: true });

  test("恢复期间读者一碰屏幕，位置立刻交给读者", async ({ page }) => {
    const departedFrom = await returnToSlowLibrary(page, 900);
    await expect(materialCards(page)).toHaveCount(0);

    // 点标题（不是链接）：真实的触摸输入，不会顺手导航走。
    const title = await page.locator("h1").first().boundingBox();
    if (!title) throw new Error("找不到书库标题");
    await page.touchscreen.tap(title.x + 12, title.y + 12);

    await expect(materialCards(page)).toHaveCount(LIBRARY_MATERIALS.length);
    await page.waitForTimeout(400);
    expect(await scrollY(page)).toBeLessThan(departedFrom - 400);
  });
});

test("再次进入更深的页面时从顶部开始，不继承它自己的位置", async ({ page }) => {
  await mockFood(page);
  await page.goto("/food", { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-food-tier]")).toHaveCount(TIER_TAGS.length);
  await waitForClientShell(page);

  await scrollTo(page, 1600);
  const departedFrom = await scrollY(page);
  const venueHref = await clickOnScreenVenue(page);
  // 深入：新页面从顶部开始。
  expect(await scrollY(page)).toBeLessThan(40);
  expect(departedFrom).toBeGreaterThan(1400);

  // 在条目页留下一个位置（这条路径因此有了记忆）。
  await scrollTo(page, 900);
  const leftAtVenue = await scrollY(page);
  expect(leftAtVenue).toBeGreaterThan(800);

  await backLink(page).click();
  await expect(page).toHaveURL(/\/food$/);
  await expect(page.locator("[data-food-tier]")).toHaveCount(TIER_TAGS.length);
  await expect.poll(() => scrollY(page)).toBeGreaterThan(1400);

  // 再深入同一条路径：它记得 900，但这是「更深」，必须从顶部开始。
  await page.evaluate((href) => {
    const link = document.querySelector<HTMLAnchorElement>(`a[href="${href}"]`);
    if (!link) throw new Error("榜单上找不到刚才那条条目");
    link.click();
  }, venueHref);
  await expect(page).toHaveURL(new RegExp(`${venueHref}$`));
  await expect.poll(() => scrollY(page)).toBeLessThan(40);
});

/**
 * 横向切换标签。练习区两个同级标签（收藏夹 / 数据）在 390x844 下不足一屏，
 * 把视口压到 390x400 才滚得动，「落回原处」和「从顶部开始」才有区别可断言。
 * 两个标签互不为对方的上一层，按「向上才恢复」都该从顶部开始。
 */
test.describe("横向切换标签", () => {
  test.use({ viewport: { width: 390, height: 400 } });

  test("从顶部开始，不继承另一条路径的位置", async ({ page }) => {
    await page.goto("/practice/favorites", { waitUntil: "domcontentloaded" });
    await waitForClientShell(page);
    await waitForRoomToScroll(page, 130);
    await scrollTo(page, 120);
    expect(await scrollY(page)).toBeGreaterThan(100);

    await clickNavTab(page, "/practice/stats");
    await expect(page).toHaveURL(/\/practice\/stats$/);
    await expect.poll(() => scrollY(page)).toBeLessThan(20);

    await waitForRoomToScroll(page, 110);
    await scrollTo(page, 100);
    expect(await scrollY(page)).toBeGreaterThan(80);

    // 横着切回收藏夹：它自己记得 120，但这不是返回上一级。
    await clickNavTab(page, "/practice/favorites");
    await expect(page).toHaveURL(/\/practice\/favorites$/);
    await page.waitForTimeout(400);
    expect(await scrollY(page)).toBeLessThan(20);
  });
});

test("刷题页的返回箭头回题库目录，而不是平台首页", async ({ page }) => {
  await page.goto("/practice/quiz", { waitUntil: "domcontentloaded" });
  await waitForClientShell(page);

  await expect(backLink(page)).toHaveText("← 题库");
  await expect(backLink(page)).toHaveAttribute("href", "/practice");
  await backLink(page).click();

  await expect(page).toHaveURL(/\/practice$/);
  await expect(backLink(page)).toHaveText("← henukit");
});

for (const { route, label, destination } of SUB_SITE_INNER_PAGES) {
  test(`${route} 的返回箭头回上一层 ${destination}`, async ({ page }) => {
    await mockFood(page);
    // 直接打开深层地址，前面没有任何站内导航：落点仍然由路径层级决定。
    await page.goto(route, { waitUntil: "domcontentloaded" });

    await expect(backLink(page)).toHaveText(label);
    await expect(backLink(page)).toHaveAttribute("href", destination);

    await backLink(page).click();

    await expect(page).toHaveURL(new RegExp(`${destination.replace(/\//g, "\\/")}$`));
    // 到了上一级，箭头再往上指一层：子站首页回平台首页，收藏夹概览回题库。
    await expect(backLink(page)).toHaveText(
      destination === "/practice/favorites" ? "← 题库" : "← henukit"
    );
  });
}

for (const { route, label } of SUB_SITE_HOMES) {
  test(`${route} 子站首页的返回箭头回平台首页`, async ({ page }) => {
    await page.goto(route, { waitUntil: "domcontentloaded" });

    await expect(backLink(page)).toHaveText(label);
    await expect(backLink(page)).toHaveAttribute("href", "/");

    await backLink(page).click();

    await expect(page).toHaveURL(/\/$/);
  });
}
