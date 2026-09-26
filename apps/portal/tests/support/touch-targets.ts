import { expect, type Page } from "@playwright/test";

/**
 * 触控目标检查（DESIGN_SYSTEM §13；#543）：页面上每个可见的 a[href]、button、input、select，
 * 其 getBoundingClientRect 宽高都不小于 44。
 *
 * 只按 WCAG 2.5.8 的例外放过两类，别的一律要达标：
 * - 句中的行内链接：链接本身是 display: inline，且与句子的其他文字同在一个段落里
 *   （如登录页同意告知里的《用户协议》），高度受那一行文字约束。被放过的链接由调用方逐个写明，
 *   不在预期里的也算失败；
 * - 读屏隐藏的装饰元素：自身或祖先带 aria-hidden="true"。
 */

export const MIN_TARGET = 44;

type Target = { control: string; width: number; height: number };

/**
 * 范围内所有不达标的控件（undersized），以及因「句中行内链接」而放过的小控件（inSentence）。
 * 后者也列出来，由调用方写明预期：例外只能落在已知的那几个链接上。
 */
async function measureTargets(
  page: Page,
  within: string
): Promise<{ undersized: Target[]; inSentence: string[] }> {
  return page.evaluate(
    ({ min, within }) => {
      const isInSentence = (element: Element) => {
        if (getComputedStyle(element).display !== "inline") return false;
        const parent = element.parentElement;
        if (!parent) return false;
        return Array.from(parent.childNodes).some(
          (node) => node !== element && node.nodeType === Node.TEXT_NODE && node.textContent?.trim()
        );
      };
      const describe = (element: Element) => {
        const name =
          element.getAttribute("aria-label") ||
          (element as HTMLInputElement).placeholder ||
          element.textContent?.replace(/\s+/g, " ").trim() ||
          element.getAttribute("href") ||
          "";
        return `<${element.tagName.toLowerCase()}> ${name.slice(0, 40)}`;
      };

      const undersized: { control: string; width: number; height: number }[] = [];
      const inSentence: string[] = [];
      for (const root of document.querySelectorAll(within)) {
        for (const element of root.querySelectorAll("a[href], button, input, select")) {
          const box = element.getBoundingClientRect();
          const style = getComputedStyle(element);
          if (box.width === 0 || box.height === 0 || style.visibility === "hidden") continue;
          if (element.closest("[aria-hidden='true']")) continue;
          // 亚像素取整：43.99 视为 44。
          if (box.width + 0.5 >= min && box.height + 0.5 >= min) continue;
          if (isInSentence(element)) {
            inSentence.push(describe(element));
            continue;
          }
          undersized.push({
            control: describe(element),
            width: Math.round(box.width),
            height: Math.round(box.height),
          });
        }
      }
      return { undersized, inSentence };
    },
    { min: MIN_TARGET, within }
  );
}

/**
 * 断言范围（默认整页 body）内的控件都不小于 44×44；inSentence 写明按句中链接放过的控件。
 */
export async function expectTouchTargets(
  page: Page,
  what: string,
  { inSentence = [] as string[], within = "body" } = {}
) {
  const measured = await measureTargets(page, within);
  expect(measured.undersized, `${what}：以下控件小于 ${MIN_TARGET}×${MIN_TARGET}`).toEqual([]);
  expect(measured.inSentence, `${what}：按句中链接放过的控件`).toEqual(inSentence);
}
