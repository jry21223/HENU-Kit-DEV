import { expect, test, type Page } from "@playwright/test";

/**
 * 滚轮/触控板的一次突发（一个窗口）最多切一屏（#510）。
 *
 * #507 留下的待证实项 (b)：滚轮路径**没有手势锁**——它靠 `animating` 布尔量挡动画期间的
 * tick，而 1.1s 动画一结束，同一次物理滚动（触控板惯性尾巴）继续产生的 tick 就会被当成
 * 第二次滚动，一次轻扫连跳两屏。（b）在本票里先测量后决定，结论是**确认缺陷**：
 *
 * 在 1024×800、`hasTouch` 的真实 Chromium 里派发衰减滚轮序列（deltaY 40 → 5，跨度 2.5/3.0/3.5s），
 * 读者从第 0 屏被一路推到第 3/3/4 屏——3–4 次起跳，除第一次以外的每一次都由**补间结束之后**
 * 到达的 tick 触发（实测：后一段缓动都在前一段落点稳定后 ~150–250ms 内开始）。原始数据见 PR
 * 正文；本文件的用例把这个结论固化成回归。
 *
 * 窗口与阈值的实测依据（`src/lib/navigation/gesture-intent.ts` 里的常量注释写着同一份）：
 * - 突发的跨度窗口取 **3500ms**：它要盖住「整段补间（页面内 rAF 采样到视野停止移动为止：
 *   空载 ~980ms、被抢占 ~2000ms；名义 1.1s）+ 落在它之后的惯性尾巴（合成事件复现出的衰减
 *   尾巴实测到 ~2.5s）」，否则就是留了个「动画一结束、尾巴还在流」的缺口，那正是缺陷本身。
 * - 静默多久算「这次滚动手势结束」取 **600ms**：下界是尾巴的 tick 间隔——页面收到的事件实测
 *   min 12–25 / 中位 17–44 / max 44–90ms（判定看到的是 Observer 的桶回调，更粗：尾巴里单个
 *   事件 5–20px、最坏 3 个事件凑一回调 ≈ 270ms，所以 600ms 是最坏回调间隔的 2.2 倍，见
 *   `gesture-intent.ts` 的常量注释）；上界是读者有意一屏一屏滚的节奏，必须大于它否则第二下会被
 *   吞掉。**上界的实测证据在 PR 正文**：把界设成 1500ms 时，1.3s 节奏的 4 个刻度只走 2 屏；
 *   600ms 下 1.3s / 1.5s / 1.8s 都是 4 个刻度走 4 屏。本文件的「等补间走完再滚一下」与
 *   「离散鼠标滚轮看到落点就滚下一格」两条用例钉的是这条结论的**行为面**（落点驱动的节奏不被
 *   吞、不迟滞）；上界的具体取值由常量注释指向 PR 正文那份测量（注释里写明「原始数据见 PR
 *   正文」），端到端不断言它——负载高时补间会拖长，那是环境而不是被测行为。
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
 * 就发生在那里（实测改造前：读者从第 0 屏被推到第 3 屏）。
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
  // 采样间隔比 #509 的默认值（100/250/500/1000ms）密：调用方要在补间刚走完时就尽快看到落点
  // （「离散鼠标滚轮」那条用例还要拿这个时刻量两次刻度之间的静默），默认间隔会晚发现最多 1s。
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
    // 一次轻扫的量级：deltaY 40 → 5，序列**铺满大半个跨度窗口**（目标 2500ms，留 1000ms 的
    // 余量给派发本身的开销——每个 tick 一次 CDP 往返，机器一忙整个循环会拖长），所以它一定
    // 覆盖「补间结束之后、窗口合上之前」那一段——#510 的缺陷正是发生在这里（实测改造前：
    // 读者从第 0 屏被推到第 3 屏）。
    //
    // 用例的前提是「铺满的是同一段连续的滚动」：判定看的是 `Observer` 的桶回调，所以下面按
    // `tolerance: 12` 把页面收到的 wheel 事件聚成回调序列再量间隔。机器被抢占到回调之间出现
    // ≥ 静默界的停顿时，这段序列按定义已经是两次手势，那次观察不作数——重铺一次（最多三次），
    // 三次都没铺成就明确失败，而不是拿一个前提不成立的落点去下结论。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    // 监听器只挂一次：每轮只清空缓冲区，否则重试时同一个事件会被记 2、3 次，重建出来的回调
    // 序列比真实更密，前提检查反而在重试时变松。
    await page.evaluate(() => {
      const w = window as unknown as { __ticks?: Array<[number, number]> };
      w.__ticks = [];
      window.addEventListener(
        "wheel",
        (e) => w.__ticks?.push([Math.round(performance.now()), e.deltaY]),
        { capture: true, passive: true }
      );
    });

    let maxGap = Number.POSITIVE_INFINITY;
    for (let attempt = 0; attempt < 3; attempt += 1) {
      if (attempt > 0) {
        // 回到第 0 屏再铺一次（上一轮可能已经把读者推到了后面的屏）。
        for (let step = 0; step < SECTION_COUNT && (await activeSection(page)) > 0; step += 1) {
          await page.keyboard.press("PageUp");
          await waitForSnapSettle(page);
        }
        await expect.poll(() => activeSection(page)).toBe(0);
      }

      await page.evaluate(() => {
        (window as unknown as { __ticks?: Array<[number, number]> }).__ticks = [];
      });

      const burstMs = await burstAcrossWindow(page, 2500);
      // 铺满的是**同一个**突发：真拖过 3500ms 的跨度界，这条用例就不再说它想说的那件事了。
      expect(burstMs).toBeLessThan(3500);

      maxGap = await page.evaluate(() => {
        const ticks = (window as unknown as { __ticks?: Array<[number, number]> }).__ticks ?? [];
        // Observer 的桶：|deltaY| 累加到 `tolerance`（12）才回调一次并清零
        // （`Observer.js:190-210`），所以判定看到的时刻是这些「回调时刻」。
        const callbacks: number[] = [];
        let bucket = 0;
        for (const [at, dy] of ticks) {
          bucket += Math.abs(dy);
          if (bucket >= 12) {
            callbacks.push(at);
            bucket = 0;
          }
        }
        return callbacks
          .slice(1)
          .reduce((max, t, i) => Math.max(max, t - callbacks[i]), 0);
      });
      if (maxGap < BURST_GAP_MS) break;
    }
    // 前提：整段里没有 ≥ 静默界的停顿——那样的停顿按定义就是新的一次手势，页面多走一屏是
    // **对的**。三次都没铺成时在这里明确失败，而不是让下面的落点断言以假乱真。
    expect(maxGap).toBeLessThan(BURST_GAP_MS);

    // 一个窗口 = 一屏：从第 0 屏落到第 1 屏，之后不许再被尾巴上的 tick 推走。
    await expectLandedOn(page, 1);
  });

  test("补间期间开始的滚轮突发，落定后马上补跳一屏", async ({ page }) => {
    // 补间占着的时候，滚轮那几下走不成：判定不该被花掉，否则读者「动画期间滚了一下、落定后
    // 应该走一屏」的意图就被整段吞掉。也不能反过来多走一屏——那正是本票的缺陷。
    //
    // 用键盘先起一屏（PageDown），在它的补间期间一直滚，然后看两件事（都在页面里量，不猜
    // 补间多长——负载高时它会从 1.1s 拖到 2s 上下）：
    //   1. 落点是「PageDown 那一屏 + 滚轮那一屏」= 第 2 屏，一屏不多；
    //   2. 从键盘那一屏落定，到滚轮那一屏**开始**动，间隔要短——读者那一下没被吞掉，补间一
    //      释放就该落地（实测：合成刻度每 200ms 一记，落定后下一记就走）。
    // 只留第 1 条的话，「动画期间把突发花掉、直到窗口重开才动」的实现也能蒙混过去。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    const tops = await moduleTops(page);
    await page.evaluate(() => {
      const w = window as unknown as { __raf?: Array<[number, number]> };
      w.__raf = [];
      const loop = () => {
        w.__raf?.push([Math.round(performance.now()), Math.round(window.scrollY)]);
        requestAnimationFrame(loop);
      };
      requestAnimationFrame(loop);
    });

    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: 500, y: 400, button: "none" });
    await page.keyboard.press("PageDown");
    await cdp.send("Input.dispatchMouseEvent", {
      type: "mouseWheel",
      x: 500,
      y: 400,
      deltaX: 0,
      deltaY: 40,
      button: "none",
    });
    // 每 200ms 再补一记，直到页面越过第 1 屏（滚轮那一屏真的开始走了），最多 24 记。
    for (let i = 0; i < 24; i += 1) {
      const moved = await page.evaluate((target) => window.scrollY > target + 1, tops[1]);
      if (moved) break;
      await page.waitForTimeout(200);
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

    // 1. 一个窗口只走一屏：第 2 屏，之后不许再动。
    await expectLandedOn(page, 2);

    // 2. 补间一释放就落地。
    const delayMs = await page.evaluate(
      (target) => {
        const raf = (window as unknown as { __raf?: Array<[number, number]> }).__raf ?? [];
        const landed = raf.find(([, y]) => y >= target - 1);
        if (!landed) return Number.POSITIVE_INFINITY;
        const stepped = raf.find(([t, y]) => t > landed[0] && y > target + 1);
        return stepped ? stepped[0] - landed[0] : Number.POSITIVE_INFINITY;
      },
      tops[1]
    );
    expect(delayMs).toBeLessThan(1000);
  });

});
test.describe("滚轮方向语义与离散刻度不回退", () => {
  test.use({ viewport: { width: 1024, height: 800 }, hasTouch: true });

  test("离散鼠标滚轮看到落点就滚下一格，仍然一屏一屏走", async ({ page }) => {
    // 「不许迟滞」的对照：滚一格 → 一看到落点就滚下一格（快速轮询，不等落定）→ 再滚一格。
    // 两次刻度之间的静默是「补间 + 一次轮询」= 1.1s 量级，大于 600ms 的静默界，所以每一格都
    // 是新的一次手势、各走一屏。
    //
    // 注意：这条用例**不再声称**自己钉住了静默界的上界。把界设成 1500ms 时这 1.1–1.3s 的第二格
    // 会被吞掉——那份实测证据在 PR 正文里（1.3s 节奏的 4 个刻度只走 2 屏），本文件只断言「读者
    // 看到落点就滚」的节奏不被吞、不迟滞；上界的取值由常量注释指向那份测量。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    const tops = await moduleTops(page);
    await page.evaluate(() => {
      const w = window as unknown as { __ticks?: number[] };
      w.__ticks = [];
      window.addEventListener("wheel", () => w.__ticks?.push(Math.round(performance.now())), {
        capture: true,
        passive: true,
      });
    });

    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, 100);
    await expect
      .poll(async () => Math.round(await readScrollY(page)), {
        timeout: 20_000,
        intervals: [50, 50, 50, 100, 100, 200],
      })
      .toBe(tops[1]);
    await page.mouse.wheel(0, 100);
    await expectLandedOn(page, 2);

    // 两次刻度之间的静默必须已经过了静默界——否则这一格会被算进同一次突发，用例就不再是
    // 「新的一次手势」了。（上界不断言：负载高时补间会拖长，那属于环境，不属于被测行为。）
    const gap = await page.evaluate(() => {
      const ticks = (window as unknown as { __ticks?: number[] }).__ticks ?? [];
      return ticks.length >= 2 ? ticks[1] - ticks[0] : 0;
    });
    expect(gap).toBeGreaterThanOrEqual(BURST_GAP_MS);
  });

  test("等补间走完再滚一下仍然每个刻度走一屏", async ({ page }) => {
    // 读者有意一屏一屏滚的节奏：滚一下 → 等这屏的补间走完、落定 → 再滚一下。两次之间的静默
    // 是「补间 + 落定采样」（≈2s 量级，负载高时更长），远大于 600ms 的静默界，所以每一格都是
    // 新的一次手势——滚轮不因为加了聚合而变迟滞（#507 用户故事 9）。
    //
    // 它**不声称**自己钉住了静默界的上界：这么长的静默即使在旧的 1500ms 界下也是新的一次手势。
    // 会落在 [600ms, 1500ms) 区间的是上一条（看到落点就滚，静默只有补间 + 一次轮询），但那条
    // 在负载高时前提会漂，所以也不断言上界；上界的证据是 PR 正文里那份测量（1500ms 界下 1.3s
    // 节奏的 4 个刻度只走 2 屏），常量注释指向它。
    test.slow();
    await openHomepage(page);
    await waitForSnapTakeover(page);
    await expect.poll(() => activeSection(page)).toBe(0);

    await page.mouse.move(500, 400);
    await page.mouse.wheel(0, 100);
    await expectLandedOn(page, 1);
    await page.mouse.wheel(0, 100);
    await expectLandedOn(page, 2);

    // 上滚一个刻度退回上一屏：滚轮方向语义不变（正数向下、负数向上）。
    await page.mouse.wheel(0, -100);
    await expectLandedOn(page, 1);
  });
});
