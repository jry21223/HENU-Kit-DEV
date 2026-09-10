"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { parentRoute } from "@/lib/navigation/parent-route";

/**
 * 左上角的「返回上一级」。落点由路径层级决定（刷题 → 题库，题库 → 平台首页），
 * 而不是永远回平台首页。落点页面的浏览位置由 ScrollMemory 恢复。
 *
 * `data-back-link` 是浏览器验收用来定位这个控件的钩子（与 `data-food-tier` 同类）；
 * 可见文案只有一个箭头加层级名，所以另给可访问名说明「回到哪一层」。
 */
export default function BackLink({ className }: { className?: string }) {
  const pathname = usePathname();
  const parent = parentRoute(pathname);

  return (
    <Link
      href={parent.href}
      className={className}
      data-back-link
      aria-label={`返回上一级：${parent.spokenAs ?? parent.label}`}
    >
      ← {parent.label}
    </Link>
  );
}
