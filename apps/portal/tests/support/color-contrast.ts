import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

/**
 * 文字对比度检查（#536；DESIGN_SYSTEM.md 第 3 节“文字配色”）：只跑 axe 的 color-contrast 规则。
 *
 * - 读屏隐藏（aria-hidden）的元素不在检查范围：WCAG 1.4.3 对纯装饰没有对比度要求，十字对位标记、
 *   档案卡缩写这类装饰可以保持浅色。aria-hidden 只能加在装饰上，不能拿来藏信息。
 * - axe 算不出底色时只把文字记为“无法判断”，不算违规。为了让这些文字也被检查，扫描前去掉
 *   两类挡住判断的装饰，见 revealTextBackgrounds。axe 仍会跳过只有一个字符的文字（它按图标处理），
 *   如档位“夯”，这类文字的配色按 DESIGN_SYSTEM.md 的规则核对。
 */

/**
 * 去掉挡住底色判断的装饰，文字按它真正压着的底色检查：
 * - 背景图：Portal 的背景图只有工程图纸网格（1px 细线，见 globals.css 的 bg-blueprint 与求职雷达），
 *   axe 遇到背景图就不判断上面的文字；
 * - 读屏隐藏的装饰：细线插图（首屏图纸 SVG）和静止时移出视野的色块（磁吸按钮的橙色填充）
 *   会被 axe 当成“盖在文字下面的图片 / 元素”，同样不判断。它们本身也不在检查范围，见文件头。
 */
export async function revealTextBackgrounds(page: Page) {
  await page.addStyleTag({
    content: `
      *, *::before, *::after { background-image: none !important; }
      [aria-hidden="true"] { visibility: hidden !important; }
    `,
  });
}

type ContrastData = { fgColor?: string; bgColor?: string; contrastRatio?: number; expectedContrastRatio?: string };

/**
 * 当前滚动位置下 axe 报出的对比度不足节点，每条写明是哪一处、什么颜色、差多少，
 * 失败时不用再开 trace 查。include 给出时只扫这个选择器命中的元素。
 */
export async function contrastViolations(page: Page, include?: string): Promise<string[]> {
  const builder = new AxeBuilder({ page }).withRules(["color-contrast"]);
  const results = await (include ? builder.include(include) : builder).analyze();
  return results.violations.flatMap((violation) =>
    violation.nodes.map((node) => {
      const data = (node.any[0]?.data ?? {}) as ContrastData;
      const text = node.html.replace(/\s+/g, " ").slice(0, 120);
      return `${node.target.join(" ")}：${data.fgColor} / ${data.bgColor} = ${data.contrastRatio}:1，需 ${data.expectedContrastRatio}（${text}）`;
    })
  );
}
