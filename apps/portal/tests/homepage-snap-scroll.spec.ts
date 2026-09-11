import { expect, test, type Page } from "@playwright/test";

/**
 * 首页在 md 断点以上把滚轮/触摸交给 GSAP Observer 做整屏切换。Observer 的 deltaY
 * 是「输入自身的位移」：滚轮 deltaY 就是页面滚动量（正数 = 向下），触摸/指针拖动
 * 给的却是手指位移（负数 = 手指上滑 = 页面向下）。
 *
 * 两个回调共用一套正负判断时，触摸端会整体反向：手指上滑退回上一屏，手指下滑反而
 * 进下一屏；鼠标滚轮和键盘仍然是正常的。这个测试同时钉住两种输入的方向。
 *
 * 另外三组：首尾屏的回读/下读不再越界，以及 768px 以下和 reduce 偏好下不接管。
 */

/** hero + 资料库 + 刷题 + 美食 + 互助 + 求职 + footer */
const SECTION_COUNT = 7;

/**
 * 一次滑动 = 一次手势 = 一屏，所以默认只发一段位移：方向用例只关心方向，不必让
 * 投递时序参与。需要真实连续拖动的用例显式传 steps（原生滚动、慢速拖动回归）。
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

/**
 * 打开首页并等到 7 个整屏模块就位、客户端外壳水合完成。dev 下首次访问要现编译，
 * 生产是预构建，所以给冷编译留出时间，别让它冒充失败。
 *
 * 水合标记只说明外壳挂上了：它不证明整屏接管已经生效（ScrollMemory 与 SnapScroll
 * 是两个 effect，先后提交）。要断言方向或边界的用例接着调 waitForSnapTakeover。
 */
async function openHomepage(page: Page) {
  await page.goto("/", { waitUntil: "load" });
  await expect(page.locator(".snap-screen")).toHaveCount(SECTION_COUNT, { timeout: 30_000 });
  await page.waitForSelector("html[data-scroll-memory='ready']", { timeout: 30_000 });
}

/**
 * 等到整屏接管确实生效：键盘路径与触摸路径同属 SnapScroll 的接管，按一次 PageDown
 * 应当整屏跳一块；没接管时它只会原生滚动若干像素。跑一个来回把状态留在第一屏。
 *
 * 没有这一步，触摸事件可能在接管装上之前发出，断言就会在正确代码上变红。
 */
async function waitForSnapTakeover(page: Page) {
  await page.keyboard.press("PageDown");
  await expect.poll(() => activeSection(page)).toBe(1);
  // 索引在动画中途就会翻过去，而防连滚锁要等动画走完才释放：先落定再按回去。
  await waitForSnapSettle(page);
  await page.keyboard.press("PageUp");
  await expect.poll(() => activeSection(page)).toBe(0);
  await waitForSnapSettle(page);
}

/**
 * 等滚动落定，替代按动画时长的固定睡眠。
 *
 * 要连续 3 次采样相同：缓动尾段的位移会小到两次取整后一样，而这时防连滚锁还没释放，
 * 紧接着的手势会被吞掉。
 */
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

/** 各模块顶端：接管时每次输入都会正好落在其中一个上，不接管时不会。 */
async function moduleTops(page: Page) {
  return page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>(".snap-screen")).map(
      (section) => section.offsetTop
    )
  );
}

test.describe("首页整屏切换方向", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("手指上滑进下一屏，手指下滑回上一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 手指上滑 = 读者想往下读
    await swipeFinger(page, 640, 240);
    await expect.poll(() => activeSection(page)).toBe(1);

    // 索引在动画中途就会翻过去，所以下一次手势要等落定（防连滚锁释放）再发，
    // 否则它会被吞掉——负载越高这个窗口越明显。
    await waitForSnapSettle(page);
    await swipeFinger(page, 240, 640);
    await expect.poll(() => activeSection(page)).toBe(0);
  });

  test("一次慢速拖动只切一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 位移一直持续到整屏动画（~1.1s）结束之后：同一次手势不该再起跳一次。
    // 滚轮读者连续滚动仍然一屏一屏走，所以这里只钉触摸手势。
    await swipeFinger(page, 700, 200, 20, 120);
    await expect.poll(() => activeSection(page)).toBe(1);
  });

  test("同一页面上鼠标滚轮保持原有方向", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, 400);
    await expect.poll(() => activeSection(page)).toBe(1);

    // 同上：等这一屏的动画走完再滚回去。
    await waitForSnapSettle(page);
    await page.mouse.wheel(0, -400);
    await expect.poll(() => activeSection(page)).toBe(0);
  });
});

