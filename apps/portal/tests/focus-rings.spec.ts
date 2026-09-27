import { expect, test, type Page } from "@playwright/test";
import { tabThroughScroller } from "./support/focus-rings";
import { mockGatewayWithContent, waitForHydration } from "./support/readability-routes";

/**
 * 焦点框不被滚动容器裁掉（DESIGN_SYSTEM §13）。滚动容器会裁掉画在控件外面的东西，
 * 控件又常常贴着容器的边：这些控件把焦点框画在自己里面，键盘聚焦时整个滑进可见范围。
 * 子站标签行由 sub-site-nav.spec.ts 检查，这里查首页手机菜单、美食五档榜单导览和账户中心菜单。
 */

test.use({ contextOptions: { reducedMotion: "reduce" } });

const SIGNED_IN = {
  user_id: "11111111-1111-4111-8111-111111111111",
  display_name: "小河同学",
  expires_at: "2030-01-01T00:00:00Z",
};

async function signIn(page: Page) {
  await page.route("**/api/v1/session", (route) => route.fulfill({ json: SIGNED_IN }));
}

// 手机上面板放得下时不滚，矮屏（横屏手机）上面板自己滚：两种都要看得见整圈焦点框。
for (const viewport of [
  { label: "390 × 844", width: 390, height: 844 },
  { label: "667 × 375", width: 667, height: 375 },
]) {
  test(`${viewport.label} 首页手机菜单：每一行的焦点框都露在面板里`, async ({ page }) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await mockGatewayWithContent(page);
    await signIn(page);
    await page.goto("/");
    await waitForHydration(page);

    const toggle = page.locator("header").getByRole("button", { name: "打开菜单" });
    const panel = page.locator(`[id="${await toggle.getAttribute("aria-controls")}"]`);
    await toggle.click();
    await expect(panel).toBeVisible();
    await expect(panel.getByRole("link", { name: "小河同学的账户概览" })).toBeAttached();

    const { stops, clipped } = await tabThroughScroller(page, panel, { maxPresses: 2 });
    // 五个模块加账户行。
    expect(stops).toBe(6);
    expect(clipped).toEqual([]);
  });
}

for (const viewport of [
  { width: 390, height: 844 },
  { width: 1440, height: 900 },
]) {
  test(`${viewport.width}px 美食五档榜单导览：每一档的焦点框都露在导览里`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockGatewayWithContent(page);
    await page.goto("/food");
    await waitForHydration(page);

    const tiers = page.getByRole("navigation", { name: "五档榜单导览" });
    await expect(tiers.getByRole("link")).toHaveCount(5);
    const { stops, clipped } = await tabThroughScroller(page, tiers, { maxPresses: 40 });
    expect(stops).toBe(5);
    expect(clipped).toEqual([]);
  });

  test(`${viewport.width}px 账户中心菜单：每一项的焦点框都露在菜单里`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await mockGatewayWithContent(page);
    await signIn(page);
    await page.goto("/account/security");
    await waitForHydration(page);

    const menu = page.locator("aside nav");
    await expect(menu.getByRole("link", { name: /安全设置/ })).toHaveAttribute("aria-current", "page");
    const { stops, clipped } = await tabThroughScroller(page, menu, { maxPresses: 20 });
    // 八个页面加「退出登录」。
    expect(stops).toBe(9);
    expect(clipped).toEqual([]);
  });
}
