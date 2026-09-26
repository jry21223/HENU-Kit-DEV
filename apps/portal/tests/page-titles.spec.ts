import { expect, test, type Page } from "@playwright/test";

/**
 * 全站只有一种标题格式（#544）：`页面名 — 模块名 | HENU Kit`；模块首页和不属于任何模块的
 * 页面是 `页面名 | HENU Kit`。品牌只由根布局的模板补一次。除了同类详情页，任意两个页面
 * 的标题都不相同，标签页和搜索结果里一眼能分清。
 */
const TITLE_FORMAT = /^[^|—]+(?: — [^|—]+)? \| HENU Kit$/;

const PAGES = [
  { path: "/library", title: "资料库 | HENU Kit" },
  { path: "/library/shelf", title: "我的书架 — 资料库 | HENU Kit" },
  { path: "/library/item/titles-missing", title: "资料详情 — 资料库 | HENU Kit" },
  { path: "/practice", title: "智能刷题 | HENU Kit" },
  { path: "/practice/quiz", title: "练习 — 智能刷题 | HENU Kit" },
  { path: "/practice/leaderboard", title: "排行榜 — 智能刷题 | HENU Kit" },
  { path: "/practice/stats", title: "数据面板 — 智能刷题 | HENU Kit" },
  { path: "/practice/favorites", title: "收藏夹 — 智能刷题 | HENU Kit" },
  { path: "/practice/favorites/bank-titles", title: "题库收藏夹 — 智能刷题 | HENU Kit" },
  { path: "/food", title: "美食榜 | HENU Kit" },
  { path: "/food/publish", title: "提交推荐 — 美食榜 | HENU Kit" },
  { path: "/food/post/titles-missing", title: "锐评 — 美食榜 | HENU Kit" },
  { path: "/campus", title: "互助平台 | HENU Kit" },
  { path: "/campus/deals", title: "我的交易 — 互助平台 | HENU Kit" },
  { path: "/campus/publish", title: "发布单子 — 互助平台 | HENU Kit" },
  { path: "/campus/item/titles-missing", title: "单子详情 — 互助平台 | HENU Kit" },
  { path: "/career", title: "求职雷达 | HENU Kit" },
  { path: "/career/history", title: "扫描历史 — 求职雷达 | HENU Kit" },
  { path: "/account/login", title: "登录 | HENU Kit" },
  { path: "/account/recover", title: "找回密码 | HENU Kit" },
  { path: "/account", title: "账户中心 | HENU Kit" },
  { path: "/account/security", title: "安全设置 — 账户中心 | HENU Kit" },
  { path: "/account/wallet", title: "积分钱包 — 账户中心 | HENU Kit" },
  { path: "/account/membership", title: "会员权益 — 账户中心 | HENU Kit" },
  { path: "/account/tickets", title: "工单 — 账户中心 | HENU Kit" },
  { path: "/account/notifications", title: "系统通知 — 账户中心 | HENU Kit" },
  { path: "/account/posts", title: "我的投稿 — 账户中心 | HENU Kit" },
  { path: "/account/profile", title: "求职画像 — 账户中心 | HENU Kit" },
  { path: "/privacy", title: "隐私政策 | HENU Kit" },
  { path: "/terms", title: "用户协议 | HENU Kit" },
  { path: "/this-page-does-not-exist", title: "页面不存在 | HENU Kit" },
];

/**
 * 已登录（否则账户页和发布页会先跳去登录页），其余接口一律不可用：静态标题不依赖接口
 * 数据，详情页拿不到内容时停在失败态，标签页上仍是该类页面的名字。
 */
async function mockGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
        request_id: "req_titles_unavailable",
      }),
    })
  );
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: "11111111-1111-4111-8111-111111111111",
        display_name: "小河同学",
        expires_at: "2030-01-01T00:00:00Z",
      }),
    })
  );
}

test("no two pages share a title and every title follows the one site format", () => {
  const titles = PAGES.map((entry) => entry.title);
  for (const title of titles) expect(title).toMatch(TITLE_FORMAT);
  expect(new Set(titles).size).toBe(titles.length);
});

