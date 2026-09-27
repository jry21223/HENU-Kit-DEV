import { expect, test, type Locator, type Page } from "@playwright/test";

/**
 * 首页手机菜单（#541）：按钮名随开合在「打开菜单 / 关闭菜单」之间切换，并用
 * aria-controls 指向菜单面板；Esc 与点遮罩都能关，关后焦点回到菜单按钮。打开期间
 * 页面不滚动（矮屏上面板自己滚）、Tab 只在菜单里循环、读屏软件也进不到遮罩下面的页面；
 * 账户行整行可点，已登录时昵称完整显示。
 */

/** 未登录，其余接口一律不可用：菜单不依赖接口数据。 */
async function mockGateway(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" },
        request_id: "req_menu_unavailable",
      }),
    })
  );
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({ status: 401, contentType: "application/json", body: "{}" })
  );
}

/** 打开首页并等客户端外壳水合完成：水合前点按钮不会有反应。 */
async function openHomepage(page: Page) {
  await page.goto("/");
  await expect(page.locator("html[data-scroll-memory='ready']")).toHaveCount(1, { timeout: 30_000 });
}

const menuToggle = (page: Page) => page.locator("header").getByRole("button", { name: /^(打开|关闭)菜单$/ });

/** 菜单面板按按钮的 aria-controls 找，顺带确认两者连得上。 */
async function menuPanel(page: Page): Promise<Locator> {
  const id = await menuToggle(page).getAttribute("aria-controls");
  expect(id, "菜单按钮要用 aria-controls 指向面板").toBeTruthy();
  return page.locator(`[id="${id}"]`);
}

declare global {
  interface Window {
    __wheelSettled?: boolean;
  }
}

/**
 * 视口实际用的纵向 overflow：<html> 不是 visible 时用它自己的，否则沿用 <body> 的
 * （CSS 溢出传播）。只看 body 会漏掉「<html> 另设了 overflow，body 的 hidden 管不到视口」。
 */
const viewportOverflowY = (page: Page) =>
  page.evaluate(() => {
    const root = getComputedStyle(document.documentElement).overflowY;
    return root === "visible" ? getComputedStyle(document.body).overflowY : root;
  });

/**
 * 滚一下滚轮，等页面处理完这一下：滚轮事件到达页面后再过两帧，页面若会滚，scrollY
 * 这时已经变了。等的是这一下滚轮本身，不按固定时长干等。
 */
async function wheelAndSettle(page: Page, deltaY: number) {
  await page.evaluate(() => {
    window.__wheelSettled = false;
    const settle = () =>
      requestAnimationFrame(() =>
        requestAnimationFrame(() => {
          window.__wheelSettled = true;
        })
      );
    window.addEventListener("wheel", settle, { once: true, capture: true, passive: true });
  });
  await page.mouse.wheel(0, deltaY);
  await expect
    .poll(() => page.evaluate(() => window.__wheelSettled), { message: "页面要收到这一下滚轮" })
    .toBe(true);
}

test.use({ viewport: { width: 390, height: 800 }, contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGateway(page);
});

test("按钮名随开合切换，并指向菜单面板", async ({ page }) => {
  await openHomepage(page);
  const toggle = menuToggle(page);

  await expect(toggle).toHaveAccessibleName("打开菜单");
  await expect(toggle).toHaveAttribute("aria-expanded", "false");

  await toggle.click();
  await expect(toggle).toHaveAccessibleName("关闭菜单");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("link", { name: /资料库/ })).toBeVisible();

  await toggle.click();
  await expect(toggle).toHaveAccessibleName("打开菜单");
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(panel).toBeHidden();
});

test("Esc 关闭菜单，焦点回到菜单按钮", async ({ page }) => {
  await openHomepage(page);
  const toggle = menuToggle(page);
  await toggle.click();
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();

  // 焦点先进到面板里，才看得出 Esc 之后是被送回按钮的，而不是本来就停在那。
  await page.keyboard.press("Tab");
  await expect(panel.getByRole("link").first()).toBeFocused();

  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  await expect(toggle).toHaveAccessibleName("打开菜单");
  await expect(toggle).toBeFocused();
});

