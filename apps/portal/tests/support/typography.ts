import type { Page } from "@playwright/test";

/**
 * 字号与中文字距检查（#536；DESIGN_SYSTEM.md 第 4 节“排版”）。
 *
 * - 看得见的文字不小于 12px。只有读屏隐藏（aria-hidden）的纯装饰拉丁标签可以小到 10px；
 *   装饰里的中文照样不小于 12px。
 * - 含中文的文字不加宽字距：字距不超过 0.05em。中文本身是等宽方块，再拉开字距会读成
 *   一个字一个字的碎片；宽字距只加在拉丁字母、数字和等宽编号上，混排标签把拉丁部分拆成单独的 span。
 *
 * 逐个文本节点检查它所在元素的计算样式；输入框另查自身字号和占位文字的字距。
 * 看不见的文字不算：display: none、visibility: hidden、没有尺寸的，以及只留给读屏的 sr-only。
 * SVG 里的文字随 viewBox 缩放，按屏幕上渲染出来的大小算。
 */

export const MIN_TEXT_PX = 12;
export const MIN_DECORATIVE_PX = 10;
export const MAX_CJK_TRACKING_EM = 0.05;

/** 当前页面上字号或中文字距不达标的文字，每条写明是哪一处、多大、差多少。 */
export async function typographyViolations(page: Page): Promise<string[]> {
  return page.evaluate(
    ({ minText, minDecorative, maxTrackingEm }) => {
      // 汉字（含扩展 A 与兼容区）、中文标点和全角符号。
      const CJK = /[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff00-\uffef]/;
      const findings: string[] = [];
      const round = (value: number) => Math.round(value * 100) / 100;

      const describe = (element: Element, text: string) => {
        const className = element.getAttribute("class") ?? "";
        const snippet = text.replace(/\s+/g, " ").trim().slice(0, 40);
        return `<${element.tagName.toLowerCase()} class="${className.slice(0, 90)}">「${snippet}」`;
      };

      /**
       * 某一层祖先 display: none（SVG 里的文字 checkVisibility 看不出来），或者把它裁到 1px 以内
       * （sr-only 这类只留给读屏的文字）。
       */
      const hiddenByAncestor = (element: Element) => {
        for (let node: Element | null = element; node; node = node.parentElement) {
          const style = getComputedStyle(node);
          if (style.display === "none") return true;
          if (style.display === "contents") continue;
          if (style.overflow === "visible" && style.clipPath === "none" && style.clip === "auto") continue;
          const box = node.getBoundingClientRect();
          if (box.width <= 1 || box.height <= 1) return true;
        }
        return false;
      };

      const visible = (element: Element) =>
        element.checkVisibility({ visibilityProperty: true }) && !hiddenByAncestor(element);

      /** 屏幕上的字号：SVG 文字的 font-size 是 viewBox 里的单位，乘上缩放比例。 */
      const renderedSize = (element: Element, style: CSSStyleDeclaration) => {
        const size = Number.parseFloat(style.fontSize);
        if (!(element instanceof SVGGraphicsElement)) return size;
        const matrix = element.getScreenCTM();
        return matrix ? size * Math.sqrt(Math.abs(matrix.a * matrix.d - matrix.b * matrix.c)) : size;
      };

      const check = (element: Element, text: string, style: CSSStyleDeclaration) => {
        const cjk = CJK.test(text);
        const decorative = element.closest("[aria-hidden='true']") !== null;
        const floor = decorative && !cjk ? minDecorative : minText;
        const size = renderedSize(element, style);
        if (size + 0.01 < floor) {
          findings.push(`字号 ${round(size)}px，需 ≥${floor}px：${describe(element, text)}`);
        }
        if (!cjk) return;
        const spacing = style.letterSpacing === "normal" ? 0 : Number.parseFloat(style.letterSpacing);
        const tracking = spacing / Number.parseFloat(style.fontSize);
        if (tracking > maxTrackingEm + 0.001) {
          findings.push(`中文字距 ${round(tracking)}em，需 ≤${maxTrackingEm}em：${describe(element, text)}`);
        }
      };

      const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        const text = node.textContent ?? "";
        if (!text.trim()) continue;
        const element = node.parentElement;
        if (!element || element.closest("script, style, noscript, template, title")) continue;
        const range = document.createRange();
        range.selectNodeContents(node);
        if (!Array.from(range.getClientRects()).some((box) => box.width > 0 && box.height > 0)) continue;
        if (!visible(element)) continue;
        check(element, text, getComputedStyle(element));
      }

      // 输入框：输入的内容按输入框自身的字号显示；没有内容时显示占位文字。
      const controls = document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(
        "input, textarea, select"
      );
      for (const control of controls) {
        if (control.matches("[type=hidden], [type=checkbox], [type=radio], [type=range], [type=file], [type=color]")) {
          continue;
        }
        const box = control.getBoundingClientRect();
        if (box.width === 0 || box.height === 0 || !visible(control)) continue;
        if (control instanceof HTMLSelectElement) {
          check(control, control.selectedOptions[0]?.text ?? "", getComputedStyle(control));
          continue;
        }
        const placeholder = control.value === "" ? control.placeholder : "";
        check(
          control,
          placeholder || control.value,
          getComputedStyle(control, placeholder ? "::placeholder" : null)
        );
      }
      return findings;
    },
    { minText: MIN_TEXT_PX, minDecorative: MIN_DECORATIVE_PX, maxTrackingEm: MAX_CJK_TRACKING_EM }
  );
}
