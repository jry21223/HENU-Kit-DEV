"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { parentRoute } from "@/lib/navigation/parent-route";

/**
 * 左上角的「返回上一级」。落点由路径层级决定（刷题 → 题库，题库 → 平台首页），
 * 而不是永远回平台首页。落点页面的浏览位置由 ScrollMemory 恢复。
 */
export default function BackLink({ className }: { className?: string }) {
  const pathname = usePathname();
  const parent = parentRoute(pathname);

  return (
    <Link href={parent.href} className={className} data-back-link>
      ← {parent.label}
    </Link>
  );
}
