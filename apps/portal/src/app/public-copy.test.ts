import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

/**
 * 用户可见文案不写实现细节、内部服务名或没有依据的说法（#532）。
 * 用户只需要知道「解析」「答对 / 答错」，不需要知道结果来自哪个服务。
 */
const INTERNAL_TERMS = [
  "服务端",
  "ACCOUNT PORTFOLIO",
  "REAL DATA",
  "FACTS",
  "接口尚未接通",
  "同学也在看",
  // 同一类内部措辞的其他写法、面向评审的承诺与求职雷达的实现用语
  "尚未接通",
  "练习服务",
  "作答事实",
  "排行事实",
  "示例图表",
  "异步",
  "受控",
  "持久化",
  "原文件字节",
  "PERSISTED",
  "IMMUTABLE",
  "LEDGER",
  "Redis",
  // 面向评审的承诺与暗示存在「假数据」的说法
  "以本地",
  "真实题库",
  "真实章节",
  "真实排行榜",
  "真实课程",
  // 页面并不提供的功能
  "逐级定位",
  "按专业",
];

/** #532 点名的页面：改名或挪位置时这里要跟着改，免得检查悄悄漏掉它们。 */
const CITED_FILES = [
  "components/practice/bank-hero.tsx",
  "app/practice/quiz/page.tsx",
  "app/practice/leaderboard/page.tsx",
  "app/practice/stats/page.tsx",
  "app/account/(console)/page.tsx",
  "app/account/(console)/wallet/page.tsx",
  "app/campus/publish/page.tsx",
  "components/career/career-free-view.tsx",
  "components/library/item-detail.tsx",
];

/**
 * 承载信息的英文改用中文（#573），资料类型沿用资料库的叫法（#572）。
 * 纯装饰的英文（FIG 编号、KEEP IN TOUCH、SCHEMATIC、T-01 / MUST EAT 这类标签）不在此列。
 */
const ENGLISH_INFORMATION = [
  "ENTRIES",
  "FILES INDEXED",
  "SYNCING",
  "LOADING…",
  "MEMBERSHIP LOADING",
  "TICKETS LOADING",
  "NOTIFICATIONS LOADING",
  "WORK RADAR LOADING",
  "SCAN HISTORY LOADING",
  "FOOD POSTS LOADING",
  "CAREER PROFILE LOADING",
  "TICKET DETAIL LOADING",
  "STANDBY",
  "SCANNING",
  "JOBS FOUND",
  "往年试卷",
  "学长笔记",
  "HANDOUT",
  "COURSEWARE",
  "EXERCISE",
  "TEXTBOOK",
];

const SRC = path.resolve(__dirname, "..");

/** 直接对外提供的纯文本文件，整份都是可见文案。 */
const PUBLIC_TEXT_FILES = [path.resolve(SRC, "../public/llms.txt")];

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(full);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [full] : [];
  });
}

/**
 * 源码里会被渲染出来的文字：JSX 文本、字符串和模板字面量。
 * 走语法树而不是正则，注释和标识符天然不在其中。
 */
function renderableStrings(fileName: string, source: string) {
  const file = ts.createSourceFile(
    fileName,
    source,
    ts.ScriptTarget.Latest,
    true,
    fileName.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS
  );
  const strings: Array<{ text: string; line: number }> = [];
  const visit = (node: ts.Node) => {
    if (ts.isJsxText(node) || ts.isStringLiteral(node) || ts.isTemplateLiteralToken(node)) {
      const { line } = file.getLineAndCharacterOfPosition(node.getStart(file));
      strings.push({ text: node.text, line: line + 1 });
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  return strings;
}

describe("user-visible copy", () => {
  it("reads JSX text, attributes and literals but skips comments and identifiers", () => {
    const probe = `
      // 服务端注释
      const REAL_FACTS = "字符串文案";
      export function Probe() {
        /* 服务端 */
        return (
          <p title="属性文案">
            {/* 服务端 */}
            正文文案{\`模板\${REAL_FACTS}尾巴\`}
          </p>
        );
      }
    `;
    const texts = renderableStrings("probe.tsx", probe).map(({ text }) => text.trim());

    expect(texts).toEqual(
      expect.arrayContaining(["字符串文案", "属性文案", "正文文案", "模板", "尾巴"])
    );
    expect(texts.join("\n")).not.toMatch(/服务端|FACTS/);
  });

  it("never names internal services, APIs or claims the product cannot back", () => {
    const files = sourceFiles(SRC);
    expect(files.map((file) => path.relative(SRC, file))).toEqual(
      expect.arrayContaining(CITED_FILES.map((file) => path.normalize(file)))
    );

    const hits = files.flatMap((file) =>
      renderableStrings(file, readFileSync(file, "utf8")).flatMap(({ text, line }) =>
        INTERNAL_TERMS.filter((term) => text.includes(term)).map(
          (term) => `${path.relative(SRC, file)}:${line} ${term}`
        )
      )
    );

    expect(hits).toEqual([]);
  });

  it("states counts, status and material types in Chinese", () => {
    const hits = sourceFiles(SRC).flatMap((file) =>
      renderableStrings(file, readFileSync(file, "utf8")).flatMap(({ text, line }) =>
        ENGLISH_INFORMATION.filter((term) => text.includes(term)).map(
          (term) => `${path.relative(SRC, file)}:${line} ${term}`
        )
      )
    );

    expect(hits).toEqual([]);
  });

  it("keeps the same wording out of the public plain-text files", () => {
    const hits = PUBLIC_TEXT_FILES.flatMap((file) =>
      readFileSync(file, "utf8")
        .split("\n")
        .flatMap((text, index) =>
          INTERNAL_TERMS.filter((term) => text.includes(term)).map(
            (term) => `${path.relative(path.dirname(SRC), file)}:${index + 1} ${term}`
          )
        )
    );

    expect(hits).toEqual([]);
  });
});
