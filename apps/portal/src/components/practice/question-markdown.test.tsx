import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import QuestionMarkdown from "./question-markdown";

describe("question Markdown", () => {
  it("renders Java fences with indentation and generic type characters intact", () => {
    const html = renderToStaticMarkup(<QuestionMarkdown content={'阅读代码：\n\n```java\nList<String> list = new ArrayList<>();\n  list.add("aaa");\n```'} />);
    expect(html).toContain("<pre");
    expect(html).toContain("language-java");
    expect(html).toContain("  list.");
    expect(html).toContain("&lt;");
    expect(html).not.toContain("```");
  });

  it("renders tables and inline code while keeping raw HTML and unsafe URLs inert", () => {
    const html = renderToStaticMarkup(<QuestionMarkdown content={'使用 `List<String>`\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n<script>alert(1)</script>\n\n[危险](javascript:alert(1))'} />);
    expect(html).toContain("<table>");
    expect(html).toContain("<code>List&lt;String&gt;</code>");
    expect(html).not.toContain("<script");
    expect(html).not.toContain('href="javascript:');
  });

  it("keeps option content valid inside a native answer button", () => {
    const html = renderToStaticMarkup(<button><QuestionMarkdown option content={'**选项**\n\n```java\nList<String> value;\n```\n\n[链接](https://example.com)'} /></button>);
    expect(html).toContain("<strong>选项</strong>");
    expect(html.slice("<button>".length, -"</button>".length)).not.toMatch(/<(div|p|pre|a|input|button)[\s>]/g);
  });

  it("preserves rows and cells in a table option", () => {
    const html = renderToStaticMarkup(<QuestionMarkdown option content={'| A | B |\n| - | - |\n| 1 | 2 |'} />);
    expect(html).toContain('role="table"');
    expect(html.match(/role="row"/g)).toHaveLength(2);
    expect(html.match(/role="cell"/g)).toHaveLength(2);
    expect(html).toContain('class="table-cell border border-line p-2">1</span>');
  });

  it("keeps plain Java generic notation visible even when the source omitted code formatting", () => {
    const html = renderToStaticMarkup(<QuestionMarkdown content="List<String> values;" />);
    expect(html).toContain("List&lt;String&gt; values;");
  });

  it("keeps reviewed H3C figures inside an option without an interactive link", () => {
    const path = `/practice-assets/h3c/${"b".repeat(64)}.png`;
    const html = renderToStaticMarkup(<button><QuestionMarkdown option content={`![配置](${path})`} /></button>);
    expect(html).toContain(`src="${path}"`);
    expect(html).toContain('alt="配置"');
    expect(html).not.toContain("<a ");
  });


  it("combines fenced commands with reviewed figures without requesting untrusted images", () => {
    const path = `/practice-assets/h3c/${"a".repeat(64)}.png`;
    const html = renderToStaticMarkup(<QuestionMarkdown content={`配置：\n\n\`\`\`\ninterface GE1/0\n ip address 10.1.1.1 24\n\`\`\`\n\n![网络拓扑](${path})\n\n![外部图](https://outside.example/a.png)\n![越界图](/practice-assets/h3c/../secret.png)`} />);
    expect(html).toContain("<pre");
    expect(html).toContain(" ip address 10.1.1.1 24");
    expect(html).toContain(`src="${path}"`);
    expect(html).toContain('aria-label="打开网络拓扑原图"');
    expect(html.match(/<img /g)).toHaveLength(1);
    expect(html).not.toContain('src="https://');
  });

});
