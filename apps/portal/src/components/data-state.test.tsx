import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { EmptyBlock, ErrorBanner, LoadingBlock } from "./data-state";

/**
 * 空状态说明为什么没有内容，并可带一个下一步（链接或按钮）；中文不追加英文后缀、
 * 不拉宽字距（#545）。
 */
describe("EmptyBlock", () => {
  it("shows the Chinese label alone, without an English suffix or wide tracking", () => {
    const html = renderToStaticMarkup(<EmptyBlock label="暂无互助或闲置信息" />);

    expect(html).toContain("暂无互助或闲置信息");
    expect(html).not.toContain("EMPTY");
    // 任何加宽字距都不行（tracking-[…]、tracking-wide/wider/widest），只允许 tracking-normal。
    expect(html).not.toMatch(/tracking-(?!normal\b)/);
    expect(html).not.toContain("<a");
    expect(html).not.toContain("<button");
  });

  it("offers a link as the next step", () => {
    const html = renderToStaticMarkup(
      <EmptyBlock label="还没有收藏任何题目" action={{ label: "去题库", href: "/practice" }} />
    );

    expect(html).toMatch(/<a[^>]*href="\/practice"[^>]*>去题库<\/a>/);
    expect(html).not.toContain("<button");
  });

  it("offers a button as the next step", () => {
    const html = renderToStaticMarkup(
      <EmptyBlock label="无匹配单子" action={{ label: "清除筛选", onClick: () => {} }} />
    );

    expect(html).toMatch(/<button[^>]*type="button"[^>]*>清除筛选<\/button>/);
    expect(html).not.toContain("<a");
  });
});

describe("LoadingBlock", () => {
  it("marks the Chinese label as in progress with an ellipsis instead of an English suffix", () => {
    const html = renderToStaticMarkup(<LoadingBlock label="加载互助单" />);

    expect(html).toContain("加载互助单…");
    expect(html).not.toContain("LOADING");
    // 任何加宽字距都不行（tracking-[…]、tracking-wide/wider/widest），只允许 tracking-normal。
    expect(html).not.toMatch(/tracking-(?!normal\b)/);
  });
});

/**
 * 加载失败横幅只展示一条主信息，外加「重试」；有请求编号时显示为「错误编号」，
 * 方便用户提工单时附上（#533）。
 */
describe("ErrorBanner", () => {
  it("shows one message and a retry, without a fixed headline or a second sentence", () => {
    const html = renderToStaticMarkup(
      <ErrorBanner message="资料库暂时无法加载，请稍后重试。" onRetry={() => {}} />
    );

    expect(html).toMatch(/^<div[^>]*role="alert"/);
    expect(html.match(/<p[^>]*>/g)).toHaveLength(1);
    expect(html).toContain("资料库暂时无法加载，请稍后重试。");
    expect(html).toMatch(/<button[^>]*type="button"[^>]*>重试<\/button>/);
    expect(html).not.toContain("ERROR");
    expect(html).not.toContain("数据源不可用");
    expect(html).not.toContain("服务暂时不可用，请稍后再来。");
    expect(html).not.toContain("错误编号");
  });

  it("shows the request id as an error number the user can quote", () => {
    const html = renderToStaticMarkup(
      <ErrorBanner message="服务暂时不可用，请稍后再试。" requestId="req_edge404" onRetry={() => {}} />
    );

    expect(html).toContain("错误编号");
    expect(html).toContain("req_edge404");
  });

  it("offers no retry when the caller cannot retry", () => {
    const html = renderToStaticMarkup(<ErrorBanner message="服务暂时不可用，请稍后再试。" />);

    expect(html).not.toContain("<button");
  });
});
