import { expect, test } from "@playwright/test";
import { waitForHydration } from "./support/readability-routes";
import { mockGuestGateway, mockSignedInGateway } from "./support/gateway";

/**
 * 读屏软件读得出名字和状态：
 * - 输入框由看得见的标签命名，placeholder 不代替标签，搜索框的 placeholder 只举例（DESIGN_SYSTEM §11）；
 * - 子站页头的标签行和账户中心的菜单用 aria-current 标出当前页；
 * - 资料目录的展开按钮用 aria-expanded 报告开合；
 * - 已登录时页头的账户入口读出完整昵称，而不只是头像块上的一个字。
 * 排行榜周期切换只在 V2 读取开启时出现，它的组名由 practice-leaderboard-live.spec.ts 检查。
 */

test.use({ contextOptions: { reducedMotion: "reduce" } });

test("账户中心在读会话和会话读取失败时，正文也在唯一的 main 地标里", async ({ page }) => {
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await mockGuestGateway(page);
  await page.route("**/api/v1/session", async (route) => {
    await gate;
    await route.fulfill({ status: 503, json: { error: "portal_session_unavailable", request_id: "req_names_session_down" } });
  });
  await page.goto("/account", { waitUntil: "domcontentloaded" });

  const main = page.getByRole("main");
  await expect(main).toHaveCount(1);
  await expect(main).toHaveAttribute("data-account-session-state", "loading");

  release();
  await expect(main.locator('[data-account-session-state="error"]')).toBeVisible();
  await expect(main).toHaveCount(1);
});

test("资料库、互助和题库的搜索框由看得见的标签命名", async ({ page }) => {
  await mockGuestGateway(page);

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

test("安全设置的每个输入框都读得出自己的标签", async ({ page }) => {
  await mockSignedInGateway(page);
  await page.goto("/account/security");
  await waitForHydration(page);

  for (const label of ["当前密码", "新密码", "确认新密码", "绑定邮箱", "邮箱验证码"]) {
    await expect(page.getByLabel(label, { exact: true }), label).toBeVisible();
  }
  await page.getByText("确认新密码", { exact: true }).click();
  await expect(page.getByLabel("确认新密码", { exact: true })).toBeFocused();
});

test("子站页头用 aria-current 标出当前标签", async ({ page }) => {
  await mockGuestGateway(page);
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
  await mockSignedInGateway(page);
  await page.goto("/account/security");
  await waitForHydration(page);

  const menu = page.locator("aside nav");
  await expect(menu.getByRole("link", { name: /安全设置/ })).toHaveAttribute("aria-current", "page");
  await expect(menu.locator("[aria-current]")).toHaveCount(1);
});

test("资料目录的展开按钮报告开合", async ({ page }) => {
  await mockGuestGateway(page);
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
  await mockSignedInGateway(page);

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

test("登录 / 注册是一组标签页，方向键在两个标签间切换，提交按钮不夹空格（#557）", async ({ page }) => {
  await mockGuestGateway(page);
  await page.goto("/account/login");
  await waitForHydration(page);

  const tabs = page.getByRole("tablist", { name: "登录或注册" });
  const signIn = tabs.getByRole("tab", { name: "登录" });
  const register = tabs.getByRole("tab", { name: "注册" });
  await expect(signIn).toHaveAttribute("aria-selected", "true");
  await expect(register).toHaveAttribute("aria-selected", "false");
  // 只有选中的标签在 Tab 键顺序里。
  await expect(signIn).toHaveAttribute("tabindex", "0");
  await expect(register).toHaveAttribute("tabindex", "-1");

  const panel = page.getByRole("tabpanel", { name: "登录" });
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("button", { name: "登录", exact: true })).toHaveAttribute("type", "submit");

  await signIn.focus();
  await page.keyboard.press("ArrowRight");
  await expect(register).toBeFocused();
  await expect(register).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("heading", { level: 1, name: "注册" })).toBeVisible();
  await expect(page.getByRole("tabpanel", { name: "注册" }).getByRole("button", { name: "注册", exact: true })).toBeVisible();

  await page.keyboard.press("Home");
  await expect(signIn).toBeFocused();
  await expect(signIn).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("ArrowLeft");
  await expect(register).toBeFocused();
  await page.keyboard.press("End");
  await expect(register).toBeFocused();

  // 落回已选中的标签（End、再点一下）不算切换：刚出的字段错误都还在。
  await page.getByRole("tabpanel", { name: "注册" }).getByRole("button", { name: "注册", exact: true }).click();
  // Next 自带的路由播报也是 role="alert"，只看正文里的提示。
  const alerts = page.locator("main").getByRole("alert");
  await expect(alerts.first()).toBeVisible();
  const shown = await alerts.allTextContents();
  await register.focus();
  await page.keyboard.press("End");
  await register.click();
  await expect(register).toHaveAttribute("aria-selected", "true");
  await expect(alerts).toHaveText(shown);

  // 登录方式是一对开关按钮，读得出哪一个按下了。
  await signIn.click();
  const codeMode = page.getByRole("button", { name: "验证码登录" });
  await expect(codeMode).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "密码登录" })).toHaveAttribute("aria-pressed", "false");

  // 再点已按下的那一个也不算切换：刚出的字段错误都还在。
  await page.getByRole("tabpanel", { name: "登录" }).getByRole("button", { name: "登录", exact: true }).click();
  await expect(alerts.first()).toBeVisible();
  const loginErrors = await alerts.allTextContents();
  await codeMode.click();
  await expect(codeMode).toHaveAttribute("aria-pressed", "true");
  await expect(alerts).toHaveText(loginErrors);
});
