import { expect, test, type Page } from "@playwright/test";

/**
 * 整屏吸附按**手势意图**判定（#509）：松手时用这次手势的净位移与峰值速度决定要不要走
 * 一屏，一次手势只判一次。净位移门槛是**视口高的 6%**（1024×800 下 48px），速度通道
 * 另要净位移过视口高的 3%（24px）且峰值速度 ≥ 600 px/s。
 *
 * 要修的是**过灵敏**：Observer 的 `tolerance: 12` 是桶累计阈值，位移累加到 12px 就回调
 * 一次，所以十几像素的手抖、误触、点击前的位移都会翻一整屏（#507 的实测证据）。
 *
 * 两条现状必须保住，不许被收紧的门槛改坏：
 * - **慢速长拖动仍然切屏**（8px/帧 × 20 帧量级）。`tolerance` 是桶累计阈值，逐帧的小位
 *   移会被累加，所以这条**现在就是绿的**；理由见 #507「已被实测推翻、不要再复用的旧表
 *   述」——任何以「慢拖失效」为由的改动都是错的。
 * - 单帧大位移（一帧 30px）不跳两屏。
 *
 * 阈值本身不进端到端断言（px/s 与像素门槛依赖帧节奏）：这里的手势幅度与门槛保持距离，
 * 只断言落在第几屏；阈值两侧由 `src/lib/navigation/gesture-intent.test.ts` 钉死。
 * 接管门槛（<768px、prefers-reduced-motion: reduce 退回普通滚动）与首末屏边界「停住不动」
 * 由 homepage-snap-scroll.spec.ts 守着，本文件不重复；这里只补一条边界上的空转不花掉
 * 这次手势（#506 的契约）。
 */

/** hero + 资料库 + 刷题 + 美食 + 互助 + 求职 + footer */
const SECTION_COUNT = 7;

/**
 * 逐段合成一次触摸手势。默认只发一段位移（一次手势 = 一次判定，方向用例不必让投递时序
 * 参与）；steps/stepDelayMs 留给「真实连续拖动」——慢速长拖动与抖动都要多帧累计才成立，
 * 因为单帧 ≤13px 的合成 touchmove 在 Chrome 里根本投递不到页面（#507 的能力边界声明）。
 *
 * 与 homepage-snap-scroll.spec.ts、latent-go-origin.spec.ts 的同名助手保持一致。
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
 * 一次手势内按折线走（不抬手），用来构造「同一次手势里先反后正」——手指先下滑回读、
 * 再反手上滑继续往下读。
 */
async function swipeFingerPath(page: Page, waypoints: number[], stepDelayMs = 40) {
  const cdp = await page.context().newCDPSession(page);
  const x = 200;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x, y: waypoints[0] }],
  });
  for (const y of waypoints.slice(1)) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x, y }],
    });
    await page.waitForTimeout(stepDelayMs);
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}

/** 按下即抬起，中间没有位移：读者只是点了一下。 */
async function tapFinger(page: Page, y: number) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: 200, y }],
  });
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}

/**
 * 走到一半被系统取消的手势（第二根手指、浏览器接管）：抬手事件是 touchcancel。位移足够
 * 构成一次意图，但读者没有机会表达「我要走一屏」，所以不该替他决定。
 */
async function cancelFinger(page: Page, from: number, to: number, steps = 4, stepDelayMs = 40) {
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
  await cdp.send("Input.dispatchTouchEvent", { type: "touchCancel", touchPoints: [] });
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

/**
 * 各模块顶端：接管时每次输入都会正好落在其中一个上，不接管时不会。
 *
 * 取整对齐 `latent-go-origin.spec.ts` 的 `layout()`：断言拿它和取整后的 scrollY 比，
 * 两边都取整才不会因为一次小数偏移变成隐性 flake。
 */
async function moduleTops(page: Page) {
  return page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>(".snap-screen")).map((section) =>
      Math.round(section.offsetTop)
    )
  );
}

/**
 * 断言读者停在某一屏的顶端。
 *
 * 先用 poll 等它**真的到**那一屏顶端，再等落定后复核一次：缓动尾段的位移会小到连续两次采样
 * 取整后一样，机器被抢占时更会停在中途伪装成落定——先 poll 就不会拿动画中途的读数当结论，
 * 后面的复核负责确认没有第二次起跳把它挪走。
 */
