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
        className="inline-flex min-h-11 min-w-11 items-center font-mono text-xs tracking-widest text-ink/70 transition-colors hover:text-ink"
      >
        登录<span>/</span>注册
      </Link>
    );
  }

  return (
    <Link href="/account" className="group inline-flex min-h-11 min-w-11 items-center gap-2">
      <span className="flex h-7 w-7 items-center justify-center border border-ink bg-paper font-display text-sm font-bold transition-colors group-hover:border-accent group-hover:text-ink">
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
