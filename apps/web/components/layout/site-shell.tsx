import Link from "next/link";
import type { ReactNode } from "react";
import { BookOpen, ShieldCheck } from "lucide-react";

export function SiteShell({ children }: { children: ReactNode }) {
  return (
    <main className="min-h-screen px-4 py-4 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex max-w-6xl flex-col gap-6">
        <header className="sticky top-4 z-20 rounded-2xl border border-border/80 bg-card/90 px-4 py-3 shadow-sm backdrop-blur">
          <nav className="flex items-center justify-between gap-4">
            <Link className="flex items-center gap-2 font-semibold tracking-tight" href="/">
              <span className="grid size-8 place-items-center rounded-xl bg-primary text-primary-foreground">
                <BookOpen className="size-4" aria-hidden="true" />
              </span>
              <span>软件学院资料库</span>
            </Link>
            <div className="flex items-center gap-1 text-sm text-muted-foreground sm:gap-2">
              <Link className="rounded-lg px-3 py-2 hover:bg-muted hover:text-foreground" href="/courses">
                课程资料
              </Link>
              <a className="hidden rounded-lg px-3 py-2 hover:bg-muted hover:text-foreground sm:inline-flex" href="/#guarantee">
                资料保障
              </a>
              <Link className="rounded-lg border border-border bg-card px-3 py-2 text-foreground hover:bg-muted" href="/login">
                登录
              </Link>
            </div>
          </nav>
        </header>
        {children}
        <footer className="pb-6 text-center text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <ShieldCheck className="size-3" aria-hidden="true" />
            PDF 稳定供应 · 轻水印 · 持续维护
          </span>
        </footer>
      </div>
    </main>
  );
}
