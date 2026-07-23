"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";

/**
 * 跨站账号入口：SSR 恒渲染未登录态，水合后自动切换。
 * 未登录 → mono 登录链接；已登录 → 首字方形头像块 + 用户名。
 */
export default function AccountEntry({ compact = false }: { compact?: boolean }) {
  const { user } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );

  if (!user) {
    return (
      <Link
        href="/account/login"
        className="font-mono text-xs tracking-widest text-ink/70 transition-colors hover:text-accent"
      >
        登录<span className="text-ink/30">/</span>注册
      </Link>
    );
  }

  return (
    <Link href="/account" className="group flex items-center gap-2">
      <span className="flex h-7 w-7 items-center justify-center border border-ink bg-paper font-display text-sm font-bold transition-colors group-hover:border-accent group-hover:text-accent">
        {user.name.slice(0, 1)}
      </span>
      {!compact && (
        <span className="max-w-20 truncate font-mono text-xs text-ink/80 group-hover:text-ink">
          {user.name}
        </span>
      )}
    </Link>
  );
}
