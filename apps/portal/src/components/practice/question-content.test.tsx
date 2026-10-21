import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import QuestionContent from "./question-content";

describe("Practice question content", () => {
  it("displays a reviewed source figure and preserves multiline commands", () => {
    const path = `/practice-assets/h3c/${"a".repeat(64)}.png`;
    const html = renderToStaticMarkup(
      <QuestionContent content={`根据题图判断路径。\n![网络拓扑](${path})\ninterface GE1/0\n ip address 10.1.1.1 24`} />
    );

    expect(html).toContain(`src="${path}"`);
    expect(html).toContain('alt="网络拓扑"');
    expect(html).toContain('aria-label="打开网络拓扑原图"');
    expect(html).toContain("whitespace-pre-wrap");
    expect(html).toContain("interface GE1/0\n ip address 10.1.1.1 24");
    expect(html).not.toContain("![网络拓扑]");
  });

  it("renders HTML and untrusted image locations as text without network requests", () => {
    const html = renderToStaticMarkup(
      <QuestionContent content={'<script>alert(1)</script>\n![图](https://outside.example/a.png)\n![图](/practice-assets/h3c/../secret.png)'} />
    );

    expect(html).not.toContain("<img");
    expect(html).not.toContain("<script>");
    expect(html).toContain("&lt;script&gt;");
    expect(html).toContain("https://outside.example/a.png");
  });

  it("preserves paragraph breaks and command indentation around figure rows", () => {
    const path = `/practice-assets/h3c/${"a".repeat(64)}.png`;
    const before = "检查配置。\n\n[MSR] interface GE1/0\n ip address 10.1.1.1 24";
    const after = "  [MSR] ospf 1\n area 0\n  network 10.1.1.0 0.0.0.255\n\n请选择正确描述。";
    const html = renderToStaticMarkup(
      <QuestionContent content={`${before}\n\n![配置](${path})\n\n${after}\n\n![拓扑](${path})`} />
    );

    expect(html).toContain(before);
    expect(html).toContain(after);
  });

  it("does not nest an interactive image link inside an answer button", () => {
    const path = `/practice-assets/h3c/${"b".repeat(64)}.png`;
    const html = renderToStaticMarkup(
      <button><QuestionContent content={`![配置](${path})`} openImages={false} /></button>
    );

    expect(html).toContain("<img");
    expect(html).not.toContain("<a ");
  });
});
