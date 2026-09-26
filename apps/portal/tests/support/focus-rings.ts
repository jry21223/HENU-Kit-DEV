import type { Locator, Page } from "@playwright/test";

/**
 * 焦点框不被滚动容器裁掉（DESIGN_SYSTEM §13）：从当前焦点往后按 Tab，进到 scroller 里以后
 * 逐个走完里面的控件，量每个焦点框的外缘（边框盒向外 outline-offset + outline-width）有没有
 * 伸出 scroller 的裁剪框（内边距盒）。控件被滑出一截时，画在里面的焦点框也跟着出去，同样算被裁。
 *
 * 返回在 scroller 里停过几次（stops），以及每处被裁的描述（clipped，空数组才算达标）。
 */
export async function tabThroughScroller(
  page: Page,
  scroller: Locator,
  { maxPresses = 10 }: { maxPresses?: number } = {}
): Promise<{ stops: number; clipped: string[] }> {
  const focusInside = () => scroller.evaluate((element) => element.contains(document.activeElement));
  for (let presses = 0; presses < maxPresses && !(await focusInside()); presses += 1) {
    await page.keyboard.press("Tab");
  }

  const clipped: string[] = [];
  let stops = 0;
  // 上限只防死循环：焦点离开 scroller 就停。
  while (stops < 50 && (await focusInside())) {
    stops += 1;
    const report = await scroller.evaluate((element) => {
      const focused = document.activeElement as HTMLElement;
      const name = (focused.getAttribute("aria-label") || focused.textContent || "").replace(/\s+/g, " ").trim();
      const style = getComputedStyle(focused);
      if (style.outlineStyle === "none") return `${name} 没有焦点框`;
      const reach = parseFloat(style.outlineOffset) + parseFloat(style.outlineWidth);
      const box = focused.getBoundingClientRect();
      const clip = element.getBoundingClientRect();
      const clipStyle = getComputedStyle(element);
      const cut = [
        ["上", clip.top + parseFloat(clipStyle.borderTopWidth) - (box.top - reach)],
        ["下", box.bottom + reach - (clip.bottom - parseFloat(clipStyle.borderBottomWidth))],
        ["左", clip.left + parseFloat(clipStyle.borderLeftWidth) - (box.left - reach)],
        ["右", box.right + reach - (clip.right - parseFloat(clipStyle.borderRightWidth))],
      ].filter(([, px]) => (px as number) > 0.5);
      return cut.length ? `${name} 被裁掉 ${cut.map(([side, px]) => `${side} ${px}px`).join("、")}` : "";
    });
    if (report) clipped.push(report);
    await page.keyboard.press("Tab");
  }
  return { stops, clipped };
}