test.describe("首页整屏切换边界", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("第一屏再回读停在第一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 滚轮上滚 = 键鼠读者回读上一屏；第一屏已经没有上一屏
    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, -400);
    await waitForSnapSettle(page);
    expect(await activeSection(page)).toBe(0);
    expect(await readScrollY(page)).toBe(0);

    // 手指下滑 = 触摸读者回读上一屏
    await swipeFinger(page, 240, 640);
    await waitForSnapSettle(page);
    expect(await activeSection(page)).toBe(0);
    expect(await readScrollY(page)).toBe(0);
  });

  test("读到最后一块后再下读停在原地", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 一路下读到位置不再变化（每屏 ~1.1s，所以给足轮次）
    await page.mouse.move(500, 400);
    let settled = 0;
    for (let step = 0; step < SECTION_COUNT + 2; step += 1) {
      const before = await readScrollY(page);
      await page.mouse.wheel(0, 400);
      await waitForSnapSettle(page);
      settled = await readScrollY(page);
      if (settled === before) break;
    }
    expect(settled).toBeGreaterThan(0);
    const lastIndex = await activeSection(page);

    // 滚轮继续下滚：停在原地，不越界
    await page.mouse.wheel(0, 400);
    await waitForSnapSettle(page);
    expect(await readScrollY(page)).toBe(settled);

    // 手指上滑（触摸读者的下读手势）：同样停在原地
    await swipeFinger(page, 640, 240);
    await waitForSnapSettle(page);
    expect(await readScrollY(page)).toBe(settled);

    expect(await activeSection(page)).toBe(lastIndex);
    expect(lastIndex).toBe(SECTION_COUNT - 1);
  });
});

test.describe("未达到接管条件时退回普通滚动", () => {
  test.describe("768px 以下", () => {
    test.use({ viewport: { width: 480, height: 800 }, hasTouch: true });

    test("手机竖屏不被接管", async ({ page }) => {
      await openHomepage(page);
      await expect.poll(() => activeSection(page)).toBe(0);

      const tops = await moduleTops(page);
      await page.mouse.move(200, 300);
      await page.mouse.wheel(0, 120);
      await waitForSnapSettle(page);

      // 接管时滚轮会被 preventDefault 并整屏跳到下一块；不接管时只滚 deltaY
      const wheelY = await readScrollY(page);
      expect(wheelY).toBeGreaterThan(0);
      expect(wheelY).toBeLessThan(tops[1] / 2);

      // 触摸同样退回普通滚动：落点不会正好压在模块顶端（这里要真实的连续拖动，
      // 原生滚动才走得动，所以显式多段）
      await swipeFinger(page, 640, 400, 16);
      await waitForSnapSettle(page);
      const touchY = await readScrollY(page);
      expect(touchY).toBeGreaterThan(wheelY);
      expect(tops).not.toContain(touchY);
    });
  });

  test.describe("prefers-reduced-motion: reduce", () => {
    test.use({
      viewport: { width: 1024, height: 800 },
      contextOptions: { reducedMotion: "reduce" },
    });

    test("减少动态偏好下不被接管", async ({ page }) => {
      await openHomepage(page);
      await expect.poll(() => activeSection(page)).toBe(0);

      const tops = await moduleTops(page);
      await page.mouse.move(500, 400);
      await page.mouse.wheel(0, 120);
      await waitForSnapSettle(page);

      const y = await readScrollY(page);
      expect(y).toBeGreaterThan(0);
      expect(y).toBeLessThan(tops[1] / 2);
    });
  });
});
