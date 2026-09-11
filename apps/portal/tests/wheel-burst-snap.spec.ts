import { expect, test, type Page } from "@playwright/test";

/**
 * 滚轮/触控板的一次突发最多切一屏（#510）。
 *
 * #507 留下的待证实项 (b)：滚轮路径**没有手势锁**——它靠 `animating` 布尔量挡动画期间的
 * tick，而 1.1s 动画一结束，同一次物理滚动（触控板惯性尾巴）继续产生的 tick 就会被当成
 * 第二次滚动，一次轻扫连跳两屏。（b）在本票里先测量后决定，结论是**确认缺陷**：
 *
 * 在 1024×800、`hasTouch` 的真实 Chromium 里派发衰减滚轮序列（deltaY 40 → 5，跨度 2.5/3.0/3.5s），
 * 读者从第 0 屏被一路推到第 2/2/3 屏——2–3 次起跳，除第一次以外的每一次都由**补间结束之后**
 * 到达的 tick 触发（实测：第二次缓动都在第一次落点稳定后 ~150ms 内开始）。原始数据见 PR 正文；
 * 本文件的用例把这个结论固化成回归。
 *
 * 窗口与阈值的实测依据（`src/lib/navigation/gesture-intent.ts` 里的常量注释写着同一份）：
 * - 突发的跨度窗口取 **3500ms**：它要盖住「整段补间（页面内 rAF 采样到视野停止移动为止：
 *   空载 ~980ms、被抢占 ~2000ms；名义 1.1s）+ 落在它之后的惯性尾巴（合成事件复现出的衰减
 *   尾巴实测到 ~2.5s）」，否则就是留了个「动画一结束、尾巴还在流」的缺口，那正是缺陷本身。
 * - 一次突发内部的 tick 间隔实测 min 12–25 / 中位 17–44 / max 44–90ms（合成序列；真实触控板
 *   事件的节流目标是 60Hz ≈ 16.7ms，只会更密）。静默多久算「这次滚动手势结束」取 **600ms**：
 *   它必须远大于尾巴的 tick 间隔（6.7× 实测最大值），这样整段尾巴留在同一个突发里；又必须小于
 *   读者有意一屏一屏滚的节奏（等补间走完 ~1.1s 再加反应时间，本文件用 1.3s 与 1.8s 两种刻度
 *   间隔量它），否则「等动画走完再滚一下」会被吞掉、滚轮变迟滞。
 *
 * 不许退化的对照：**离散鼠标滚轮仍一屏一屏走**——间隔 1.3s / 1.8s 的刻度各是一次手势，每个
 * 刻度走一屏；现状本来就如此（`animating` 会吃掉动画期间的刻度），这里把它钉住。1500ms 的
 * 静默界会把 1.3s 那一档吞掉，所以这条对照正是把界压到 600ms 的理由。
 *
 * 近似程度的边界（本文件不掩盖）：合成 wheel 事件无法复现真实触控板的 rAF 去抖与动量曲线，
 * 只能复现「衰减的 tick 序列跨过 1.1s 补间结束」这一结构，尾巴能拖多长也只是本机合成出来的
 * 量级；真实设备验收按 #510 的部署批次在生产上补做。
 */

/** hero + 资料库 + 刷题 + 美食 + 互助 + 求职 + footer */
const SECTION_COUNT = 7;

/**
 * 与 `gesture-intent.ts` 的 `WHEEL_BURST_GAP_MS` 同值：测试文件不 import 应用源码（Playwright
 * 的转换读不了这个仓库的 tsconfig），所以这里写一份并在下面用它检查「这段 tick 是不是一次连续
 * 突发」这个前提。改常量时两处一起改——单测按阈值两侧钉死了那个值，这里只在前提检查里用。
 */
const BURST_GAP_MS = 600;

/**
 * 一次**铺满整个跨度窗口**的衰减突发：从 40 衰减到 5，一直发到 `windowMs` 用完为止，所以
 * 无论 tick 多密、补间多慢，序列一定覆盖「补间结束之后、窗口合上之前」那一段——#510 的缺陷
 * 就发生在那里（实测 pre-fix：页面从第 1 屏走到第 2 屏）。
 *
 * 固定长度的突发靠不住：合成事件的实际间隔随机器漂（实测 14–75ms），尾巴落在补间结束之前
 * 还是之后也跟着漂；铺满窗口则把「补间结束之后、窗口合上之前」这一段整个包住。
 */
