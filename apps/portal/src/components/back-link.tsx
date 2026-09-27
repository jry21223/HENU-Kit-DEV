"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { parentRoute } from "@/lib/navigation/parent-route";
import { cn } from "@/lib/cn";

/**
 * 左上角的「返回上一级」。落点由路径层级决定（刷题 → 题库，题库 → 平台首页），
 * 而不是永远回平台首页。落点页面的浏览位置由 ScrollMemory 恢复。
 *
 * `data-back-link` 是浏览器验收用来定位这个控件的钩子（与 `data-food-tier` 同类）；
 * 可见文案只有一个箭头加层级名，所以另给可访问名说明「回到哪一层」。
 *
 * 默认样式就是子站导航里那一行，点击区撑到 44px 高（DESIGN_SYSTEM §13）。传进来的 className
 * 会与它**合并**（tailwind-merge）而不是替换，所以页面正文里的回退入口只要补自己的间距/边框，
 * 导航那一行的观感和点击区都不会丢；不要再传 `inline-block` 之类的 display，它会顶掉 inline-flex。
 */
const NAV_LINK_CLASS =
  "inline-flex min-h-11 items-center font-mono text-xs text-ink/60 transition-colors hover:text-accent-text";
export default function BackLink({ className }: { className?: string }) {
  const pathname = usePathname();
  const parent = parentRoute(pathname);

  return (
    <Link
      href={parent.href}
      className={cn(NAV_LINK_CLASS, className)}
      data-back-link
      aria-label={`返回上一级：${parent.spokenAs ?? parent.label}`}
    >
      ← {parent.label}
    </Link>
  );
}

/**
 * 详情页整页状态（加载中、不存在、暂时读不到）正文里的回退入口：橙色等宽字，点击区撑到
 * 44 × 44（DESIGN_SYSTEM §13）。点击区比文字高出的部分上下各一半，所以上外边距用 mt-3，
 * 文字仍在原来 mt-6 的位置。
 */
export function DetailStateBackLink() {
  return (
    <BackLink className="mt-3 inline-flex min-h-11 min-w-11 items-center font-mono text-sm text-accent-text hover:underline" />
  );
}