test("点面板下方的遮罩关闭菜单，焦点回到菜单按钮", async ({ page }) => {
  await openHomepage(page);
  const toggle = menuToggle(page);
  await toggle.click();
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();
  await page.keyboard.press("Tab");

  const box = await panel.boundingBox();
  if (!box) throw new Error("菜单面板没有渲染出来");
  const viewport = page.viewportSize()!;
  // 面板没占满视口，下面露出的那一截就该是遮罩。
  expect(box.y + box.height).toBeLessThan(viewport.height - 40);
  await page.mouse.click(viewport.width / 2, viewport.height - 20);

  await expect(panel).toBeHidden();
  await expect(toggle).toBeFocused();
  // 这次点击只用来关菜单，不能穿透到遮罩下面的页面去。
  await expect(page).toHaveURL(/\/$/);
});

test("菜单打开期间页面不滚动，关上后恢复", async ({ page }) => {
  await openHomepage(page);
  const toggle = menuToggle(page);
  const scrollY = () => page.evaluate(() => window.scrollY);

  await toggle.click();
  await expect(await menuPanel(page)).toBeVisible();
  await expect.poll(() => viewportOverflowY(page)).toBe("hidden");
  // 在遮罩上滚滚轮，等页面处理完这一下：页面不能跟着走。
  await page.mouse.move(195, 760);
  await wheelAndSettle(page, 600);
  expect(await scrollY()).toBe(0);

  await page.keyboard.press("Escape");
  await expect.poll(() => viewportOverflowY(page)).not.toBe("hidden");
  // 手机上首页是原生滚动：关上菜单后，同样一下滚轮、同样的等法，页面要已经滚动。
  // 这也说明上面的等法够久：锁要是失效，那一下同样来得及滚。
  await wheelAndSettle(page, 600);
  expect(await scrollY()).toBeGreaterThan(0);
});

test("点菜单里的链接离开首页，滚动锁不带到下一页", async ({ page }) => {
  await openHomepage(page);
  await menuToggle(page).click();
  const panel = await menuPanel(page);
  await panel.getByRole("link", { name: /资料库/ }).click();

  await expect(page).toHaveURL(/\/library$/);
  await expect.poll(() => page.evaluate(() => getComputedStyle(document.body).overflowY)).not.toBe("hidden");
});

test("菜单开着时窗口拉宽到桌面布局：菜单收起，页面恢复滚动", async ({ page }) => {
  await openHomepage(page);
  const toggle = menuToggle(page);
  await toggle.click();
  await expect(await menuPanel(page)).toBeVisible();

  // 桌面布局没有菜单按钮，锁要是留着，读者就再也关不掉它。
  await page.setViewportSize({ width: 1024, height: 800 });
  await expect.poll(() => page.evaluate(() => getComputedStyle(document.body).overflowY)).not.toBe("hidden");
  await page.mouse.move(512, 400);
  await page.mouse.wheel(0, 600);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(0);

  await page.setViewportSize({ width: 390, height: 800 });
  await expect(toggle).toHaveAccessibleName("打开菜单");
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
});

test("菜单打开期间 Tab 只在菜单按钮和面板里循环", async ({ page }) => {
  await openHomepage(page);
  const toggle = menuToggle(page);
  await toggle.click();
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();
  const stops = await panel.locator("a[href], button").count();

  const focusInMenu = () =>
    page.evaluate(() => {
      const active = document.activeElement;
      const toggleButton = document.querySelector("header button[aria-controls]");
      const menu = toggleButton && document.getElementById(toggleButton.getAttribute("aria-controls") ?? "");
      return active === toggleButton || !!menu?.contains(active);
    });

  // 按钮 → 面板里每一项 → 再回到按钮，中间一步都不能跑到菜单外面去。
  for (let step = 0; step < stops; step += 1) {
    await page.keyboard.press("Tab");
    expect(await focusInMenu(), `第 ${step + 1} 次 Tab`).toBe(true);
    await expect(toggle).not.toBeFocused();
  }
  await page.keyboard.press("Tab");
  await expect(toggle).toBeFocused();

  // 反向同理：从按钮 Shift+Tab 落到面板最后一项。
  await page.keyboard.press("Shift+Tab");
  await expect(panel.locator("a[href], button").last()).toBeFocused();
});

