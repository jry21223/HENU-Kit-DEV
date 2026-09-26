"use client"; // 错误边界必须是客户端组件

import ErrorFallback from "@/components/error-fallback";

/**
 * 根布局以下任何一层渲染出错时的兜底：替换掉出错的那一层（连同子站布局），根布局照常。
 * 根布局本身出错时走 global-error.tsx。
 */
export default function RouteError({
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  return <ErrorFallback retry={retry} />;
}
