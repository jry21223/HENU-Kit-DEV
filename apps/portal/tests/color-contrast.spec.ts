import { expect, test, type Locator, type Page } from "@playwright/test";
import { contrastViolations, revealTextBackgrounds } from "./support/color-contrast";
import {
  SUB_SITES,
  VIEWPORTS,
  gotoHomeWithContent,
  mockGatewayWithContent,
  waitForHydration,
} from "./support/readability-routes";

/**
 * 文字对比度达到 WCAG AA（#536；DESIGN_SYSTEM.md 第 3 节“文字配色”、第 13 节）：axe 的
 * color-contrast 规则在首页、五个子站首页和登录页上为 0，桌面 1440 与手机 390 两种宽度都算。
 * 检查范围和扫描前的准备见 tests/support/color-contrast.ts。
 *
 * - 页面和网关 mock 与 typography.spec.ts 共用（tests/support/readability-routes.ts），列表、筛选、
 *   卡片和档位标签都渲染出来；减少动态设置下没有播到一半的动画。
 * - 首页逐屏滚到每个模块后再扫整页：固定页头是半透明纸白，压在墨色的刷题模块上时底色会变深，
 *   每个滚动位置都要算。
 */

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGatewayWithContent(page);
});

for (const viewport of VIEWPORTS) {
  test.describe(`${viewport.label}px`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    test("首页每一屏的文字对比度都达到 AA", async ({ page }) => {
      await gotoHomeWithContent(page);
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
