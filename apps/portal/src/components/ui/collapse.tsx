"use client";

import { useEffect, useState, type ReactNode } from "react";
import { cn } from "@/lib/cn";

/** 展开与收起的时长（ms），与 globals.css 里 enter-* 的缓动同一条曲线。 */
const DURATION = 320;

/**
 * 展开 / 收起的高度过渡：内容按自身高度从 0 撑开、同时淡入，收起时反向，播完再卸载。
 *
 * - 收起状态下内容不在 DOM 里，与原先 `{open && …}` 的语义一致：读屏、Tab 和测试都看不到它；
 * - 用 grid-template-rows 0fr → 1fr 过渡，不必测量高度，内容变高变矮也不跳；
 * - 减少动态效果时直接显示 / 隐藏，没有过渡。
 */
export default function Collapse({
  open,
  children,
  className,
  id,
}: {
  open: boolean;
  children: ReactNode;
  className?: string;
  id?: string;
}) {
  const [mounted, setMounted] = useState(open);
  const [expanded, setExpanded] = useState(open);

  // 打开：先以 0 高度挂上，下一帧再撑开；关闭：立即开始收起。
  if (open && !mounted) setMounted(true);
  if (!open && expanded) setExpanded(false);

  useEffect(() => {
    if (open && mounted && !expanded) {
      let inner = 0;
      const outer = requestAnimationFrame(() => {
        inner = requestAnimationFrame(() => setExpanded(true));
      });
      return () => {
        cancelAnimationFrame(outer);
        cancelAnimationFrame(inner);
      };
    }
    if (!open && mounted) {
      const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      const timer = window.setTimeout(() => setMounted(false), reduced ? 0 : DURATION);
      return () => window.clearTimeout(timer);
    }
  }, [open, mounted, expanded]);

  if (!mounted) return null;

  return (
    <div
      id={id}
      data-collapse={expanded ? "open" : "closed"}
      className={cn(
        "grid transition-[grid-template-rows,opacity] duration-300 ease-[cubic-bezier(0.25,1,0.5,1)] motion-reduce:transition-none",
        expanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
        className
      )}
    >
      <div className="min-h-0 overflow-hidden">{children}</div>
    </div>
  );
}
