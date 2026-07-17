import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import * as esbuild from "esbuild";

const outdir = join(tmpdir(), "quizcraft-code-syntax-test");
mkdirSync(outdir, { recursive: true });

const outfile = join(outdir, `CodeSyntaxScope-${Date.now()}.mjs`);
await esbuild.build({
  entryPoints: ["src/components/codeSyntaxRenderer.ts"],
  outfile,
  bundle: true,
  platform: "node",
  format: "esm",
  jsx: "automatic",
  logLevel: "silent",
});

const module = await import(pathToFileURL(outfile).href);
const parseRichText = module.parseRichText;
const highlightCodeParts = module.highlightCodeParts;
const getLanguageLabel = module.getLanguageLabel;

assert.equal(typeof parseRichText, "function", "parseRichText must be exported");
assert.equal(typeof highlightCodeParts, "function", "highlightCodeParts must be exported");
assert.equal(typeof getLanguageLabel, "function", "getLanguageLabel must be exported");

const escapeHtml = (value) =>
  value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");

const renderCodeParts = (parts) =>
  parts
    .map((part) => {
      if (part.kind === "text") return escapeHtml(part.value);
      return `<span class="qc-code-token qc-code-token--${part.kind}">${escapeHtml(part.value)}</span>`;
    })
    .join("");

const renderSegment = (segment) => {
  if (segment.kind === "text") {
    return escapeHtml(segment.value).replace(/\r?\n/g, "<br />");
  }

  const highlighted = renderCodeParts(highlightCodeParts(segment.value, segment.language));
  const languageLabel = getLanguageLabel(segment.language);
  if (!segment.block) {
    return `<span class="qc-inline-code">${highlighted}</span>`;
  }

  return `<span class="qc-code-block" data-language="${languageLabel}" data-swipe-ignore="true"><code>${highlighted}</code></span>`;
};

const renderRichText = (text) => parseRichText(text).map(renderSegment).join("");

const javaBlock = renderRichText("阅读代码：\n```java\nint a = 1;\nSystem.out.println(a);\n```");
assert.match(javaBlock, /qc-code-block/);
assert.match(javaBlock, /data-language="JAVA"/);
assert.match(javaBlock, /data-swipe-ignore="true"/);
assert.match(javaBlock, /System\.out/);

const backtickInline = renderRichText('已知 `String str = "Java";` 的结果');
assert.match(backtickInline, /qc-inline-code/);
assert.match(backtickInline, /String str/);

const nakedHtml = renderRichText("在HTML5中，<video>标签用于嵌入视频，<audio>用于音频。");
assert.match(nakedHtml, /qc-inline-code/, "naked HTML tags should render as inline code");
assert.match(nakedHtml, /qc-code-token--tag">video/);
assert.match(nakedHtml, /qc-code-token--tag">audio/);

const plainText = renderRichText("普通题干不应该被错误解析。");
assert.equal(plainText, "普通题干不应该被错误解析。");

const snakeCaseText = parseRichText("foo_bar_baz should stay text");
assert.deepEqual(
  snakeCaseText,
  [{ kind: "text", value: "foo_bar_baz should stay text" }],
  "snake_case identifiers should not render as italic markdown",
);

const routerPrompt = renderRichText("<MSR>display ip routing-table 6.6.6.6");
assert.doesNotMatch(routerPrompt, /qc-inline-code/, "router prompts should not be treated as HTML tags");

writeFileSync(join(outdir, "last-output.html"), [javaBlock, backtickInline, nakedHtml, plainText].join("\n"));

const quizSource = readFileSync("src/pages/Quiz.tsx", "utf8");
assert.match(
  quizSource,
  /grid-cols-\[44px_minmax\(0,1fr\)/,
  "option cards must use a fixed label column and minmax(0,1fr) content column",
);
assert.match(
  quizSource,
  /min-w-0 max-w-full/,
  "option content must allow long code text to shrink inside the card",
);
assert.match(
  quizSource,
  /\[overflow-wrap:anywhere\]/,
  "option content must allow long code strings to wrap anywhere",
);

const cssSource = readFileSync("src/index.css", "utf8");
const getCssRuleBody = (source, selector) => {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = source.match(new RegExp(`${escapedSelector}\\s*\\{([\\s\\S]*?)\\}`));
  assert.ok(match, `${selector} rule must exist`);
  return match[1];
};
const inlineCodeRule = getCssRuleBody(cssSource, ".qc-inline-code");
const codeBlockRule = getCssRuleBody(cssSource, ".qc-code-block");
const codeBlockCodeRule = getCssRuleBody(cssSource, ".qc-code-block code");

assert.match(
  codeBlockRule,
  /display:\s*flex;/,
  "code block containers must use flex layout for stable alignment",
);
assert.match(
  codeBlockRule,
  /flex-direction:\s*column;/,
  "code block containers must keep the language label above code content",
);
assert.match(
  codeBlockRule,
  /justify-content:\s*center;/,
  "code block containers must center their content along the main axis",
);

assert.match(
  inlineCodeRule,
  /white-space:\s*pre-wrap;/,
  "inline code must wrap instead of forcing a single long line",
);
assert.doesNotMatch(
  codeBlockCodeRule,
  /min-width:\s*max-content;/,
  "code blocks inside rich text must not force max-content width",
);
assert.match(
  codeBlockCodeRule,
  /overflow-wrap:\s*anywhere;/,
  "code blocks must allow long HTML or JavaScript snippets to wrap",
);
