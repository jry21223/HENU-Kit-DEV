/**
 * 从 packages/design-tokens/tokens.json 生成 Portal 的 Tailwind 颜色主题 src/app/theme.css（#536）。
 *
 * 主题里写算好的色值，不写 var(--hk-*)：Tailwind 只有拿到字面色值，才能给 bg-accent/5、
 * text-ink/60 这类透明度写法预先算出回退。回退是 var() 时，不支持 color-mix() 的浏览器
 * 会拿到实心的橙或墨，压在上面的同色字看不见。
 *
 * 改色值：改 tokens.json 与 tokens.css，再运行 pnpm --filter @henukit/portal generate:theme。
 * src/app/design-tokens.test.ts 检查 theme.css 与 tokens.json 一致。
 */
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
export const TOKENS_PATH = path.resolve(HERE, "../../../packages/design-tokens/tokens.json");
export const THEME_PATH = path.resolve(HERE, "../src/app/theme.css");

/** Tailwind 颜色名 → tokens.json 里的 color.<group>.<name>。 */
export const COLOR_MAP = [
  ["paper", "surface.paper"],
  ["ink", "text.primary"],
  ["accent", "brand.accent"],
  // 浅色底上的橙字，不论大小（DESIGN_SYSTEM.md 的“文字配色”）。
  ["accent-text", "brand.accent_text"],
  ["line", "surface.line"],
  ["line-dark", "surface.line_dark"],
  // 功能色：只用于难度分徽标等数据标识，不进装饰层。
  ["easy", "semantic.success"],
  ["mid", "semantic.warning"],
  ["hard", "semantic.danger"],
];

export function renderTheme(tokens) {
  const lines = COLOR_MAP.map(([name, tokenPath]) => {
    const [group, key] = tokenPath.split(".");
    const value = tokens.color?.[group]?.[key]?.$value;
    if (typeof value !== "string") throw new Error(`tokens.json 缺少颜色 color.${tokenPath}`);
    return `  --color-${name}: ${value.toLowerCase()};`;
  });
  return [
    "/* 由 scripts/generate-theme.mjs 从 packages/design-tokens/tokens.json 生成，不要手改。",
    "   改色值时改 tokens.json 与 tokens.css，再运行 pnpm --filter @henukit/portal generate:theme。 */",
    "@theme {",
    ...lines,
    "}",
    "",
  ].join("\n");
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  writeFileSync(THEME_PATH, renderTheme(JSON.parse(readFileSync(TOKENS_PATH, "utf8"))));
  console.log(`已生成 ${path.relative(process.cwd(), THEME_PATH)}`);
}