async function burstAcrossWindow(page: Page, windowMs: number, from = 40, to = 5) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: 500, y: 400, button: "none" });
  const started = Date.now();
  for (;;) {
    const elapsed = Date.now() - started;
    if (elapsed >= windowMs) break;
    const delta = Math.max(to, Math.round(from * Math.pow(to / from, elapsed / windowMs)));
    await cdp.send("Input.dispatchMouseEvent", {
      type: "mouseWheel",
      x: 500,
      y: 400,
      deltaX: 0,
      deltaY: delta,
      button: "none",
    });
  }
  await cdp.detach();
  return Date.now() - started;
}

/** 定间隔的刻度序列（离散鼠标滚轮对照）：每个间隔一次，不用等落定。 */
async function wheelTicks(page: Page, deltas: number[], gapMs: number) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: 500, y: 400, button: "none" });
  for (let i = 0; i < deltas.length; i += 1) {
    await cdp.send("Input.dispatchMouseEvent", {
      type: "mouseWheel",
      x: 500,
      y: 400,
      deltaX: 0,
      deltaY: deltas[i],
      button: "none",
    });
    if (i < deltas.length - 1) await page.waitForTimeout(gapMs);
  }
  await cdp.detach();
}

/**
 * 打开首页并等到 7 个整屏模块就位、客户端外壳水合完成。dev 下首次访问要现编译，
 * 生产是预构建，所以给冷编译留出时间，别让它冒充失败。
 *
 * 水合标记只说明外壳挂上了：它不证明整屏接管已经生效（ScrollMemory 与 SnapScroll
 * 是两个 effect，先后提交）。要断言方向或边界的用例接着调 waitForSnapTakeover。
 *
 * 与 homepage-snap-scroll.spec.ts、gesture-intent-snap.spec.ts、latent-go-origin.spec.ts
 * 的同名助手保持一致（本文件是第四份拷贝；抽成共享 fixture 是后续的整理项）。
 */
async function openHomepage(page: Page) {
  await page.goto("/", { waitUntil: "load" });
  await expect(page.locator(".snap-screen")).toHaveCount(SECTION_COUNT, { timeout: 30_000 });
  await page.waitForSelector("html[data-scroll-memory='ready']", { timeout: 30_000 });
}

/**
 * 等到整屏接管确实生效：键盘路径与滚轮/触摸路径同属 SnapScroll 的接管，按一次 PageDown
 * 应当整屏跳一块；没接管时它只会原生滚动若干像素。跑一个来回把状态留在第一屏。
 *
 * 没有这一步，滚轮事件可能在接管装上之前发出，断言就会在正确代码上变红。
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
 * 取整对齐 latent-go-origin.spec.ts 的 `layout()`：断言拿它和取整后的 scrollY 比，
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
 * 这次输入之后读者停在某一屏顶端，而且**没有再动**。
 *
 * 先 poll 等它真的到那一屏顶端，再等落定后复核一次：缓动尾段的位移会小到连续两次采样取整
 * 后一样，机器被抢占时更会停在中途伪装成落定——先 poll 就不会拿动画中途的读数当结论，后面
 * 的复核负责确认没有第二次起跳把它挪走。这正是本票要抓的东西：缺陷的表现就是「先到第 1 屏、
 * 尾巴上的 tick 又把它推到第 2 屏」。
 *
 * 沿用 #509 的 poll-then-verify 写法。
 */
async function expectLandedOn(page: Page, screen: number) {
  const tops = await moduleTops(page);
  // 采样间隔比 #509 的默认值（100/250/500/1000ms）密：调用方拿它当「补间什么时候结束」的
  // 时间戳用，默认间隔会让落点被晚发现最多 1s，量出来的补间时长就掺了采样误差。
  await expect
    .poll(async () => Math.round(await readScrollY(page)), {
      timeout: 20_000,
      intervals: [50, 50, 50, 100, 100, 200],
    })
    .toBe(tops[screen]);
  await waitForSnapSettle(page);
  expect(Math.round(await readScrollY(page))).toBe(tops[screen]);
  expect(await activeSection(page)).toBe(screen);
}

