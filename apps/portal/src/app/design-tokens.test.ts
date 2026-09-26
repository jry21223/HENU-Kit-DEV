import { mkdtempSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { renderTheme } from "../../scripts/generate-theme.mjs";

/**
 * Portal 的颜色来自 packages/design-tokens（#536）。这里检查设计系统承诺的文字配色
 * 达到 WCAG AA（小字 4.5:1，大字 3:1），tokens.css 与 tokens.json 是同一组色值，
 * 以及 Portal 的 Tailwind 主题 theme.css 由 tokens.json 生成、没有落后。
 */

/** 去掉 CSS 注释：注释里的 #537 之类是 issue 编号，举例写的声明也不算数。 */
function withoutComments(css: string) {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

const TOKENS_DIR = path.resolve(__dirname, "../../../../packages/design-tokens");
const TOKENS_CSS = withoutComments(readFileSync(path.join(TOKENS_DIR, "tokens.css"), "utf8"));
const TOKENS_JSON = JSON.parse(readFileSync(path.join(TOKENS_DIR, "tokens.json"), "utf8"));

/** [r, g, b, alpha]，r/g/b 为 0–255，alpha 为 0–1。 */
type Rgba = [number, number, number, number];

/** 解析 #rgb(a) / #rrggbb(aa) 与 rgb() / rgba()；其他写法返回 null。 */
function parseColour(raw: string): Rgba | null {
  const value = raw.trim().toLowerCase();
  const hex = /^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/.exec(value);
  if (hex) {
    const digits = hex[1].length <= 4 ? [...hex[1]].map((digit) => digit + digit).join("") : hex[1];
    const [r, g, b, a = 255] = (digits.match(/../g) ?? []).map((pair) => Number.parseInt(pair, 16));
    return [r, g, b, a / 255];
  }
  const fn = /^rgba?\(([^)]*)\)$/.exec(value);
  if (fn) {
    const [r, g, b, a = "1"] = fn[1].split(/[\s,/]+/).filter(Boolean);
    const alpha = a.endsWith("%") ? Number.parseFloat(a) / 100 : Number.parseFloat(a);
    return [Number.parseFloat(r), Number.parseFloat(g), Number.parseFloat(b), alpha];
  }
  return null;
}

/** 同一颜色的不同写法（大小写、#rgb 简写、rgb() 与十六进制）得到同一个键。 */
function colourKey([r, g, b, a]: Rgba) {
  return `${r},${g},${b},${Math.round(a * 1000) / 1000}`;
}

/** WCAG 2.x 相对亮度。 */
function luminance([r, g, b]: Rgba) {
  const channel = (value: number) => {
    const c = value / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function contrast(a: Rgba, b: Rgba) {
  const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (light + 0.05) / (dark + 0.05);
}

/** 半透明色叠在不透明底色上，按浏览器合成的方式在 sRGB 下混合。 */
function over(top: Rgba, alpha: number, base: Rgba): Rgba {
  return [0, 1, 2].map((i) => top[i] * alpha + base[i] * (1 - alpha)).concat(1) as Rgba;
}

/** tokens.json 里 color.<group>.<name> 的色值。 */
function token(group: string, name: string): Rgba {
  const value = TOKENS_JSON.color?.[group]?.[name]?.$value;
  const colour = typeof value === "string" ? parseColour(value) : null;
  if (!colour) throw new Error(`tokens.json 缺少颜色 color.${group}.${name}`);
  return colour;
}

function jsonColours(node: unknown): string[] {
  if (!node || typeof node !== "object") return [];
  const entry = node as { $type?: unknown; $value?: unknown };
  if (entry.$type === "color" && typeof entry.$value === "string") {
    const colour = parseColour(entry.$value);
    return colour ? [colourKey(colour)] : [];
  }
  return Object.values(node).flatMap(jsonColours);
}

describe("design-tokens 的文字配色（#536）", () => {
  const paper = token("surface", "paper");
  const raised = token("surface", "raised");
  const ink = token("text", "primary");
  const accent = token("brand", "accent");

  // [说明, 文字色, 底色]。强调橙在纸白上连大字的 3:1 都不到，浅色底上的橙字不论大小都用 accent_text。
  const PAIRS: Array<[string, () => Rgba, () => Rgba]> = [
    ["橙字在纸白上", () => token("brand", "accent_text"), () => paper],
    ["橙字在白色卡片上", () => token("brand", "accent_text"), () => raised],
    ["橙字在浅橙底上", () => token("brand", "accent_text"), () => token("brand", "accent_soft")],
    // Portal 的提示框和选中态用 5% 强调橙叠在纸白上（bg-accent/5），比 accent_soft 更深。
    ["橙字在 5% 强调橙叠纸白上", () => token("brand", "accent_text"), () => over(accent, 0.05, paper)],
    // 列表行悬停（hover:bg-ink/5）和压在墨色区块上的导航条（bg-paper/95）都是 5% 墨色叠纸白。
    ["橙字在 5% 墨色叠纸白上", () => token("brand", "accent_text"), () => over(ink, 0.05, paper)],
    ["次要文字在纸白上", () => token("text", "muted"), () => paper],
    ["墨色字在强调橙色块上", () => ink, () => accent],
    ["强调橙字在墨色底上", () => accent, () => ink],
    ["纸白字在墨色底上", () => paper, () => ink],
    // Portal 用墨色透明度写灰字（text-ink/NN），下限 ink/60；叠在 5% 色块上时下限 ink/65。
    ["ink/60 灰字在纸白上", () => over(ink, 0.6, paper), () => paper],
    ["ink/60 灰字在白色卡片上", () => over(ink, 0.6, raised), () => raised],
    ["ink/65 灰字在 5% 墨色叠纸白上", () => over(ink, 0.65, over(ink, 0.05, paper)), () => over(ink, 0.05, paper)],
    ["ink/65 灰字在 5% 强调橙叠纸白上", () => over(ink, 0.65, over(accent, 0.05, paper)), () => over(accent, 0.05, paper)],
    // 墨色底上的纸白字（text-paper/NN）下限 paper/50。
    ["paper/50 纸白字在墨色底上", () => over(paper, 0.5, ink), () => ink],
  ];

  it.each(PAIRS)("%s不低于 4.5:1", (_label, text, background) => {
    expect(contrast(text(), background())).toBeGreaterThanOrEqual(4.5);
  });

  // 大字（≥24px，或 ≥18.66px 粗体）只要 3:1：首页美食榜 36px 粗体的名次用 ink/50。
  const LARGE_PAIRS: Array<[string, () => Rgba, () => Rgba]> = [
    ["ink/50 大字在纸白上", () => over(ink, 0.5, paper), () => paper],
  ];

  it.each(LARGE_PAIRS)("%s不低于 3:1", (_label, text, background) => {
    expect(contrast(text(), background())).toBeGreaterThanOrEqual(3);
  });

  it("tokens.css 与 tokens.json 声明同一组色值", () => {
    const cssColours = [...TOKENS_CSS.matchAll(/--hk-[\w-]+:\s*([^;]+);/g)]
      .map((match) => parseColour(match[1]))
      .filter((colour): colour is Rgba => colour !== null)
      .map(colourKey);
    expect([...new Set(cssColours)].sort()).toEqual([...new Set(jsonColours(TOKENS_JSON))].sort());
  });
});

describe("globals.css 的颜色来自 design-tokens（#536）", () => {
  const globals = withoutComments(readFileSync(path.resolve(__dirname, "globals.css"), "utf8"));
  const theme = readFileSync(path.resolve(__dirname, "theme.css"), "utf8");

  it("引入由 tokens.json 生成的 theme.css，且它没有落后于 tokens.json", () => {
    const imports = [...globals.matchAll(/@import\s+["']([^"']+)["']/g)].map((match) =>
      path.resolve(__dirname, match[1])
    );
    expect(imports).toContain(path.resolve(__dirname, "theme.css"));
    // 改了 tokens.json 却没重新生成时在这里失败：运行 pnpm --filter @henukit/portal generate:theme。
    expect(theme).toBe(renderTheme(TOKENS_JSON));
  });

  it("不再另写 tokens 里已有的色值", () => {
    const tokenColours = new Set(jsonColours(TOKENS_JSON));
    const duplicated = (globals.match(/#[0-9a-f]{3,8}\b|rgba?\([^)]*\)/gi) ?? []).filter((literal) => {
      const colour = parseColour(literal);
      return colour !== null && tokenColours.has(colourKey(colour));
    });
    expect(duplicated).toEqual([]);
  });

  it("不引用 --hk-* 变量", () => {
    // Portal 不引入 tokens.css，这些变量在页面上没有定义；var() 引用它们不报错，只会让颜色悄悄失效。
    expect(globals.match(/var\(--hk-[\w-]+/g) ?? []).toEqual([]);
  });

  it("选中文字是强调橙底墨色字", () => {
    // 与首页跑马灯同理：纸白字在强调橙上只有 2.92:1。axe 不检查 ::selection。
    const selection = /::selection\s*\{([^}]*)\}/.exec(globals)?.[1] ?? "";
    expect(selection).toMatch(/background(-color)?:\s*var\(--color-accent\);/);
    expect(selection).toMatch(/(^|[;\s])color:\s*var\(--color-ink\);/);
  });

  it("橙字有自己的 Tailwind 颜色 accent-text", () => {
    // 没有这条映射时 text-accent-text 不会生成任何样式，也不报错。
    const accentText = TOKENS_JSON.color.brand.accent_text.$value.toLowerCase();
    expect(theme).toContain(`--color-accent-text: ${accentText};`);
  });
});

describe("透明度写法在不支持 color-mix() 的浏览器里有静态回退", () => {
  // 与 Next 构建用同一份 Tailwind 插件；postcss 不是 Portal 的直接依赖，从插件所在位置解析。
  const requireFromPortal = createRequire(__filename);
  const tailwind = requireFromPortal("@tailwindcss/postcss");
  const postcss = createRequire(requireFromPortal.resolve("@tailwindcss/postcss"))("postcss");

  /** 按生产构建的方式（压缩）编译 globals.css，只生成给定的类。 */
  async function compile(classes: string[]) {
    // 自动扫描源码的根目录指向空目录，产物里只有 @source inline 列出的类。
    const emptyBase = mkdtempSync(path.join(os.tmpdir(), "portal-theme-"));
    const input = `@import "${path.resolve(__dirname, "globals.css")}";\n@source inline("${classes.join(" ")}");`;
    const result = await postcss([tailwind({ base: emptyBase, optimize: { minify: true } })]).process(input, {
      from: path.resolve(__dirname, "theme-fallback-check.css"),
    });
    return result.css as string;
  }

  // 不支持 color-mix() 的浏览器只认规则里的第一条声明。它必须是算好的半透明色：
  // 若是 var(--color-*)，bg-accent/5、bg-ink/5 会变成实心的橙或墨，压在上面的同色字看不见。
  const CASES: Array<[string, string]> = [
    ["bg-accent/5", "background-color"],
    ["bg-ink/5", "background-color"],
    ["bg-ink/10", "background-color"],
    ["bg-paper/95", "background-color"],
    ["text-ink/60", "color"],
    ["text-paper/50", "color"],
    ["border-ink/30", "border-color"],
  ];

  it("每个透明度写法的第一条声明都是算好的色值", async () => {
    const css = await compile(CASES.map(([className]) => className));
    const fallbacks = CASES.map(([className, property]) => {
      const selector = `.${className.replace("/", "\\/")}{`.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const declaration = new RegExp(`${selector}${property}:([^;}]*)`).exec(css)?.[1];
      return [className, declaration ?? "未生成"];
    });
    expect(fallbacks.filter(([, value]) => !/^#[0-9a-f]{8}$/i.test(value))).toEqual([]);
  }, 30_000);
});
