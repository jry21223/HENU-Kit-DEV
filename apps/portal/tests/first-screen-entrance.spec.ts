import { expect, test, type Page } from "@playwright/test";

/**
 * 首屏入场只能单调地变可见（#537）。
 *
 * SSR 输出的 HTML 已经把标题画出来了；入场动画如果用 GSAP 的 from() 在水合时才把
 * 元素设成初始隐藏态，用户就会看到「可见 → 瞬间消失 → 重新进场」。慢 CPU 上水合
 * 晚，这个空档长到肉眼可见（实测 390×844、4× 降速：847ms 可见，2180ms 被清零）。
 *
 * 这里在导航前装一个逐帧采样器（addInitScript 先于页面任何脚本运行），从首帧开始记录
 * 被测元素的 opacity / scaleX，断言每个元素一旦显露就不再往回退。元素按「选择器 + 文字
 * + 同文字里的第几个」追踪而不是按对象身份：水合失败时 React 会换掉整棵 DOM，CSS 动画随新
 * 节点从头播放——对用户同样是一次重播，按身份追踪会把它当成一个新元素放过去。
 */

type Measure = "opacity" | "scaleX";

interface Probe {
  selector: string;
  measure: Measure;
  /** 数值往哪个方向走是在「显露」：opacity、生长线往上走；遮住标题的橙色块往下走。 */
  reveal: "up" | "down";
}

interface Series {
  probe: number;
  label: string;
  /** [rAF 时间戳, 读数, 采样时是否已水合] */
  points: [number, number, boolean][];
}

declare global {
  interface Window {
    __entrance?: { series: Record<string, Series>; last: number };
  }
}

/** 装采样器。只能用序列化进页面的代码，不能引用这个文件里的其他函数。 */
async function recordFromFirstFrame(page: Page, probes: Probe[]) {
  await page.addInitScript((probes: Probe[]) => {
    const series: Record<string, Series> = {};
    const state = { series, last: 0 };
    window.__entrance = state;

    const read = (element: Element, measure: Measure) => {
      const style = getComputedStyle(element);
      if (measure === "opacity") return Number(style.opacity);
      return style.transform === "none" ? 1 : new DOMMatrixReadOnly(style.transform).a;
    };

    const tick = (now: number) => {
      // 水合标记由根布局的 ScrollMemory 在客户端外壳就绪后写上。
      const hydrated = document.documentElement.dataset.scrollMemory === "ready";
      probes.forEach((probe, probeIndex) => {
        const seen = new Map<string, number>();
        document.querySelectorAll(probe.selector).forEach((element) => {
          const text = (element.textContent ?? "").replace(/\s+/g, "");
          const nth = seen.get(text) ?? 0;
          seen.set(text, nth + 1);
          const key = `${probeIndex}|${text}|${nth}`;
          series[key] ??= { probe: probeIndex, label: `${probe.selector}「${text.slice(0, 12)}」#${nth}`, points: [] };
          series[key].points.push([Math.round(now), read(element, probe.measure), hydrated]);
        });
      });
      state.last = now;
      // 不设上限：采样要一直活到 settle() 结束（水合最多等 90s，再多采 settleMs），
      // 页面随测试结束关闭。
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }, probes);
}

/** 模拟问题里的慢设备：4× CPU 降速。 */
async function throttleCPU(page: Page) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
}

/** 等水合完成，再多采 settleMs：入场动画（最长约 2s）和水合后的任何重播都要落在窗口里。 */
async function settle(page: Page, settleMs = 3_000) {
  await page.waitForSelector("html[data-scroll-memory='ready']", { state: "attached", timeout: 90_000 });
  const from = await page.evaluate(() => performance.now());
  await expect
    .poll(() => page.evaluate(() => window.__entrance?.last ?? 0), { timeout: 30_000 })
    .toBeGreaterThan(from + settleMs);
}

async function collect(page: Page) {
  const state = await page.evaluate(() => window.__entrance);
  if (!state) throw new Error("采样器没有装上");
  return Object.values(state.series);
}

/** 每个元素第一次往回退的那一帧；全部单调时返回空数组。 */
function relapses(series: Series[], probes: Probe[]) {
  const EPSILON = 0.02;
  const found: string[] = [];
  for (const entry of series) {
    const { reveal } = probes[entry.probe];
    let best = entry.points[0]?.[1];
    for (const [time, value, hydrated] of entry.points) {
      const regressed = reveal === "up" ? value < best - EPSILON : value > best + EPSILON;
      if (regressed) {
        found.push(`${entry.label} 在 ${time}ms（${hydrated ? "水合完成后" : "水合完成前"}）从 ${best.toFixed(2)} 退回 ${value.toFixed(2)}`);
        break;
      }
      best = reveal === "up" ? Math.max(best, value) : Math.min(best, value);
    }
  }
  return found;
}

/** 前提：确实采到了水合之前（SSR 已画出）的帧，否则单调断言是空转。 */
function sampledBeforeHydration(series: Series[]) {
  return series.some((entry) => entry.points.some(([, , hydrated]) => !hydrated));
}

