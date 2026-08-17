"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { AccountConsoleSessionProvider } from "@/components/account/account-console-session";
import { cn } from "@/lib/cn";
import {
  clearCachedSession,
  fetchSession,
  formatPortalError,
  logout,
  PortalUnauthorizedError,
} from "@/lib/api/client";
import type { PortalSession } from "@/lib/api/types";
import { publicDisplayName } from "@/lib/auth/display-name";

const MENU = [
  { href: "/account", index: "A-01", label: "概览", exact: true },
  { href: "/account/security", index: "A-02", label: "安全设置" },
  { href: "/account/wallet", index: "A-03", label: "积分钱包" },
  { href: "/account/membership", index: "A-04", label: "会员权益" },
  { href: "/account/tickets", index: "A-05", label: "工单" },
  { href: "/account/notifications", index: "A-06", label: "系统通知" },
  { href: "/account/posts", index: "A-07", label: "我的投稿" },
  { href: "/account/profile", index: "A-08", label: "求职画像" },
];

type SessionState =
  | { kind: "loading" }
  | { kind: "authenticated"; session: PortalSession }
  | { kind: "anonymous" }
  | { kind: "error"; message: string };

function LoadingBlock() {
  return (
    <div data-account-session-state="loading" className="flex min-h-[60vh] items-center justify-center">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
        AUTH CHECK<span className="animate-pulse text-accent">…</span>
      </p>
    </div>
  );
}

export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [sessionState, setSessionState] = useState<SessionState>({ kind: "loading" });
  const [signingOut, setSigningOut] = useState(false);
  const [logoutError, setLogoutError] = useState("");
  const requestVersion = useRef(0);

  const requestSession = useCallback((version: number) => {
    void fetchSession().then(
      (session) => {
        if (version !== requestVersion.current) return;
        setSessionState(session ? { kind: "authenticated", session } : { kind: "anonymous" });
      },
      (error: unknown) => {
        if (version === requestVersion.current) {
          setSessionState({ kind: "error", message: formatPortalError(error) });
        }
      }
    );
  }, []);

  const loadSession = useCallback(() => {
    const version = ++requestVersion.current;
    setSessionState({ kind: "loading" });
    requestSession(version);
  }, [requestSession]);

  const redirectToAccountLogin = useCallback(() => {
    router.replace(`/account/login?next=${encodeURIComponent(pathname)}`);
  }, [pathname, router]);

  const requireLogin = useCallback(() => {
    requestVersion.current += 1;
    setSessionState({ kind: "anonymous" });
    // The cached user must go too, or the login page bounces straight back
    // into the account console (the #412 loop).
    clearCachedSession();
    redirectToAccountLogin();
  }, [redirectToAccountLogin]);

  useEffect(() => {
    const version = ++requestVersion.current;
    requestSession(version);
    return () => {
      requestVersion.current += 1;
    };
  }, [requestSession]);

  // Anonymous is a Gateway fact, never a missing local mock. Keep the return
  // path so the OAuth entry can resume the account route after sign-in.
  useEffect(() => {
    if (sessionState.kind === "anonymous") {
      redirectToAccountLogin();
    }
  }, [redirectToAccountLogin, sessionState.kind]);

  const signOut = async () => {
    setSigningOut(true);
    setLogoutError("");
    try {
      // Clears the Gateway session (including the Core session revocation)
      // and the cached local user, so the login page stays put.
      await logout();
      clearCachedSession();
      router.replace("/account/login");
    } catch (error) {
      if (error instanceof PortalUnauthorizedError) {
        requireLogin();
        return;
      }
      setLogoutError(formatPortalError(error));
      setSigningOut(false);
    }
  };

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
          {sessionState.kind === "authenticated" ? (
            <Link
              href="/account"
              aria-label={`${publicDisplayName(sessionState.session.display_name)}的账户概览`}
              className="group flex min-h-11 min-w-11 items-center justify-center border border-ink bg-paper font-display text-sm font-bold transition-colors hover:border-accent hover:text-accent"
            >
              {publicDisplayName(sessionState.session.display_name).slice(0, 1)}
            </Link>
          ) : (
            <Link
              href={`/account/login?next=${encodeURIComponent(pathname)}`}
              className="inline-flex min-h-11 items-center font-mono text-xs tracking-widest text-ink/70 transition-colors hover:text-accent"
            >
              登录<span className="text-ink/30">/</span>注册
            </Link>
          )}
        </div>
      </header>

      {sessionState.kind === "loading" || sessionState.kind === "anonymous" ? <LoadingBlock /> : null}

      {sessionState.kind === "error" ? (
        <section data-account-session-state="error" role="alert" className="mx-auto mt-10 max-w-2xl border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">账户服务暂不可用</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{sessionState.message}</p>
          <p className="mt-3 text-sm leading-6 text-ink/60">账户信息暂时加载不出来，请稍后重新加载。</p>
          <button
            type="button"
            onClick={loadSession}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {sessionState.kind === "authenticated" ? (
        <AccountConsoleSessionProvider session={sessionState.session} requireLogin={requireLogin}>
          <div className="mx-auto max-w-[1440px] lg:flex">
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
                        "inline-flex min-h-11 shrink-0 items-center border-l-2 px-3 py-2 font-mono text-xs tracking-widest transition-colors lg:py-2.5",
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
                  disabled={signingOut}
                  onClick={() => void signOut()}
                  className="mt-0 inline-flex min-h-11 shrink-0 items-center border-l-2 border-transparent px-3 py-2 text-left font-mono text-xs tracking-widest text-ink/55 transition-colors hover:text-accent disabled:cursor-wait disabled:opacity-50 lg:mt-8 lg:py-2.5"
                >
                  <span className="mr-1.5 text-ink/30">A-00</span>
                  {signingOut ? "正在退出…" : "退出登录"}
                </button>
                {logoutError ? <p role="alert" className="mt-3 px-3 text-xs leading-5 text-accent">{logoutError}</p> : null}
              </nav>
            </aside>

            <div className="min-w-0 flex-1 px-5 py-10 md:px-8">{children}</div>
          </div>
        </AccountConsoleSessionProvider>
      ) : null}
    </div>
  );
}
