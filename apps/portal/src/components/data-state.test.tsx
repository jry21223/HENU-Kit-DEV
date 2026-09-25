import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ErrorBanner } from "./data-state";

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
