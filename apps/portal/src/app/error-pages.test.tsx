import { Children, isValidElement, type ReactElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import ErrorFallback from "@/components/error-fallback";
import RouteError from "./error";
import GlobalError from "./global-error";

// next/font 只在 Next 的编译里可用；这里只关心字体变量挂到了 <html> 上。
vi.mock("./fonts", () => ({ fontVariables: "font-vars" }));

/**
 * 出错兜底页只说明出错了和能做什么（#534）：中文说明、「重试」与「回首页」，带全站页脚；
 * 错误原文、digest 和堆栈可能带内部细节，一律不上屏。
 */

const leakyError = Object.assign(
  new Error("upstream 502 from portal-gateway at /api/v2/internal"),
  { digest: "digest-7f3a91" }
);

function expectBrandedErrorPage(html: string) {
  expect(html).toMatch(/<h1[^>]*>页面出错了<\/h1>/);
  expect(html).toContain("这个页面没能正常显示");
  expect(html).toMatch(/<button[^>]*type="button"[^>]*>重试<\/button>/);
  expect(html).toMatch(/<a[^>]*href="\/"[^>]*>回首页<\/a>/);
  expect(html).toMatch(/<footer[^>]*>[\s\S]*学生自主运营 · 非河南大学官方项目[\s\S]*<\/footer>/);

  expect(html).not.toContain("upstream");
  expect(html).not.toContain("portal-gateway");
  expect(html).not.toContain(leakyError.digest);
  expect(html).not.toMatch(/at \S+ \(/);
}

describe("route error page", () => {
  it("explains the failure and offers retry and home without leaking the error", () => {
    expectBrandedErrorPage(renderToStaticMarkup(<RouteError error={leakyError} retry={() => {}} />));
  });
});

describe("global error page", () => {
  it("renders its own zh-CN document with the site fonts, a title and the same fallback", () => {
    const html = renderToStaticMarkup(<GlobalError error={leakyError} retry={() => {}} />);
    expect(html).toMatch(/^<html[^>]*lang="zh-CN"/);
    expect(html).toMatch(/^<html[^>]*class="[^"]*font-vars/);
    expect(html).toContain("<title>页面出错了 | HENU Kit</title>");
    expect(html).toMatch(/<body[^>]*>/);
    expectBrandedErrorPage(html);
  });
});

type AnyElement = ReactElement<Record<string, unknown>>;

/** 不渲染组件，只沿 children 找第一个符合条件的元素（没有 DOM 环境也能拿到 onClick）。 */
function findElement(node: ReactNode, match: (element: AnyElement) => boolean): AnyElement | undefined {
  if (!isValidElement<Record<string, unknown>>(node)) return undefined;
  if (match(node)) return node;
  for (const child of Children.toArray(node.props.children as ReactNode)) {
    const found = findElement(child, match);
    if (found) return found;
  }
  return undefined;
}

// 「重试」得真的接到 Next 传进来的 retry 上：这个 prop 在 16.2/16.3 间改过名，只看按钮在不在测不出接错。
describe("retry wiring", () => {
  it("the 重试 button calls the retry it is given", () => {
    const retry = vi.fn();
    const button = findElement(ErrorFallback({ retry }), (element) => element.props.children === "重试");
    expect(button?.props.onClick).toBeTypeOf("function");
    (button!.props.onClick as () => void)();
    expect(retry).toHaveBeenCalledOnce();
  });

  it.each([
    ["route", RouteError],
    ["global", GlobalError],
  ])("the %s error page hands Next's retry to the fallback", (_, Page) => {
    const retry = vi.fn();
    const fallback = findElement(Page({ error: leakyError, retry }), (element) => element.type === ErrorFallback);
    expect(fallback?.props.retry).toBe(retry);
  });
});
