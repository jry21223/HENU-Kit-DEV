import { expect, test, type Page } from "@playwright/test";
import { waitForHydration } from "./support/readability-routes";
import {
  collect,
  recordFromFirstFrame,
  relapses,
  sampledBeforeHydration,
  settle,
  throttleCPU,
  type Probe,
} from "./support/entrance";
import { mockGuestGateway } from "./support/gateway";

/**
 * 首页 02 刷题模块在学习数据晚到时的入场（#557）。
 *
 * 生产配置打开 V2 读取（本 spec 属于 test:e2e:stats），掌握度要等水合之后的请求回来才渲染。
 * 面板淡入和打字机每次挂载只建一次，数据到达时只给新出现的掌握度条做生长：再叠一个面板的
 * from() 会把已经藏起来的面板当成终点，面板就再也不出现；读者已经在看时重来一遍打字机，
 * 解析文字会被清空重打。这组跑在 next dev 上，React 严格模式会把挂载演一遍卸载再挂载，
 * 撤销之后入场要重新建起来，这里一并覆盖。
 */

const STATS = {
  request_id: "req_module_stats",
  data: {
    total_answers: 4,
    correct_answers: 3,
    accuracy: 75,
    streak_days: 2,
    mastery: [
      {
        bank_id: "10ca9b18-c303-4b7a-ab14-1241e41b665a",
        label: "计算机基础",
        value: 50,
        total_questions: 4,
        correct_questions: 2,
      },
    ],
  },
};

/** 解析文字的最后一句：打字机打完时才出现。 */
const TYPED_TO_THE_END = "该公式只在 x→0 时成立。";

/** 学习数据先扣住，调用返回的函数才回应：模拟水合之后才到的掌握度。 */
async function holdStats(page: Page) {
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await mockGuestGateway(page);
  await page.route("**/api/v1/practice/stats*", async (route) => {
    await gate;
    // 刷新前发出的那一次请求已经随旧页面作废，回应它会报错，不算这条用例的失败。
    await route.fulfill({ json: STATS }).catch(() => {});
  });
  return () => release();
}

const practice = (page: Page) => page.locator("section:has([data-terminal])");

async function scrollToPractice(page: Page) {
  const top = await practice(page).evaluate((section) => section.getBoundingClientRect().top + window.scrollY);
  await page.evaluate((y) => window.scrollTo(0, y), top);
  await expect.poll(() => page.evaluate(() => Math.round(window.scrollY))).toBe(Math.round(top));
}

const scaleX = (element: Element) => {
  const transform = getComputedStyle(element).transform;
  return transform === "none" ? 1 : new DOMMatrixReadOnly(transform).a;
};

/** 面板淡入完、解析打完、掌握度条长满。 */
async function expectFullyEntered(page: Page) {
  const section = practice(page);
  await expect
    .poll(() => section.locator("[data-terminal]").evaluate((element) => Number(getComputedStyle(element).opacity)), {
      timeout: 20_000,
    })
    .toBe(1);
  await expect(section.locator("[data-typewriter]")).toContainText(TYPED_TO_THE_END, { timeout: 20_000 });
  await expect
    .poll(() => section.locator("[data-bar]").first().evaluate(scaleX), { timeout: 20_000 })
    .toBe(1);
}

test.use({ viewport: { width: 1440, height: 900 } });
test.describe.configure({ timeout: 150_000 });

test("数据在读者滚到之前就到了：滚到时面板淡入、解析打完、掌握度条长满", async ({ page }) => {
  const release = await holdStats(page);
  await page.goto("/");
  await waitForHydration(page);
  release();
  await expect(practice(page).locator("[data-bar]")).toHaveCount(1);

  await scrollToPractice(page);
  await expectFullyEntered(page);
});

test("读者先到、数据后到：掌握度条从 0 长出来，面板和解析不重来", async ({ page }) => {
  const release = await holdStats(page);
  await page.goto("/");
  await waitForHydration(page);
  await scrollToPractice(page);
  const section = practice(page);
  await expect
    .poll(() => section.locator("[data-terminal]").evaluate((element) => Number(getComputedStyle(element).opacity)), {
      timeout: 20_000,
    })
    .toBe(1);
  await expect(section.locator("[data-typewriter]")).toContainText(TYPED_TO_THE_END, { timeout: 20_000 });

  // 入场走完之后，逐帧记下面板透明度、解析长度和掌握度条的 scaleX，再放数据进来。
  await page.evaluate(() => {
    const w = window as unknown as { __after?: Array<{ opacity: number; text: number; bar: number | null }> };
    w.__after = [];
    const loop = () => {
      const terminal = document.querySelector("[data-terminal]");
      const text = document.querySelector("[data-typewriter]");
      const bar = document.querySelector("[data-bar]");
      const transform = bar ? getComputedStyle(bar).transform : "none";
      w.__after?.push({
        opacity: terminal ? Number(getComputedStyle(terminal).opacity) : -1,
        text: (text?.textContent ?? "").length,
        bar: bar ? (transform === "none" ? 1 : new DOMMatrixReadOnly(transform).a) : null,
      });
      requestAnimationFrame(loop);
    };
    requestAnimationFrame(loop);
  });
  release();
  await expectFullyEntered(page);

  const samples = await page.evaluate(
    () => (window as unknown as { __after?: Array<{ opacity: number; text: number; bar: number | null }> }).__after ?? []
  );
  expect(samples.length).toBeGreaterThan(0);
  expect(samples.filter((sample) => sample.opacity < 0.99), "面板在数据到达后又淡出过").toEqual([]);
  expect(new Set(samples.map((sample) => sample.text)).size, "解析文字在数据到达后被清空重打").toBe(1);
  const bars = samples.map((sample) => sample.bar).filter((value): value is number => value !== null);
  expect(bars[0], "掌握度条一出现就是满的，没有生长").toBeLessThan(0.5);
  expect(bars[bars.length - 1]).toBe(1);
});

test("停在刷题模块时刷新、数据后到：已经画出的面板和解析不先消失", async ({ page }) => {
  const probes: Probe[] = [
    { selector: "[data-terminal]", measure: "opacity", reveal: "up", identity: "position" },
    { selector: "[data-typewriter]", measure: "textLength", reveal: "up", identity: "position" },
    { selector: "[data-bar]", measure: "scaleX", reveal: "up", identity: "position" },
  ];
  const release = await holdStats(page);
  await page.goto("/");
  await waitForHydration(page);
  await scrollToPractice(page);

  await recordFromFirstFrame(page, probes);
  // 与 first-screen-entrance 的中间位置用例一样降速，保证采到水合之前的帧。
  await throttleCPU(page);
  await page.reload({ waitUntil: "commit" });
  await waitForHydration(page);
  release();
  await expect(practice(page).locator("[data-bar]")).toHaveCount(1);
  await settle(page);

  // 前提：浏览器确实把页面恢复到了这个模块，否则这条断言测的是首屏。
  const restored = await practice(page).evaluate((section) => {
    const box = section.getBoundingClientRect();
    return box.top < window.innerHeight * 0.5 && box.bottom > window.innerHeight * 0.5;
  });
  expect(restored, "刷新后停在刷题模块").toBe(true);

  const series = await collect(page);
  expect(sampledBeforeHydration(series), "采到了水合之前的帧").toBe(true);
  expect(series.some((entry) => entry.label.startsWith("[data-bar]")), "采到了数据到达后才出现的掌握度条").toBe(true);
  expect(relapses(series, probes)).toEqual([]);
  await expect(practice(page).locator("[data-typewriter]")).toContainText(TYPED_TO_THE_END);
});