test("菜单打开期间，读屏的滑动浏览也进不到遮罩下面的页面", async ({ page }) => {
  await openHomepage(page);
  // 读屏软件（VoiceOver / TalkBack）左右滑动走的是浏览器交出的无障碍树，不是 Tab 顺序：
  // 直接读 Chromium 的无障碍树，看遮罩下面的正文和页脚还在不在里面。
  const cdp = await page.context().newCDPSession(page);
  const exposed = async () => {
    const { nodes } = await cdp.send("Accessibility.getFullAXTree");
    const reachable = nodes.filter((node) => !node.ignored);
    const has = (role: string, name?: string) =>
      reachable.some((node) => node.role?.value === role && (name === undefined || node.name?.value === name));
    return {
      main: has("main"),
      footer: has("contentinfo"),
      heroEntry: has("link", "找资料"),
      menuAccountRow: has("link", "登录 / 注册"),
    };
  };
  const closedTree = { main: true, footer: true, heroEntry: true, menuAccountRow: false };

  expect(await exposed()).toEqual(closedTree);

  await menuToggle(page).click();
  await expect(await menuPanel(page)).toBeVisible();
  // 正文、页脚连同首屏入口都退出无障碍树；菜单本身照常可读。
  await expect
    .poll(exposed)
    .toEqual({ main: false, footer: false, heroEntry: false, menuAccountRow: true });

  await page.keyboard.press("Escape");
  await expect.poll(exposed).toEqual(closedTree);
  await cdp.detach();
});

test("矮屏上页面锁住了，面板自己滚动，最后一项也够得着", async ({ page }) => {
  // 横屏手机：宽度不到 md，高度放不下整张面板。
  await page.setViewportSize({ width: 667, height: 375 });
  await openHomepage(page);
  await menuToggle(page).click();
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();

  const lastItem = panel.getByRole("link").last();
  await lastItem.scrollIntoViewIfNeeded();
  await expect(lastItem).toBeInViewport({ ratio: 1 });
  // 滚的是面板，页面本身仍停在原处。
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
});

/** 账户行和上面的模块行一样整行可点：链接的点按区域横跨整个面板。 */
async function expectFullRow(panel: Locator, link: Locator) {
  const [panelBox, linkBox] = await Promise.all([panel.boundingBox(), link.boundingBox()]);
  if (!panelBox || !linkBox) throw new Error("菜单面板或账户行没有渲染出来");
  expect(linkBox.x).toBeLessThanOrEqual(panelBox.x + 1);
  expect(linkBox.width).toBeGreaterThanOrEqual(panelBox.width - 1);
  expect(linkBox.height).toBeGreaterThanOrEqual(44);
}

test("未登录时账户行是整行可点的「登录 / 注册」，不再标 ACC", async ({ page }) => {
  await openHomepage(page);
  await menuToggle(page).click();
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();

  const account = panel.getByRole("link", { name: "登录 / 注册", exact: true });
  await expect(account).toHaveAttribute("href", "/account/login");
  await expectFullRow(panel, account);
  await expect(panel).not.toContainText("ACC");
});

test("已登录时账户行整行进入账户页，显示完整昵称", async ({ page }) => {
  const nickname = "河大计算机学院小河同学";
  await page.route("**/api/v1/session", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        user_id: "11111111-1111-4111-8111-111111111111",
        display_name: nickname,
        expires_at: "2030-01-01T00:00:00Z",
      }),
    })
  );
  await openHomepage(page);
  await menuToggle(page).click();
  const panel = await menuPanel(page);
  await expect(panel).toBeVisible();

  const account = panel.getByRole("link", { name: new RegExp(nickname) });
  await expect(account).toHaveAttribute("href", "/account");
  await expectFullRow(panel, account);
  await expect(panel).not.toContainText("ACC");

  // 整行有的是地方：昵称要完整显示，不能照桌面页头那样在 80px 处截成省略号。
  const name = account.getByText(nickname, { exact: true });
  await expect(name).toBeVisible();
  expect(await name.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
});

test.describe("页头带着滚动位移动画时", () => {
  test.use({ contextOptions: { reducedMotion: "no-preference" } });

  test("滚动过再打开菜单，遮罩仍铺满面板下方", async ({ page }) => {
    await openHomepage(page);
    // 下滚再回滚一点：页头被滚动动画收起又放出，身上留着 transform。
    await page.mouse.move(195, 400);
    await page.mouse.wheel(0, 600);
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(300);
    await page.mouse.wheel(0, -200);
    const toggle = menuToggle(page);
    await expect(toggle).toBeInViewport({ ratio: 1 });
    await expect.poll(() => page.locator("header").evaluate((el) => getComputedStyle(el).transform)).not.toBe("none");

    await toggle.click();
    const panel = await menuPanel(page);
    await expect(panel).toBeVisible();
    const viewport = page.viewportSize()!;
    await page.mouse.click(viewport.width / 2, viewport.height - 20);
    await expect(panel).toBeHidden();
    await expect(toggle).toBeFocused();
  });
});
