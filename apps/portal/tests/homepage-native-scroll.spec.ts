import { expect, test, type Page } from "@playwright/test";

/**
 * 首页原生滚动契约：
 * 不再由 JS（GSAP Observer）对 wheel / touch / keyboard 做 preventDefault 整屏劫持，
 * 恢复浏览器原生跟手滚动，零吞手势、零动画锁阻塞，并保留 7 个 `.snap-screen` 区块与 ScrollMemory 水合。
 */

/** hero + 资料库 + 刷题 + 美食 + 互助 + 求职 + footer */
const SECTION_COUNT = 7;

async function openHomepage(page: Page) {
  await page.goto("/", { waitUntil: "load" });
  await expect(page.locator(".snap-screen")).toHaveCount(SECTION_COUNT, { timeout: 30_000 });
  await page.waitForSelector("html[data-scroll-memory='ready']", { timeout: 30_000 });
}

async function readScrollY(page: Page) {
  return page.evaluate(() => window.scrollY);
}

async function moduleTops(page: Page) {
  return page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>(".snap-screen")).map((section) =>
      Math.round(section.offsetTop)
    )
  );
}

async function swipeFinger(page: Page, from: number, to: number, steps = 16, stepDelayMs = 12) {
  const cdp = await page.context().newCDPSession(page);
  const x = 200;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x, y: from }],
  });
  for (let step = 1; step <= steps; step += 1) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x, y: from + ((to - from) * step) / steps }],
    });
    await page.waitForTimeout(stepDelayMs);
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}

test.describe("首页桌面端原生滚动（1440×900）", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test("连续 6 次快速向下滚零吞事件，紧接着立即向上反滚立刻回退", async ({ page }) => {
    await openHomepage(page);
    await page.mouse.move(720, 450);

    let previous = await readScrollY(page);
    expect(previous).toBe(0);

    for (let step = 1; step <= 6; step += 1) {
      await page.mouse.wheel(0, 220);
      await page.waitForTimeout(200);
      const current = await readScrollY(page);
      expect(current, `第 ${step} 次向下滚被吞`).toBeGreaterThan(previous);
      previous = current;
    }

    // 紧接着立即向上反滚（无静默等待）：必须立刻回退
    await page.mouse.wheel(0, -260);
    await expect.poll(() => readScrollY(page)).toBeLessThan(previous);
  });

  test("轻滚 deltaY=10 立即生效且不会整屏跳到下一块", async ({ page }) => {
    await openHomepage(page);
    const tops = await moduleTops(page);

    await page.mouse.move(720, 450);
    await page.mouse.wheel(0, 10);

    await expect.poll(() => readScrollY(page)).toBeGreaterThan(0);
    const y = await readScrollY(page);
    expect(y).toBeLessThan(tops[1] / 2);
  });
});

test.describe("首页触摸与偏好设置下的原生滚动", () => {
  test.describe("桌面/平板触控（1024×800）", () => {
    test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

    test("触摸拖动跟手滚动、不整屏吸附，反向拖动立即回退", async ({ page }) => {
      await openHomepage(page);
      const tops = await moduleTops(page);

      await swipeFinger(page, 640, 420, 16);
      await expect.poll(() => readScrollY(page)).toBeGreaterThan(0);
      const afterDown = await readScrollY(page);
      expect(afterDown).toBeLessThan(tops[1]);

      await swipeFinger(page, 420, 600, 16);
      await expect.poll(() => readScrollY(page)).toBeLessThan(afterDown);
    });
  });

  test.describe("768px 以下手机竖屏", () => {
    test.use({ viewport: { width: 480, height: 800 }, hasTouch: true });

    test("滚轮与触摸均为原生滚动", async ({ page }) => {
      await openHomepage(page);
      const tops = await moduleTops(page);

      await page.mouse.move(200, 300);
      await page.mouse.wheel(0, 120);
      await expect.poll(() => readScrollY(page)).toBeGreaterThan(0);
      const wheelY = await readScrollY(page);
      expect(wheelY).toBeLessThan(tops[1] / 2);

      await swipeFinger(page, 640, 400, 16);
      await expect.poll(() => readScrollY(page)).toBeGreaterThan(wheelY);
    });
  });

  test.describe("prefers-reduced-motion: reduce", () => {
    test.use({
      viewport: { width: 1024, height: 800 },
      contextOptions: { reducedMotion: "reduce" },
    });

    test("减少动态偏好下保持原生滚动", async ({ page }) => {
      await openHomepage(page);
      const tops = await moduleTops(page);

      await page.mouse.move(500, 400);
      await page.mouse.wheel(0, 120);
      await expect.poll(() => readScrollY(page)).toBeGreaterThan(0);
      const y = await readScrollY(page);
      expect(y).toBeLessThan(tops[1] / 2);

      await page.mouse.wheel(0, -80);
      await expect.poll(() => readScrollY(page)).toBeLessThan(y);
    });
  });
});
