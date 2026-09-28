import { expect, type Page } from "@playwright/test";

/**
 * 入场逐帧采样（#537、#557）：在导航前装一个采样器（addInitScript 先于页面任何脚本运行），
 * 从首帧开始记录被测元素的读数，断言每个元素一旦显露就不再往回退。用法和缘由见
 * tests/first-screen-entrance.spec.ts 开头的说明；首页刷题模块数据晚到的用例也用它。
 */

export type Measure = "opacity" | "scaleX" | "textLength" | "stroke";

export interface Probe {
  selector: string;
  measure: Measure;
  /** 数值往哪个方向走是在「显露」：opacity、生长线往上走；遮住标题的橙色块往下走。 */
  reveal: "up" | "down";
  /**
   * 默认按「文字 + 同文字里的第几个」认元素；文字本身会变的元素（打字机，以及包着它的面板）
   * 改按它在匹配结果里的位置认。
   */
  identity?: "text" | "position";
}

export interface Series {
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
export async function recordFromFirstFrame(page: Page, probes: Probe[]) {
  await page.addInitScript((probes: Probe[]) => {
    const series: Record<string, Series> = {};
    const state = { series, last: 0 };
    window.__entrance = state;

    const read = (element: Element, measure: Measure) => {
      if (measure === "textLength") return (element.textContent ?? "").length;
      const style = getComputedStyle(element);
      if (measure === "stroke") {
        // SVG 线画出来了多少：描线动画用一段与整条线等长的虚线、偏移一整条来藏线；
        // 实线和流动的短虚线都算画全了。
        const dashes = style.strokeDasharray.split(/[\s,]+/).map(parseFloat).filter((n) => !Number.isNaN(n));
        const length = (element as SVGGeometryElement).getTotalLength();
        if (dashes.length === 0 || dashes[0] < length - 0.5) return 1;
        return Math.max(0, 1 - Math.abs(parseFloat(style.strokeDashoffset) || 0) / length);
      }
      if (measure === "opacity") return Number(style.opacity);
      return style.transform === "none" ? 1 : new DOMMatrixReadOnly(style.transform).a;
    };

    const tick = (now: number) => {
      // 水合标记由根布局的 ScrollMemory 在客户端外壳就绪后写上。
      const hydrated = document.documentElement.dataset.scrollMemory === "ready";
      probes.forEach((probe, probeIndex) => {
        const seen = new Map<string, number>();
        document.querySelectorAll(probe.selector).forEach((element, position) => {
          const text = (element.textContent ?? "").replace(/\s+/g, "");
          const nth = seen.get(text) ?? 0;
          seen.set(text, nth + 1);
          const key = probe.identity === "position" ? `${probeIndex}|@${position}` : `${probeIndex}|${text}|${nth}`;
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
export async function throttleCPU(page: Page) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
}

/** 等水合完成，再多采 settleMs：入场动画（最长约 2s）和水合后的任何重播都要落在窗口里。 */
export async function settle(page: Page, settleMs = 3_000) {
  await page.waitForSelector("html[data-scroll-memory='ready']", { state: "attached", timeout: 90_000 });
  const from = await page.evaluate(() => performance.now());
  await expect
    .poll(() => page.evaluate(() => window.__entrance?.last ?? 0), { timeout: 30_000 })
    .toBeGreaterThan(from + settleMs);
}

export async function collect(page: Page) {
  const state = await page.evaluate(() => window.__entrance);
  if (!state) throw new Error("采样器没有装上");
  return Object.values(state.series);
}

/** 每个元素第一次往回退的那一帧；全部单调时返回空数组。 */
export function relapses(series: Series[], probes: Probe[]) {
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
export function sampledBeforeHydration(series: Series[]) {
  return series.some((entry) => entry.points.some(([, , hydrated]) => !hydrated));
}
