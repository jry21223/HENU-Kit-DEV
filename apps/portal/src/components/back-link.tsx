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
 * 默认样式就是子站导航里那一行。传进来的 className 会与它**合并**（tailwind-merge）而不是
 * 替换，所以页面正文里的回退入口只要补自己的间距/边框，导航那一行的观感不会丢。
 */
const NAV_LINK_CLASS =
  "font-mono text-xs tracking-widest text-ink/60 transition-colors hover:text-accent";
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
