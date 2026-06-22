import Link from "next/link";
import type { ReactNode } from "react";
import { BookOpen, ShieldCheck } from "lucide-react";

export function SiteShell({ children }: { children: ReactNode }) {
  return (
    <main className="min-h-screen px-3 py-4 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-6xl min-w-0 flex-col gap-6">
        <header className="sticky top-4 z-20 w-full max-w-full rounded-2xl border border-border/80 bg-card/90 px-3 py-3 shadow-sm backdrop-blur sm:px-4">
          <nav className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
            <Link className="flex w-full min-w-0 items-center gap-2 font-semibold tracking-tight sm:w-auto" href="/">
              <span className="grid size-8 flex-none place-items-center rounded-xl bg-primary text-primary-foreground">
                <BookOpen className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0 truncate">软件学院资料库</span>
            </Link>
            <div className="grid w-full min-w-0 grid-cols-4 gap-1.5 text-sm text-muted-foreground sm:flex sm:w-auto sm:items-center sm:justify-end sm:gap-2">
              <Link className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/courses">
                课程资料
              </Link>
              <a className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/#guarantee">
                资料保障
              </a>
              <Link className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/me/downloads">
                我的下载
              </Link>
              <Link className="min-w-0 rounded-lg border border-border bg-card px-1.5 py-2 text-center text-foreground hover:bg-muted sm:px-3" href="/login">
                登录
              </Link>
            </div>
          </nav>
        </header>
        {children}
        <footer className="pb-6 text-center text-xs text-muted-foreground">
          <span className="inline-flex max-w-full flex-wrap items-center justify-center gap-x-1 gap-y-1 break-words">
            <ShieldCheck className="size-3 flex-none" aria-hidden="true" />
            <span>PDF 稳定供应 · 轻水印 · 持续维护</span>
          </span>
        </footer>
      </div>
    </main>
  );
}