test.describe("一次滚轮突发只走一屏", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("触控板惯性尾巴不连跳两屏", async ({ page }) => {
    // 一次轻扫的量级：deltaY 40 → 5，序列**铺满大半个跨度窗口**（目标 2500ms，留 1000ms 的余量
    // 给派发本身的开销——每个 tick 一次 CDP 往返，机器一忙整个循环会拖长），所以它一定覆盖
    // 「补间结束之后、窗口合上之前」那一段——#510 的缺陷正是发生在这里（实测改造前：读者从
    // 第 0 屏被推到第 2 屏）。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 先记下页面真正收到的 tick 时刻：这条用例的前提是「铺满的是同一段连续的滚动」。
    await page.evaluate(() => {
      const w = window as unknown as { __ticks?: number[] };
      w.__ticks = [];
      window.addEventListener(
        "wheel",
        () => w.__ticks?.push(Math.round(performance.now())),
        { capture: true, passive: true }
      );
    });

    const burstMs = await burstAcrossWindow(page, 2500);
    // 铺满的是**同一个**突发：真拖过 3500ms 的跨度界，这条用例就不再说它想说的那件事了，
    // 所以这里宁可显式失败，也不要让它悄悄退化成一个跨两次手势的序列。
    expect(burstMs).toBeLessThan(3500);

    // 前提本身也要成立：整段里不能出现 ≥ 静默界的停顿——那样的停顿按定义就是新的一次手势，
    // 页面多走一屏是**对的**。机器被抢占到这一步时，这条断言会明确说出原因，而不是让下面的
    // 落点断言以假乱真。
    const maxGap = await page.evaluate(() => {
      const ticks = (window as unknown as { __ticks?: number[] }).__ticks ?? [];
      return ticks.slice(1).reduce((max, t, i) => Math.max(max, t - ticks[i]), 0);
    });
    expect(maxGap).toBeLessThan(BURST_GAP_MS);

    // 一次突发 = 一屏：从第 0 屏落到第 1 屏，之后不许再被尾巴上的 tick 推走。
    await expectLandedOn(page, 1);
  });

  test("补间期间开始的滚轮突发，落定后只再走一屏", async ({ page }) => {
    // 补间占着的时候，滚轮那几下走不成：判定不该被花掉，否则读者「动画期间滚了一下、落定后
    // 应该走一屏」的意图就被整段吞掉（#507 证据表里「动画中滚轮 | 被 animating 吃掉，落定后
    // 正常」这条契约）。也不能反过来多走一屏——那正是本票的缺陷。
    //
    // 用键盘先起一屏（PageDown），在它 1.1s 的补间期间一直滚（每 200ms 一个刻度，共 2400ms，
    // 覆盖到补间结束后还有刻度），断言读者最后**只**多走了一屏。这条盯的是反方向的退化：
    // 如果「动画期间滚的那几下」被算成花掉的判定，读者落定后就不会再走，落点会停在第 1 屏。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    const tops = await moduleTops(page);
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: 500, y: 400, button: "none" });
    await page.keyboard.press("PageDown");
    const started = Date.now();
    for (let i = 0; i < 12; i += 1) {
      const due = started + i * 200;
      const wait = due - Date.now();
      if (wait > 0) await page.waitForTimeout(wait);
      await cdp.send("Input.dispatchMouseEvent", {
        type: "mouseWheel",
        x: 500,
        y: 400,
        deltaX: 0,
        deltaY: 40,
        button: "none",
      });
    }
    await cdp.detach();

    // 一次突发只走一屏：PageDown 那一屏 + 滚轮那一屏 = 第 2 屏，之后不许再动。
    await expectLandedOn(page, 2);
  });
});

test.describe("滚轮方向语义与离散刻度不回退", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("离散鼠标滚轮仍然一屏一屏走", async ({ page }) => {
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 间隔 1.8s 的刻度（大于 600ms 的静默界）：每个刻度都是一次独立手势，各走一屏。
    await wheelTicks(page, [100, 100, 100], 1800);
    await expectLandedOn(page, 3);
  });

  test("等补间走完再滚一下（1.3s 节奏）仍然每个刻度走一屏", async ({ page }) => {
    // 读者有意一屏一屏滚的节奏：滚一下 → 等这屏 1.1s 的补间走完、看清落点 → 再滚一下，
    // 实测节奏在 1.2–1.5s。静默界必须小于它，否则第二下会被当成同一次突发吞掉、滚轮变迟滞
    // （#507 用户故事 9）。这条用例是把这个界从 1500ms 压到 600ms 的直接理由。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 每 1.3s 一个刻度。最后一个刻度留在 1/1300 的余量里，所以断言落到第 2 屏而不是更多。
    await wheelTicks(page, [100, 100], 1300);
    await expectLandedOn(page, 2);

    // 上滚一个刻度退回上一屏：滚轮方向语义不变（正数向下、负数向上）。
    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, -100);
    await expectLandedOn(page, 1);
  });
});
