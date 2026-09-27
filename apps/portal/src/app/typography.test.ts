import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

/**
 * 最小字号与中文字距（#536；DESIGN_SYSTEM.md 第 4 节“排版”），逐个 JSX 元素检查源码：
 * - 小于 12px 的字号类（如 text-[10px]）只能加在读屏隐藏（aria-hidden）的纯装饰拉丁标签上，
 *   而且不小于 10px；
 * - 加了字距（tracking-wide 及更宽，或正值的 tracking-[…em]）的元素，写在里面的文字不能有中文。
 *   中文用 tracking-normal；中英混排的标签把拉丁部分拆成单独的 span，只给它加字距。
 *
 * 只看源码里写死的类名和文字。运行时才知道的文字（接口数据、变量）由 tests/typography.spec.ts
 * 在首页、五个子站首页和登录页上按计算样式检查。
 */

const SRC = path.resolve(__dirname, "..");
const MIN_TEXT_PX = 12;
const MIN_DECORATIVE_PX = 10;
// 汉字（含扩展 A 与兼容区）、中文标点和全角符号。
const CJK = /[\u3000-\u303f\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff00-\uffef]/;

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(full);
    return /\.tsx$/.test(entry.name) && !/\.test\.tsx$/.test(entry.name) ? [full] : [];
  });
}

type JsxNode = ts.JsxElement | ts.JsxSelfClosingElement;

/** 字符串与模板字面量里写死的那一段。 */
function isLiteralText(node: ts.Node): node is ts.StringLiteralLike | ts.TemplateLiteralLikeNode {
  return ts.isStringLiteralLike(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node);
}

/** 结果会被渲染出来的运算；比较（a === "b"）两边的字符串不会显示。 */
const RENDERED_OPERATORS = new Set<ts.SyntaxKind>([
  ts.SyntaxKind.AmpersandAmpersandToken,
  ts.SyntaxKind.BarBarToken,
  ts.SyntaxKind.QuestionQuestionToken,
  ts.SyntaxKind.PlusToken,
]);

function attributesOf(node: JsxNode) {
  return (ts.isJsxElement(node) ? node.openingElement : node).attributes.properties;
}

function attribute(node: JsxNode, name: string) {
  return attributesOf(node).find(
    (property): property is ts.JsxAttribute => ts.isJsxAttribute(property) && property.name.getText() === name
  );
}

/** className 里所有写死的类名：字符串、模板字面量，含 cn(...) 与条件表达式里的分支。 */
function classTokens(node: JsxNode): string[] {
  const initializer = attribute(node, "className")?.initializer;
  if (!initializer) return [];
  const parts: string[] = [];
  const visit = (child: ts.Node) => {
    if (isLiteralText(child)) parts.push(child.text);
    ts.forEachChild(child, visit);
  };
  visit(initializer);
  return parts.join(" ").split(/\s+/).filter(Boolean);
}