for (const { path, title } of PAGES) {
  test(`${path} is titled "${title}"`, async ({ page }) => {
    await mockGateway(page);
    await page.goto(path, { waitUntil: "domcontentloaded" });
    // 冷启动时路由要先编译；超时只用来盖住 dev 的编译时间。
    await expect(page).toHaveTitle(title, { timeout: 30_000 });
    // 水合后再确认一次：页面没有被客户端跳去别处，标题也没有被改掉。
    await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
    await expect(page).toHaveTitle(title);
  });
}

test("the home page keeps the site title", async ({ page }) => {
  await page.goto("/", { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("HENU Kit — 河南大学校园工具");
});

const MATERIAL = {
  id: "titles-material",
  type: "note",
  subject: "高等数学",
  title: "高等数学_笔记_极限复习笔记",
  author: "资料库收录",
  intro: "",
  toc: [],
  pages: [],
  price: 0,
  previewPages: 0,
  downloads: 1,
  downloadAvailable: true,
  fileSize: 1024,
};

const FOOD_POST = {
  id: "titles-post",
  campus: "minglun",
  title: "夜市逛吃指南",
  excerpt: "第一次来开封很适合从这里开始。",
  blocks: [{ type: "p", text: "选择多、烟火气足。" }],
  author: "学生编辑部",
  likes: 1,
  stars: 1,
  tags: ["夯"],
  shop: { name: "鼓楼夜市" },
  time: "2026-07-16",
  hidden: false,
  images: [],
};

const CAMPUS_ITEM = {
  id: "titles-item",
  type: "help",
  category: "errand",
  title: "代拿快递：东门到明伦校区",
  desc: "今晚八点前送到宿舍楼下。",
  price: 5,
  seller: "小河同学",
  credit: 100,
  dealsDone: 3,
  wants: 0,
  place: "明伦校区",
  status: "open",
  time: "07-16",
};

const DETAILS = [
  {
    path: `/library/item/${MATERIAL.id}`,
    endpoint: `**/api/v1/library/materials/${MATERIAL.id}`,
    body: { material: MATERIAL, request_id: "req_titles_material" },
    title: "极限复习笔记 — 资料库 | HENU Kit",
    fallback: "资料详情 — 资料库 | HENU Kit",
    back: { href: "/library", title: "资料库 | HENU Kit" },
  },
  {
    path: `/food/post/${FOOD_POST.id}`,
    endpoint: `**/api/v1/food/posts/${FOOD_POST.id}`,
    body: { post: FOOD_POST, comments: [], request_id: "req_titles_post" },
    title: "鼓楼夜市 — 美食榜 | HENU Kit",
    fallback: "锐评 — 美食榜 | HENU Kit",
    back: { href: "/food", title: "美食榜 | HENU Kit" },
  },
  {
    path: `/campus/item/${CAMPUS_ITEM.id}`,
    endpoint: `**/api/v1/campus/items/${CAMPUS_ITEM.id}`,
    body: { item: CAMPUS_ITEM, messages: [], request_id: "req_titles_item" },
    title: "代拿快递：东门到明伦校区 — 互助平台 | HENU Kit",
    fallback: "单子详情 — 互助平台 | HENU Kit",
    back: { href: "/campus", title: "互助平台 | HENU Kit" },
  },
  // 题库收藏夹的页头是题库名（来自收藏夹列表链接上的 ?name=），标签页跟着写上。
  {
    path: `/practice/favorites/titles-bank?name=${encodeURIComponent("高等数学")}`,
    endpoint: "**/api/v1/practice/banks/titles-bank/favorites",
    body: { data: [], request_id: "req_titles_folder" },
    title: "高等数学 收藏夹 — 智能刷题 | HENU Kit",
    fallback: "题库收藏夹 — 智能刷题 | HENU Kit",
    back: { href: "/practice/favorites", title: "收藏夹 — 智能刷题 | HENU Kit" },
  },
];

for (const detail of DETAILS) {
  test(`${detail.path} names its content in the title once it loads`, async ({ page }) => {
    await mockGateway(page);
    await page.route(detail.endpoint, (route) =>
      route.fulfill({ contentType: "application/json", body: JSON.stringify(detail.body) })
    );

    await page.goto(detail.path, { waitUntil: "domcontentloaded" });
    await expect(page).toHaveTitle(detail.title, { timeout: 30_000 });
    expect(detail.title).toMatch(TITLE_FORMAT);

    // Next 的 metadata 是流式到达的：站内跳转时，这一类页面的静态 <title> 可能比内容还晚
    // 挂上（React 把新挂上的 <title> 插在最前面）。照这样插一个，标签页上仍是内容名。
    await page.evaluate((fallback) => {
      const late = document.createElement("title");
      late.textContent = fallback;
      document.head.insertBefore(late, document.querySelector("head > title"));
    }, detail.fallback);
    await expect(page).toHaveTitle(detail.title);

    // 离开详情页后，标签页换成落点页自己的标题，不留着上一条内容的名字。
    await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
    await page.locator("header [data-back-link]").first().click();
    await expect(page).toHaveURL(new RegExp(`${detail.back.href}$`));
    await expect(page).toHaveTitle(detail.back.title);
  });
}

/**
 * 内容名来自用户投稿，可能带连续空格或首尾空格。document.title 读回来的是整理过空白的文字，
 * 标题要按读回来的样子写，不能因为对不上而一直重写、把页面卡死。
 */
test("a content name with irregular spacing is titled as the tab shows it, without endless rewrites", async ({ page }) => {
  // 数一数标题被写了几次；写到第 50 次还在写就不再真正写入，免得死循环把浏览器卡住。
  await page.addInitScript(() => {
    const native = Object.getOwnPropertyDescriptor(Document.prototype, "title")!;
    const counter = { writes: 0 };
    (window as unknown as { __titleWrites: typeof counter }).__titleWrites = counter;
    Object.defineProperty(document, "title", {
      configurable: true,
      get() {
        return native.get!.call(this);
      },
      set(value: string) {
        counter.writes += 1;
        if (counter.writes <= 50) native.set!.call(this, value);
      },
    });
  });
  const post = { ...FOOD_POST, id: "titles-post-spaced", shop: { name: " 鼓楼  夜市 " } };
  await mockGateway(page);
  await page.route(`**/api/v1/food/posts/${post.id}`, (route) =>
    route.fulfill({ json: { post, comments: [], request_id: "req_titles_spaced" } })
  );

  await page.goto(`/food/post/${post.id}`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("鼓楼 夜市 — 美食榜 | HENU Kit", { timeout: 30_000 });
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
  // 稍晚挂上的静态 <title> 仍会被改回内容名；只补写几次，不会一直写下去（没修好时会写满 50 次）。
  await page.evaluate(() => {
    const late = document.createElement("title");
    late.textContent = "锐评 — 美食榜 | HENU Kit";
    document.head.insertBefore(late, document.querySelector("head > title"));
  });
  await expect(page).toHaveTitle("鼓楼 夜市 — 美食榜 | HENU Kit");
  const writes = await page.evaluate(
    () => (window as unknown as { __titleWrites: { writes: number } }).__titleWrites.writes
  );
  expect(writes).toBeLessThan(10);
});

const NEXT_MATERIAL = {
  ...MATERIAL,
  id: "titles-material-next",
  title: "高等数学_笔记_导数复习笔记",
};

test("moving on to another item of the same kind drops the previous item's name", async ({ page }) => {
  await mockGateway(page);
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: [MATERIAL, NEXT_MATERIAL],
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: 2,
          downloadStarts: 0,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_titles_catalog",
      }),
    })
  );
  await page.route(`**/api/v1/library/materials/${MATERIAL.id}`, (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ material: MATERIAL, request_id: "req_titles_material" }),
    })
  );

  await page.goto("/library", { waitUntil: "domcontentloaded" });
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1);
  await page.getByRole("link", { name: /极限复习笔记/ }).click();
  await expect(page).toHaveTitle("极限复习笔记 — 资料库 | HENU Kit", { timeout: 30_000 });

  // 下一条没能加载：标签页回到这一类页面的名字，不挂着上一条内容的名字。
  await page.getByRole("link", { name: /导数复习笔记/ }).click();
  await expect(page).toHaveURL(new RegExp(`/library/item/${NEXT_MATERIAL.id}$`));
  await expect(page.getByText("资料详情暂时无法加载，请稍后重试。")).toBeVisible();
  await expect(page).toHaveTitle("资料详情 — 资料库 | HENU Kit");
});
