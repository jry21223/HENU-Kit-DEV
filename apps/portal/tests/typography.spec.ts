import { expect, test } from "@playwright/test";
import {
  SUB_SITES,
  VIEWPORTS,
  gotoHomeWithContent,
  mockGatewayWithContent,
  waitForHydration,
} from "./support/readability-routes";
import { typographyViolations } from "./support/typography";

/**
 * 最小字号与中文字距（#536；DESIGN_SYSTEM.md 第 4 节“排版”）：首页、五个子站首页和登录页上，
 * 桌面 1440 与手机 390 两种宽度下，看得见的文字都不小于 12px（读屏隐藏的纯装饰拉丁标签可到 10px），
 * 含中文的文字字距都不超过 0.05em。检查逻辑见 tests/support/typography.ts。
 *
 * 页面和网关 mock 与 color-contrast.spec.ts 共用（tests/support/readability-routes.ts），
 * 列表、筛选、卡片和档位标签都渲染出来；减少动态设置下没有播到一半的动画。
 */

test.use({ contextOptions: { reducedMotion: "reduce" } });

test.beforeEach(async ({ page }) => {
  await mockGatewayWithContent(page);
});

for (const viewport of VIEWPORTS) {
  test.describe(`${viewport.label}px`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    test("首页的文字不小于 12px，中文不加宽字距", async ({ page }) => {
      await gotoHomeWithContent(page);

      // 逐屏滚到每个模块，滚动触发的内容都挂上之后再量整页。
      const screens = page.locator(".snap-screen");
      const count = await screens.count();
      expect(count, "首屏、五个模块和页脚").toBe(7);
      for (let index = 0; index < count; index += 1) {
        await screens.nth(index).evaluate(async (element) => {
          element.scrollIntoView({ block: "start" });
          await new Promise(requestAnimationFrame);
        });
      }
      expect(await typographyViolations(page), "首页：以下文字的字号或字距不达标").toEqual([]);
    });

    for (const { route, ready } of SUB_SITES) {
      test(`${route} 的文字不小于 12px，中文不加宽字距`, async ({ page }) => {
        await page.goto(route);
        await waitForHydration(page);
        await expect(ready(page)).toBeVisible();
        expect(await typographyViolations(page), `${route}：以下文字的字号或字距不达标`).toEqual([]);
      });
    }
  });
}