function isAriaHidden(node: JsxNode) {
  const hidden = attribute(node, "aria-hidden");
  if (!hidden) return false;
  if (!hidden.initializer) return true;
  const text = hidden.initializer.getText().replace(/[{}"'\s]/g, "");
  return text === "true";
}

/** 作用在文字上的字距（placeholder: 这类只作用在占位文字上的不算）；em 为单位，没有字距类时为 null。 */
function tracking(tokens: string[]): number | null {
  let result: number | null = null;
  for (const token of tokens) {
    const variants = token.split(":");
    const utility = variants.pop() ?? "";
    if (variants.includes("placeholder")) continue;
    const match = utility.match(/^tracking-(.+)$/);
    if (!match) continue;
    const value = match[1];
    const em =
      value === "widest" ? 0.1
      : value === "wider" ? 0.05
      : value === "wide" ? 0.025
      : value.startsWith("[") ? Number.parseFloat(value.slice(1)) || 0
      : 0; // normal、tight、tighter
    result = Math.max(result ?? em, em);
  }
  return result;
}

function smallestPx(tokens: string[]): number | null {
  const sizes = tokens
    .map((token) => token.split(":").pop()?.match(/^text-\[(\d+(?:\.\d+)?)px\]$/)?.[1])
    .filter((size): size is string => Boolean(size))
    .map(Number);
  return sizes.length ? Math.min(...sizes) : null;
}

/**
 * 元素里写死的文字：JSX 文本，以及 {…} 里条件分支、map 回调写出的字符串和子元素。
 * 一直往下读，直到遇到自己设了字距的子元素（它另算）。
 */
function inheritedText(node: JsxNode): string {
  if (!ts.isJsxElement(node)) return "";
  let text = "";
  const visit = (child: ts.Node) => {
    if (ts.isJsxText(child)) text += child.text;
    else if (ts.isJsxElement(child)) {
      if (tracking(classTokens(child)) === null) child.children.forEach(visit);
    } else if (ts.isJsxSelfClosingElement(child) || ts.isJsxAttributes(child)) {
      // 自闭合元素没有文字；属性不是正文。
    } else if (isLiteralText(child)) {
      text += ` ${child.text} `;
    } else if (ts.isBinaryExpression(child) && !RENDERED_OPERATORS.has(child.operatorToken.kind)) {
      // a === "b" 这类比较里的字符串不会显示出来。
    } else ts.forEachChild(child, visit);
  };
  node.children.forEach(visit);
  return text.replace(/\s+/g, " ").trim();
}

function violations(fileName: string, source: string): string[] {
  const file = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const found: string[] = [];
  const where = (node: ts.Node) => `${fileName}:${file.getLineAndCharacterOfPosition(node.getStart(file)).line + 1}`;

  const visit = (node: ts.Node, hiddenAncestor: boolean) => {
    let hidden = hiddenAncestor;
    if (ts.isJsxElement(node) || ts.isJsxSelfClosingElement(node)) {
      hidden = hidden || isAriaHidden(node);
      const tokens = classTokens(node);
      const text = inheritedText(node);

      const px = smallestPx(tokens);
      if (px !== null && px < MIN_TEXT_PX) {
        const allowed = hidden && px >= MIN_DECORATIVE_PX && !CJK.test(text);
        if (!allowed) found.push(`${where(node)} 字号 ${px}px「${text.slice(0, 30)}」`);
      }

      const em = tracking(tokens);
      if (em !== null && em > 0 && CJK.test(text)) {
        found.push(`${where(node)} 中文字距 ${em}em「${text.slice(0, 30)}」`);
      }
    } else if (ts.isStringLiteralLike(node) && /(^|\s)text-\[(\d|1[01])(\.\d+)?px\]/.test(node.text)) {
      // 不在 JSX className 里的小字号（如 cva 的尺寸表）没法确认是不是读屏隐藏的装饰。
      const inClassName = ts.findAncestor(node, (ancestor) => ts.isJsxAttribute(ancestor));
      if (!inClassName) found.push(`${where(node)} 字号类不在 JSX className 里「${node.text.slice(0, 40)}」`);
    }
    ts.forEachChild(node, (child) => visit(child, hidden));
  };
  visit(file, false);
  return found;
}

describe("typography", () => {
  it("reads class names through cn() and nested spans, and honours aria-hidden and resets", () => {
    const probe = `
      export function Probe({ code }: { code: string }) {
        return (
          <>
            <p className={cn("font-mono text-[10px]", active && "tracking-[0.3em]")}>学 校 邮 箱</p>
            <p className="font-mono text-xs tracking-[0.3em]">SCROLL / <span>向下滚动</span></p>
            <p className="font-mono text-xs tracking-widest">L-01 <span className="tracking-normal">书库</span></p>
            <p className="font-mono text-xs"><span className="tracking-widest">L-02</span> 书库</p>
            <div aria-hidden><span className="text-[10px] tracking-[0.3em]">{code}</span></div>
            <span aria-hidden="true" className="text-[10px]">装饰</span>
            <span aria-hidden className="text-[9px]">FIG</span>
            <input className="tracking-[0.35em] placeholder:tracking-normal" placeholder="6 位数字" />
          </>
        );
      }
      const sizes = cva("text-sm", { variants: { size: { sm: "text-[11px]" } } });
    `;
    const found = violations("probe.tsx", probe).map((line) => line.replace(/^probe\.tsx:\d+ /, ""));

    expect(found).toEqual([
      "字号 10px「学 校 邮 箱」",
      "中文字距 0.3em「学 校 邮 箱」",
      "中文字距 0.3em「SCROLL / 向下滚动」",
      "字号 10px「装饰」",
      "字号 9px「FIG」",
      "字号类不在 JSX className 里「text-[11px]」",
    ]);
  });

  it("keeps text at 12px or more and Chinese untracked across Portal", () => {
    const found = sourceFiles(SRC).flatMap((file) =>
      violations(path.relative(SRC, file), readFileSync(file, "utf8"))
    );

    expect(found).toEqual([]);
  });
});
