"use client";

import Link from "next/link";
import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import AccountEntry from "@/components/account/account-entry";
import { cn } from "@/lib/cn";

const MENU = [
  { href: "/account", index: "A-01", label: "概览", exact: true },
  { href: "/account/security", index: "A-02", label: "安全设置" },
  { href: "/account/tickets", index: "A-05", label: "工单" },
  { href: "/account/notifications", index: "A-06", label: "系统通知" },
];

function LoadingBlock() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
        AUTH CHECK<span className="animate-pulse text-accent">…</span>
      </p>
    </div>
  );
}

export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );
  const router = useRouter();
  const pathname = usePathname();

  // 客户端守卫：未登录重定向登录页（携带回跳路径）
  useEffect(() => {
    if (ready && !user) {
      router.replace(`/account/login?next=${encodeURIComponent(pathname)}`);
    }
  }, [ready, user, pathname, router]);

  const authed = ready && user;

  return (
    <div className="min-h-svh bg-paper text-ink">
      {/* 顶部子导航 */}
      <header className="sticky top-0 z-40 border-b border-line bg-paper">
        <div className="mx-auto flex h-14 max-w-[1440px] items-center justify-between px-5 md:px-8">
          <div className="flex items-baseline gap-4">
            <Link
              href="/"
              className="font-display text-base font-bold tracking-tight text-ink transition-colors hover:text-accent"
            >
              ← henukit<span className="text-accent">®</span>
            </Link>
            <span className="font-display text-base font-bold tracking-tight">
              ACCOUNT<span className="text-accent">®</span>
            </span>
          </div>
          <AccountEntry compact />
        </div>
      </header>

      {authed ? (
        <div className="mx-auto max-w-[1440px] lg:flex">
          {/* 左侧栏（移动端为顶部横排） */}
          <aside className="border-b border-line lg:w-56 lg:shrink-0 lg:border-b-0 lg:border-r">
            <nav className="flex gap-1 overflow-x-auto px-4 py-3 lg:sticky lg:top-14 lg:flex-col lg:gap-0 lg:px-0 lg:py-8">
              {MENU.map((item) => {
                const active = item.exact
                  ? pathname === item.href
                  : pathname.startsWith(item.href);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={cn(
                      "shrink-0 border-l-2 px-3 py-2 font-mono text-xs tracking-widest transition-colors lg:py-2.5",
                      active
                        ? "border-accent font-semibold text-ink"
                        : "border-transparent text-ink/55 hover:text-ink"
                    )}
                  >
                    <span className={cn("mr-1.5", active ? "text-accent" : "text-ink/30")}>
                      {item.index}
                    </span>
                    {item.label}
                  </Link>
                );
              })}
              <button
                type="button"
                onClick={() => {
                  authStore.logout();
                  router.replace("/account/login");
                }}
                className="mt-0 shrink-0 border-l-2 border-transparent px-3 py-2 text-left font-mono text-xs tracking-widest text-ink/55 transition-colors hover:text-accent lg:mt-8 lg:py-2.5"
              >
                <span className="mr-1.5 text-ink/30">A-00</span>
                退出登录
              </button>
            </nav>
          </aside>

          {/* 内容区 */}
          <div className="min-w-0 flex-1 px-5 py-10 md:px-8">{children}</div>
        </div>
      ) : (
        <LoadingBlock />
      )}
    </div>
  );
}
