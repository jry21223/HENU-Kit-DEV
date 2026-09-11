import { expect, test, type Page } from "@playwright/test";

/**
 * `go()` 的起点是现推的：`currentIndex()` 把「scrollY + 35% 视口高」当作读者所在
 * 的整屏。整屏接管只在整屏边界上停得住，可读者并不总停在边界上——视口变大（旋转
 * 屏幕、拉窗口、地址栏收放、布局回流）会让每一屏重新长高，浏览器保持 scrollY 不动，
 * 读者于是落到某个边界**上方**（不足一屏处）。
 *
 * 这种落点上探针会把读者的屏号少算一屏：向上滑被判成「已经在第一屏」，`go(-1)`
 * 算出 next === from 直接返回 false，整段手势被吃掉——屏幕一动不动，而且再滑几次
 * 也不会动，只能先向下滑一屏才解得开。
 *
 * 与 #501 修掉的两条不同：那两条是输入方向归一化（触摸端整屏反向）和手势锁（一次
 * 拖动连切多屏）。这里是起点来源本身错了，方向判断和手势锁都修好了它依然在。
 *
 * 测试只断言读者看得见的结果：一次手势必须在手势方向上切到相邻的整屏模块。
 */

async function swipeFinger(page: Page, from: number, to: number, steps = 1, stepDelayMs = 12) {
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
    if (steps > 1) await page.waitForTimeout(stepDelayMs);
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}

async function openHomepage(page: Page) {
  await page.goto("/", { waitUntil: "load" });
  await expect(page.locator(".snap-screen")).toHaveCount(7, { timeout: 30_000 });
  await page.waitForSelector("html[data-scroll-memory='ready']", { timeout: 30_000 });
}

async function waitForSnapTakeover(page: Page) {
  await page.keyboard.press("PageDown");
  await expect.poll(() => activeSection(page)).toBe(1);
  await waitForSnapSettle(page);
  await page.keyboard.press("PageUp");
  await expect.poll(() => activeSection(page)).toBe(0);
  await waitForSnapSettle(page);
}

async function waitForSnapSettle(page: Page) {
  let previous = -1;
  let stable = 0;
  await expect
    .poll(
      async () => {
        const y = Math.round(await readScrollY(page));
        stable = y === previous ? stable + 1 : 0;
        previous = y;
        return stable;
      },
      { timeout: 20_000, intervals: [400] }
    )
    .toBeGreaterThanOrEqual(2);
}

async function activeSection(page: Page) {
  return page.evaluate(() => {
    const sections = Array.from(document.querySelectorAll<HTMLElement>(".snap-screen"));
    const probe = window.scrollY + window.innerHeight * 0.35;
    let index = 0;
    sections.forEach((section, position) => {
      if (section.offsetTop <= probe) index = position;
    });
    return index;
  });
}

async function readScrollY(page: Page) {
  return page.evaluate(() => window.scrollY);
}

/** 视口里可见高度最大的那一屏——读者直觉上的「我现在在哪一屏」。 */
async function layout(page: Page) {
  return page.evaluate(() => {
    const sections = Array.from(document.querySelectorAll<HTMLElement>(".snap-screen"));
    const tops = sections.map((section) => Math.round(section.offsetTop));
    const scrollY = Math.round(window.scrollY);
    const viewportHeight = window.innerHeight;
    let dominant = 0;
    let mostVisible = -1;
    sections.forEach((section, position) => {
      const top = section.offsetTop;
      const bottom = top + section.offsetHeight;
      const visible = Math.max(
        0,
        Math.min(bottom, scrollY + viewportHeight) - Math.max(top, scrollY)
      );
      if (visible > mostVisible) {
        mostVisible = visible;
        dominant = position;
      }
    });
    return { scrollY, tops, dominant, mostVisible, viewportHeight };
  });
}

// 触摸接管需要设备报告触摸能力（Observer 的 type: "touch" 只在 _isTouch 时挂监听），
// 否则 CDP 的触摸事件会退化成原生滚动——那不是这条缺陷。
test.describe("短落点上的手势起点", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("对照：停在整屏边界上时，向上滑确实能回到上一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    await page.keyboard.press("PageDown");
    await expect.poll(() => activeSection(page)).toBe(1);
    await waitForSnapSettle(page);
    const { tops } = await layout(page);

    await swipeFinger(page, 600, 900);
    await waitForSnapSettle(page);

    expect(Math.round(await readScrollY(page))).toBe(tops[0]);
  });

  test("视口变大留下的短落点：向上滑仍要回到上一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    // 读者停在第二屏顶端。
    await page.keyboard.press("PageDown");
    await expect.poll(() => activeSection(page)).toBe(1);
    await waitForSnapSettle(page);

    // 视口变大：每一屏跟着长高，浏览器保持 scrollY 不变，读者于是落到了新的第二屏
    // 边界上方约 0.43 屏处——视口里主要可见的仍然是第二屏。
    await page.setViewportSize({ width: 1024, height: 1400 });
    await page.waitForTimeout(800);
    const landed = await layout(page);
    expect(landed.dominant).toBe(1);
    expect(landed.scrollY).toBeLessThan(landed.tops[1]);

    // 向上滑（手指下滑）：读者想回到上一屏。
    await swipeFinger(page, 600, 900);
    await waitForSnapSettle(page);

    // 实际：起点被判成第 0 屏，go(-1) 空转，scrollY 一动不动，手势被整段吃掉。
    expect(Math.round(await readScrollY(page))).toBe(landed.tops[0]);
  });

  test("边界上方 40% 视口的短落点：向上滑不许原地不动", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    const { tops, viewportHeight } = await layout(page);
    const landing = tops[1] - Math.round(viewportHeight * 0.4);
    await page.evaluate((offset) => window.scrollTo(0, offset), landing);
    await expect.poll(async () => Math.round(await readScrollY(page))).toBe(landing);

    const before = await layout(page);
    expect(before.dominant).toBe(1);

    await swipeFinger(page, 600, 900);
    await waitForSnapSettle(page);

    expect(Math.round(await readScrollY(page))).toBe(tops[0]);
  });

  test("边界上方 40% 视口的短落点：向下滑要推进到下一屏，而不是补完转场", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    const { tops, viewportHeight } = await layout(page);
    const landing = tops[1] - Math.round(viewportHeight * 0.4);
    await page.evaluate((offset) => window.scrollTo(0, offset), landing);
    await expect.poll(async () => Math.round(await readScrollY(page))).toBe(landing);

    const before = await layout(page);
    expect(before.dominant).toBe(1);

    // 手指上滑（读者想继续往下读）：从第 1 屏推进到第 2 屏。
    await swipeFinger(page, 600, 300);
    await waitForSnapSettle(page);

    // 实际：起点被判成第 0 屏，这一滑只补完那次未完成的转场，停在第 1 屏边界上。
    expect(Math.round(await readScrollY(page))).toBe(tops[2]);
  });
});
