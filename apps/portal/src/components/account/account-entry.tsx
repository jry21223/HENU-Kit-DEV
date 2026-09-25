"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { cn } from "@/lib/cn";

/**
 * 跨站账号入口：SSR 恒渲染未登录态，水合后自动切换。
 * 未登录 → mono 登录链接；已登录 → 首字方形头像块 + 用户名。
 * `className` 与 `onClick` 落在链接本身上：首页手机菜单用它把账户行做成整行可点。
 * `nameClassName` 落在用户名上：用户名默认按桌面页头截到 80px，整行里用它放开。
 */
export default function AccountEntry({
  compact = false,
  className,
  nameClassName,
  onClick,
}: {
  compact?: boolean;
  className?: string;
  nameClassName?: string;
  onClick?: () => void;
}) {
  const { user } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );

  if (!user) {
    return (
      <Link
        href="/account/login"
        onClick={onClick}
        className={cn("font-mono text-xs tracking-widest text-ink/70 transition-colors hover:text-accent", className)}
      >
        登录<span className="text-ink/30">/</span>注册
      </Link>
    );
  }

  return (
    <Link href="/account" onClick={onClick} className={cn("group flex items-center gap-2", className)}>
      <span className="flex h-7 w-7 items-center justify-center border border-ink bg-paper font-display text-sm font-bold transition-colors group-hover:border-accent group-hover:text-accent">
        {user.name.slice(0, 1)}
      </span>
      {!compact && (
        <span className={cn("max-w-20 truncate font-mono text-xs text-ink/80 group-hover:text-ink", nameClassName)}>
          {user.name}
        </span>
      )}
    </Link>
  );
}