async function expectLandedOn(page: Page, screen: number) {
  const tops = await moduleTops(page);
  await expect.poll(async () => Math.round(await readScrollY(page))).toBe(tops[screen]);
  await waitForSnapSettle(page);
  expect(Math.round(await readScrollY(page))).toBe(tops[screen]);
  expect(await activeSection(page)).toBe(screen);
}

test.describe("轻触与手抖不翻屏", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("点一下（按下抬起、没有位移）不翻屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    await tapFinger(page, 400);
    await expectLandedOn(page, 0);
  });

  test("8px × 2 帧的手抖不翻屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 累计 −16px：过了旧的桶累计门槛（12px），却远不是一次「走一屏」的手势。
    await swipeFinger(page, 400, 384, 2, 40);
    await expectLandedOn(page, 0);
  });

  test("4px × 4 帧的手抖不翻屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 同样累计 −16px，只是摊到四帧里：桶在第三帧就满了，第四帧的零头不该另算一次。
    await swipeFinger(page, 400, 384, 4, 40);
    await expectLandedOn(page, 0);
  });

  test("被系统取消的手势不翻屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 位移 −400px 足够构成一次意图，但手势是被系统取消的（第二根手指、浏览器接管），
    // 读者没机会表达「我要走一屏」，所以不判定。
    await cancelFinger(page, 640, 240);
    await expectLandedOn(page, 0);
  });
});

test.describe("现状已具备、不许改坏的手势", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("8px/帧 × 20 帧的慢速长拖动仍然切一屏", async ({ page }) => {
    // 手势本身要走 20 帧（约 1.6s，8px/帧 ~80ms 是 #507 实测用的节奏），前后还各有一次整屏
    // 动画，冷编译时默认的 45s 预算不够稳；给它三倍预算，而不是把断言放宽。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 累计 −160px：逐帧小位移靠桶累计照样构成一次意图（#507 已实测证伪「慢拖失效」）。
    // 收紧门槛之后它必须还能切屏，而且只切一屏。
    await swipeFinger(page, 640, 480, 20, 80);
    await expectLandedOn(page, 1);
  });

  test("单帧 30px 的大位移不跳两屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    await swipeFinger(page, 400, 370);
    await waitForSnapSettle(page);

    // 一帧就到位的 30px 远不到一次手势的意图（6% = 48px），但无论判不判它走一屏，
    // 都不许越过第 1 屏。
    const tops = await moduleTops(page);
    expect([tops[0], tops[1]]).toContain(Math.round(await readScrollY(page)));
    expect(await activeSection(page)).toBeLessThanOrEqual(1);
  });
});

test.describe("同一手势内的方向", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("一次手势里先反后正：以最后的方向为准，且只走一屏", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    await page.keyboard.press("PageDown");
    await expect.poll(() => activeSection(page)).toBe(1);
    await waitForSnapSettle(page);

    // 第 1 屏上的一次手势：手指先下滑 200px（想回上一屏），再反手上滑 300px（最后想做
    // 的是继续往下读）。松手时只看最新方向：推到第 2 屏，而不是退回第 0 屏、也不是连走
    // 两屏到第 3 屏。
    await swipeFingerPath(page, [400, 500, 600, 500, 400, 300]);
    await expectLandedOn(page, 2);
  });

  test("首屏边界上的空转不花掉这次手势", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 第 0 屏没有上一屏：先手指下滑 300px（读者想回读，go(-1) 空转），再反手上滑 500px
    // （最后想做的是继续往下读）。#506 的「边界空转不消费手势」现在由「抬手前什么都不消
    // 费」保证，所以这一滑仍要切到第 1 屏。
    await swipeFingerPath(page, [400, 700, 200]);
    await expectLandedOn(page, 1);
  });
});

test.describe("三种输入的方向语义对照", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("同一物理意图在触摸、键盘与滚轮上都不反向", async ({ page }) => {
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 触摸上滑 = 往下读
    await swipeFinger(page, 640, 240);
    await expectLandedOn(page, 1);

    // 键盘 ArrowUp = 往上读
    await page.keyboard.press("ArrowUp");
    await expectLandedOn(page, 0);

    // 滚轮下滚 = 往下读
    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, 400);
    await expectLandedOn(page, 1);

    // 滚轮上滚 = 往上读
    await page.mouse.wheel(0, -400);
    await expectLandedOn(page, 0);
  });
});