const MOBILE = { width: 390, height: 844 };

/** 资料库目录里放一份资料，页面走到「数据已到达」的状态。 */
async function mockLibraryCatalog(page: Page) {
  await page.route("**/api/v1/library/materials", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        materials: [
          {
            id: "library-entrance", type: "note", subject: "高等数学",
            title: "极限复习笔记", author: "资料库收录", intro: "", toc: [], pages: [],
            price: 0, previewPages: 0, downloads: 1, downloadAvailable: true, fileSize: 1024,
          },
        ],
        statistics: {
          releaseId: "0123456789abcdef0123456789abcdef01234567-0123456789abcdef",
          materialCount: 1,
          downloadStarts: 1,
          countingSince: "2026-08-11T00:00:00Z",
          asOf: "2026-08-11T01:00:00Z",
        },
        request_id: "req_library_entrance",
      }),
    })
  );
}

const HOME_PROBES: Probe[] = [
  { selector: "[data-hero-line]", measure: "opacity", reveal: "up" },
  { selector: "[data-hero-gridline]", measure: "scaleX", reveal: "up" },
  { selector: "[data-hero-wipe]", measure: "scaleX", reveal: "down" },
];

test.describe("首屏入场不重播（慢 CPU）", () => {
  test.use({ viewport: MOBILE });
  test.describe.configure({ timeout: 150_000 });

  test("首页 Hero 标题从首帧起只会变得更可见", async ({ page }) => {
    await recordFromFirstFrame(page, HOME_PROBES);
    await throttleCPU(page);
    await page.goto("/", { waitUntil: "commit" });
    await settle(page);

    const series = await collect(page);
    await expect(page.locator("[data-hero-line]"), "三行标题都在").toHaveCount(3);
    expect(sampledBeforeHydration(series), "采到了水合之前的帧").toBe(true);
    expect(relapses(series, HOME_PROBES)).toEqual([]);

    // 入场结束：标题完全可见，橙色擦除块收起。
    for (const entry of series) {
      const last = entry.points.at(-1)?.[1];
      expect(last, entry.label).toBeCloseTo(HOME_PROBES[entry.probe].reveal === "up" ? 1 : 0, 2);
    }
  });

  test("资料库子站 Hero 与页面内容块从首帧起只会变得更可见", async ({ page }) => {
    const probes: Probe[] = [
      { selector: "[data-hero-title]", measure: "opacity", reveal: "up" },
      { selector: "[data-hero-line]", measure: "scaleX", reveal: "up" },
      { selector: "main [data-enter]", measure: "opacity", reveal: "up" },
    ];
    await mockLibraryCatalog(page);
    await recordFromFirstFrame(page, probes);
    await throttleCPU(page);
    await page.goto("/library", { waitUntil: "commit" });
    await expect(page.getByRole("heading", { name: "极限复习笔记" })).toBeVisible({ timeout: 90_000 });
    await settle(page);

    const series = await collect(page);
    await expect(page.locator("[data-hero-title]"), "子站 Hero 的四块文案都在").toHaveCount(4);
    expect(sampledBeforeHydration(series), "采到了水合之前的帧").toBe(true);
    expect(relapses(series, probes)).toEqual([]);
  });

  test("美食榜数据到达后，已经显示的页头不再重播", async ({ page }) => {
    const probes: Probe[] = [{ selector: "main [data-enter]", measure: "opacity", reveal: "up" }];
    await page.route("**/api/v1/food/posts", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          posts: [
            {
              id: "entrance-1", campus: "minglun", title: "鼓楼夜市", excerpt: "第一次来开封很适合从这里开始。",
              blocks: [{ type: "p", text: "选择多、烟火气足。" }], author: "学生编辑部", likes: 90, stars: 20,
              tags: ["夜市", "夯"], shop: { name: "鼓楼夜市" }, time: "07-16", hidden: false, images: [],
            },
          ],
          request_id: "req_food_entrance",
        }),
      })
    );
    await recordFromFirstFrame(page, probes);
    await throttleCPU(page);
    await page.goto("/food", { waitUntil: "commit" });
    await expect(page.getByText("鼓楼夜市").first()).toBeVisible({ timeout: 90_000 });
    await settle(page);

    const series = await collect(page);
    expect(sampledBeforeHydration(series), "采到了水合之前的帧").toBe(true);
    expect(relapses(series, probes)).toEqual([]);
  });

  test("题库首页 Hero 从首帧起入场，只会变得更可见", async ({ page }) => {
    const probes: Probe[] = [
      { selector: "[data-hero-title]", measure: "opacity", reveal: "up" },
      { selector: "main [data-enter]", measure: "opacity", reveal: "up" },
    ];
    // 未登录，题库与学习数据都不可用：Hero 的文案不依赖接口数据。
    await page.route("**/api/v1/**", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "DEPENDENCY_UNAVAILABLE", message: "unavailable" }, request_id: "req_entrance" }),
      })
    );
    await page.route("**/api/v1/session", (route) =>
      route.fulfill({ status: 401, contentType: "application/json", body: "{}" })
    );
    await recordFromFirstFrame(page, probes);
    await throttleCPU(page);
    await page.goto("/practice", { waitUntil: "commit" });
    await settle(page);

    const series = await collect(page);
    await expect(page.locator("[data-hero-title]"), "题库 Hero 的五块内容都在").toHaveCount(5);
    expect(sampledBeforeHydration(series), "采到了水合之前的帧").toBe(true);
    expect(relapses(series, probes)).toEqual([]);

    // 不重播也不等于硬加载时没有入场：和首页、子站 Hero 一样从首帧起由初始态进场，最后完全可见。
    const hero = series.filter((entry) => entry.probe === 0);
    expect(hero.length, "采到了题库 Hero").toBe(5);
    for (const entry of hero) {
      expect(entry.points[0][1], `${entry.label} 从初始态开始入场`).toBeLessThan(0.5);
      expect(entry.points.at(-1)?.[1], entry.label).toBeCloseTo(1, 2);
    }
  });
});

