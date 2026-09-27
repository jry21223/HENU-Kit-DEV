import { expect, test, type Page } from "@playwright/test";

/**
 * 读屏软件读得出名字和状态：
 * - 输入框由看得见的标签命名，placeholder 不代替标签，搜索框的 placeholder 只举例（DESIGN_SYSTEM §11）；
 *   互助发布表单的 <label> 都连着控件，分类是一组有名字的按钮；
 * - 子站页头的标签行和账户中心的菜单用 aria-current 标出当前页；
 * - 资料目录的展开按钮用 aria-expanded 报告开合；
 * - 已登录时页头的账户入口读出完整昵称，而不只是头像块上的一个字。
 * 排行榜周期切换只在 V2 读取开启时出现，它的组名由 practice-leaderboard-live.spec.ts 检查。
 */

async function waitForHydration(page: Page) {
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
}

/** 接口一律不可用；signedIn 时会话有效，否则未登录。 */
async function mockGateway(page: Page, { signedIn = false } = {}) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      json: { error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_names_unavailable" },
    })
  );
  await page.route("**/api/v1/session", (route) =>
    signedIn
      ? route.fulfill({
          json: {
            user_id: "11111111-1111-4111-8111-111111111111",
            display_name: "小河同学",
            expires_at: "2030-01-01T00:00:00Z",
          },
        })
      : route.fulfill({ status: 401, json: {} })
  );
}

test.use({ contextOptions: { reducedMotion: "reduce" } });

test("资料库、互助和题库的搜索框由看得见的标签命名", async ({ page }) => {
  await mockGateway(page);

  await page.goto("/campus");
  await waitForHydration(page);
  const campusSearch = page.getByRole("textbox", { name: "搜索单子", exact: true });
  await expect(campusSearch).toBeVisible();
  await expect(page.getByText("搜索单子", { exact: true })).toBeVisible();

  await page.goto("/practice");
  await waitForHydration(page);
  const practiceSearch = page.getByRole("textbox", { name: "SEARCH / 搜索科目", exact: true });
  await expect(practiceSearch).toBeVisible();
  // 标签和输入框连在一起：点标签就聚焦输入框。
  await page.getByText("搜索科目").click();
  await expect(practiceSearch).toBeFocused();

  // 标签已经说了「搜索什么」，placeholder 只举例子，不再重复「搜索」。
  await expect(practiceSearch).toHaveAttribute("placeholder", /^如：/);
  await page.goto("/campus");
  await waitForHydration(page);
  await expect(campusSearch).toHaveAttribute("placeholder", /^如：/);
  await page.goto("/library");
  await waitForHydration(page);
  await expect(page.getByRole("searchbox", { name: "搜索资料", exact: true })).toHaveAttribute("placeholder", /^如：/);
});

test("互助发布表单的每个字段都读得出自己的标签，分类是一组有名字的按钮", async ({ page }) => {
  await mockGateway(page, { signedIn: true });
  await page.goto("/campus/publish");
  await waitForHydration(page);
  await expect(page.getByRole("heading", { name: "发布单子" })).toBeVisible();

  for (const name of ["标题", "描述", "赏金（元）", "位置", "时限（可选）"]) {
    await expect(page.getByRole("textbox", { name, exact: true }), name).toBeVisible();
  }
  await page.getByText("位置", { exact: true }).click();
  await expect(page.getByRole("textbox", { name: "位置", exact: true })).toBeFocused();

  // 和 /campus 的筛选一样：组名读得出，选中的那一个报告「已按下」。
  const category = page.getByRole("group", { name: "分类" });
  await expect(category.getByRole("button", { name: "跑腿代办" })).toHaveAttribute("aria-pressed", "true");
  await expect(category.getByRole("button", { name: "代取快递" })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByRole("button", { name: /发求助单/ })).toHaveAttribute("aria-pressed", "true");

  // 页面上的 <label> 都连着一个控件，没有只长得像标签、读屏却对不上输入框的文字。
  const orphanLabels = await page.evaluate(() =>
    Array.from(document.querySelectorAll("label"))
      .filter((label) => !label.control)
      .map((label) => label.textContent?.trim())
  );
  expect(orphanLabels).toEqual([]);
});

test("安全设置的每个输入框都读得出自己的标签", async ({ page }) => {
  await mockGateway(page, { signedIn: true });
  await page.goto("/account/security");
  await waitForHydration(page);

  for (const label of ["当前密码", "新密码", "确认新密码", "绑定邮箱", "邮箱验证码"]) {
    await expect(page.getByLabel(label, { exact: true }), label).toBeVisible();
  }
  await page.getByText("确认新密码", { exact: true }).click();
  await expect(page.getByLabel("确认新密码", { exact: true })).toBeFocused();
});

test("子站页头用 aria-current 标出当前标签", async ({ page }) => {
  await mockGateway(page);
  const nav = page.locator("header nav");

  await page.goto("/practice/stats");
  await waitForHydration(page);
  await expect(nav.getByRole("link", { name: /P-04/ })).toHaveAttribute("aria-current", "page");
  await expect(nav.locator("[aria-current]")).toHaveCount(1);

  await page.goto("/food");
  await waitForHydration(page);
  await expect(nav.getByRole("link", { name: /F-01/ })).toHaveAttribute("aria-current", "page");
  await expect(nav.locator("[aria-current]")).toHaveCount(1);

  // 未开放的「刷题」不可点，但读者已经在这一页时它就是当前标签。
  await page.goto("/practice/quiz");
  await waitForHydration(page);
  await expect(nav.locator("[aria-current='page']")).toHaveText(/P-02/);
  await expect(nav.locator("[aria-current]")).toHaveCount(1);
});

test("账户中心菜单用 aria-current 标出当前页", async ({ page }) => {
  await mockGateway(page, { signedIn: true });
  await page.goto("/account/security");
  await waitForHydration(page);

  const menu = page.locator("aside nav");
  await expect(menu.getByRole("link", { name: /安全设置/ })).toHaveAttribute("aria-current", "page");
  await expect(menu.locator("[aria-current]")).toHaveCount(1);
});

test("资料目录的展开按钮报告开合", async ({ page }) => {
  await mockGateway(page);
  const material = {
    id: "names-material", type: "note", subject: "高等数学", title: "极限复习笔记", author: "资料库收录",
    intro: "", toc: ["第一节", "第二节", "第三节", "第四节", "第五节", "第六节", "第七节", "第八节"],
    pages: [], price: 0, previewPages: 0, downloads: 1, downloadAvailable: true, fileSize: 1024,
  };
  await page.route(`**/api/v1/library/materials/${material.id}`, (route) =>
    route.fulfill({ json: { material, request_id: "req_names_material" } })
  );
  await page.goto(`/library/item/${material.id}`);
  await waitForHydration(page);

  const expand = page.getByRole("button", { name: /展开全部 8 节/ });
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  await expand.click();
  await expect(page.getByText("第八节")).toBeVisible();
  await expect(page.getByRole("button", { name: /收起/ })).toHaveAttribute("aria-expanded", "true");
});

test("已登录时，子站页头的账户入口读出完整昵称", async ({ page }) => {
  await mockGateway(page, { signedIn: true });

  // 手机上账户入口在页头第一行；桌面上有多个标签的子站把它放在标签行末尾。
  for (const { width, route } of [
    { width: 390, route: "/library" },
    { width: 1440, route: "/food" },
  ]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(route);
    await waitForHydration(page);
    await expect(page.locator("header").getByRole("link", { name: "小河同学的账户概览", exact: true }), `${width}px ${route}`)
      .toBeVisible();
  }
});
