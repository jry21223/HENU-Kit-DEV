import { expect, test, type Page } from "@playwright/test";

/**
 * `go()` 的起点是现推的。这组用例钉住 #508 诊断出来的缺陷：**修复前**
 * `currentIndex()` 把「scrollY + 35% 视口高」当作读者所在的整屏，而整屏接管只在整屏
 * 边界上停得住——读者并不总停在边界上：视口变大（旋转屏幕、拉窗口、地址栏收放、
 * 布局回流）会让每一屏重新长高，浏览器保持 scrollY 不动，读者于是落到某个边界
 * **上方**（不足一屏处）。
 *
 * 探针与视觉不一致的是其中 35%~50% 视口高那一段：这时读者主要看着下面那一屏，35%
 * 的探针却仍把他算在上一屏（δ ≤ 35% 时两者本来一致，δ > 50% 时他确实该算上一屏）。
 * 在这个带上向上滑会被判成「已经在第一屏」，`go(-1)` 算出 next === from 直接返回
 * false，整段手势被吃掉——屏幕一动不动，而且再滑几次也不会动，只能先向下滑一屏才
 * 解得开。
 *
 * 修复后的起点是「视口里可见高度最大的那一屏」，各占一半时取靠下那屏（见
 * `snap-scroll.tsx` 的 `currentIndex`）：短落点上向上滑回到上一屏，向下滑推进到下一
 * 屏，而不是只补完那次未完成的转场。触摸与滚轮共用这一个起点，两条路径分别有用例。
 *
 * 与 #501 修掉的两条不同：那两条是输入方向归一化（触摸端整屏反向）和手势锁（一次
 * 拖动连切多屏）。这里是起点来源本身错了，方向判断和手势锁都修好了它依然在。
 *
 * 测试只断言读者看得见的结果：一次手势必须在手势方向上切到相邻的整屏模块。
 */

/** hero + 资料库 + 刷题 + 美食 + 互助 + 求职 + footer */
const SECTION_COUNT = 7;

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
  await expect(page.locator(".snap-screen")).toHaveCount(SECTION_COUNT, { timeout: 30_000 });
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

/**
 * 落定在整屏边界上时读者在第几屏——沿用**修复前**的 35% 探针，只用于接管自检与
 * PageDown/PageUp 之后说一句「已经翻到第 N 屏了」。它和 `layout()` 的可见高度判据在
 * 短落点上并不一致：读者实际看着哪一屏以 `layout().dominant` 为准，短落点用例都用它。
 */
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

/**
 * 视口里可见高度最大的那一屏——读者直觉上的「我现在在哪一屏」。判据与生产里的
 * `currentIndex()` 相同，平局同样取靠下那屏，免得断言量的是另一条规则。
 */
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
      if (visible >= mostVisible) {
        mostVisible = visible;
        dominant = position;
      }
    });
    return { scrollY, tops, dominant, viewportHeight };
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
    // 边界上方约 0.43 屏处——视口里主要可见的仍然是第二屏。等这次回流真落地（视口与
    // 第二屏边界都长到新高度），不用固定睡眠。
    await page.setViewportSize({ width: 1024, height: 1400 });
    await expect
      .poll(async () => {
        const current = await layout(page);
        return current.viewportHeight === 1400 && current.tops[1] > current.scrollY;
      })
      .toBe(true);

    const landed = await layout(page);
    expect(landed.dominant).toBe(1);
    expect(landed.scrollY).toBeLessThan(landed.tops[1]);
    // 这条用例只有在「探针会少算一屏」的那条带子里才有意义（δ > 35% 视口高）：否则
    // 它会在正确代码和 35% 探针上都变绿，悄悄退化成一个不设防的用例。
    expect(landed.tops[1] - landed.scrollY).toBeGreaterThan(landed.viewportHeight * 0.35);

    // 向上滑（手指下滑）：读者想回到上一屏。修复前起点被判成第 0 屏，go(-1) 空转，
    // scrollY 一动不动（800），手势被整段吃掉。
    await swipeFinger(page, 600, 900);
    await waitForSnapSettle(page);

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

    // 手指上滑（读者想继续往下读）：从第 1 屏推进到第 2 屏。修复前起点被判成第 0
    // 屏，这一滑只补完那次未完成的转场，停在第 1 屏边界上（800）。
    await swipeFinger(page, 600, 300);
    await waitForSnapSettle(page);

    expect(Math.round(await readScrollY(page))).toBe(tops[2]);
  });

  test("边界恰好各占一半时：起点归靠下那屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    // 视口正好被第 0/1 屏各占一半：起点必须归靠下那屏（第 1 屏），否则向上滑会被判成
    // 「已经在第 0 屏」而原地不动——这正是 35% 探针在那条带子里的错法。
    const { tops, viewportHeight } = await layout(page);
    const landing = tops[1] - Math.round(viewportHeight / 2);
    await page.evaluate((offset) => window.scrollTo(0, offset), landing);
    await expect.poll(async () => Math.round(await readScrollY(page))).toBe(landing);

    // 先确认这真是一个平局，而不是碰巧滑进了别处：上下两屏的可见高度相等。
    const halves = await page.evaluate(() => {
      const sections = Array.from(document.querySelectorAll<HTMLElement>(".snap-screen"));
      const visible = (position: number) => {
        const top = sections[position].offsetTop;
        const bottom = top + sections[position].offsetHeight;
        return (
          Math.min(bottom, window.scrollY + window.innerHeight) - Math.max(top, window.scrollY)
        );
      };
      return { upper: visible(0), lower: visible(1) };
    });
    expect(halves.upper).toBe(halves.lower);
    expect(halves.lower).toBeGreaterThan(0);
    expect((await layout(page)).dominant).toBe(1);

    await swipeFinger(page, 600, 900);
    await waitForSnapSettle(page);

    expect(Math.round(await readScrollY(page))).toBe(tops[0]);
  });
});

// 验收要求触摸视口与滚轮路径行为一致：起点判定与输入类型无关，两条路径共用同一个
// `go()`，所以滚轮读者从同一个短落点出发也要分别上/下走一屏。
test.describe("短落点上的滚轮起点", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("边界上方 40% 视口的短落点：滚轮上滚回到上一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    const { tops, viewportHeight } = await layout(page);
    const landing = tops[1] - Math.round(viewportHeight * 0.4);
    await page.evaluate((offset) => window.scrollTo(0, offset), landing);
    await expect.poll(async () => Math.round(await readScrollY(page))).toBe(landing);

    const before = await layout(page);
    expect(before.dominant).toBe(1);

    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, -400);
    await waitForSnapSettle(page);

    expect(Math.round(await readScrollY(page))).toBe(tops[0]);
  });

  test("边界上方 40% 视口的短落点：滚轮下滚推进到下一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);

    const { tops, viewportHeight } = await layout(page);
    const landing = tops[1] - Math.round(viewportHeight * 0.4);
    await page.evaluate((offset) => window.scrollTo(0, offset), landing);
    await expect.poll(async () => Math.round(await readScrollY(page))).toBe(landing);

    const before = await layout(page);
    expect(before.dominant).toBe(1);

    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, 400);
    await waitForSnapSettle(page);

    expect(Math.round(await readScrollY(page))).toBe(tops[2]);
  });
});
