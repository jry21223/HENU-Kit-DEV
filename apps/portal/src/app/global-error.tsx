"use client"; // 错误边界必须是客户端组件

import ErrorFallback from "@/components/error-fallback";
import { documentTitle } from "@/lib/seo";
import { fontVariables } from "./fonts";
import "./globals.css";

/**
 * 根布局本身出错时的兜底。它替换根布局，所以 <html>、全局样式和字体都要自己带；
 * 客户端组件不能导出 metadata，标题用 React 的 <title>。
 */
export default function GlobalError({
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  return (
    <html lang="zh-CN" className={`${fontVariables} h-full antialiased`}>
      <body className="min-h-full">
        <title>{documentTitle("页面出错了")}</title>
        <ErrorFallback retry={retry} />
      </body>
    </html>
  );
}