test.describe("客户端导航", () => {
  test.use({ viewport: MOBILE });

  // 不重播不等于不入场：客户端导航挂上的新内容此前没画过，仍然从初始态淡入。
  test("从首页进入资料库，新页面的内容仍有入场且只增不减", async ({ page }) => {
    const probes: Probe[] = [
      { selector: "main [data-enter]", measure: "opacity", reveal: "up" },
      { selector: "[data-hero-title]", measure: "opacity", reveal: "up" },
    ];
    await mockLibraryCatalog(page);
    await recordFromFirstFrame(page, probes);
    await page.goto("/");
    await page.waitForSelector("html[data-scroll-memory='ready']", { state: "attached" });
    await page.getByRole("navigation", { name: "常用功能" }).getByRole("link", { name: "找资料", exact: true }).click();
    await expect(page).toHaveURL(/\/library$/);
    await expect(page.getByRole("heading", { name: "极限复习笔记" })).toBeVisible();
    await settle(page);

    const series = await collect(page);
    for (const probe of [0, 1]) {
      const entries = series.filter((entry) => entry.probe === probe);
      expect(entries.length, probes[probe].selector).toBeGreaterThan(0);
      expect(entries.some((entry) => entry.points[0][1] < 0.5), `${probes[probe].selector} 从初始态开始入场`).toBe(true);
    }
    expect(relapses(series, probes)).toEqual([]);
  });
});

test.describe("减少动态效果", () => {
  test.use({ viewport: MOBILE, contextOptions: { reducedMotion: "reduce" } });

  test("首页 Hero 直接静态展示", async ({ page }) => {
    await recordFromFirstFrame(page, HOME_PROBES);
    await page.goto("/", { waitUntil: "commit" });
    await settle(page, 1_500);

    const series = await collect(page);
    expect(series.length).toBeGreaterThan(0);
    for (const entry of series) {
      const expected = HOME_PROBES[entry.probe].reveal === "up" ? 1 : 0;
      for (const [, value] of entry.points) expect(value, entry.label).toBeCloseTo(expected, 2);
    }
    const running = await page.locator("main").evaluate((main) => main.getAnimations({ subtree: true }).length);
    expect(running, "没有进行中的入场动画").toBe(0);
  });
});

// 眉标旁旋转的 ® 是纯装饰的慢速循环：动效开启时转，减少动态设置下停住，符号本身留着。
test.describe("眉标旁的 ®", () => {
  test.use({ viewport: MOBILE });

  const rotation = (page: Page) => page.locator("[data-hero-reg]").evaluate((element) => getComputedStyle(element).transform);

  test("动效开启时慢慢转", async ({ page }) => {
    await page.goto("/");
    await expect.poll(() => rotation(page), { timeout: 30_000 }).not.toBe("none");
  });

  test.describe("减少动态设置", () => {
    test.use({ contextOptions: { reducedMotion: "reduce" } });

    test("停着不转", async ({ page }) => {
      await page.goto("/");
      await page.waitForSelector("html[data-scroll-memory='ready']", { state: "attached", timeout: 90_000 });
      const reg = page.locator("[data-hero-reg]");
      await expect(reg).toHaveText("®");
      // 连着读 30 帧（约半秒）：在转的话，这段时间里至少转过好几度。
      const transforms = await reg.evaluate(async (element) => {
        const seen = new Set<string>();
        for (let frame = 0; frame < 30; frame += 1) {
          await new Promise((resolve) => requestAnimationFrame(resolve));
          seen.add(getComputedStyle(element).transform);
        }
        return [...seen];
      });
      expect(transforms).toEqual(["none"]);
    });
  });
});

test.describe("脚本没有运行", () => {
  test.use({ viewport: MOBILE, javaScriptEnabled: false });

  test("首页 Hero 标题最终仍然可见", async ({ page }) => {
    await page.goto("/");
    const lines = page.locator("[data-hero-line]");
    await expect(lines).toHaveCount(3);
    for (const line of await lines.all()) await expect(line).toHaveCSS("opacity", "1");
    await expect(page.locator("[data-hero-wipe]")).toHaveCSS("transform", "matrix(0, 0, 0, 1, 0, 0)");
  });
});
