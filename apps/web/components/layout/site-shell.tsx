import Link from "next/link";
import type { ReactNode } from "react";
import { BookOpen, ShieldCheck } from "lucide-react";

export function SiteShell({ children }: { children: ReactNode }) {
  const copy = {
    brand: "\u8f6f\u4ef6\u5b66\u9662\u8d44\u6599\u5e93",
    courses: "\u8bfe\u7a0b\u8d44\u6599",
    forum: "\u8ba8\u8bba",
    guarantee: "\u8d44\u6599\u4fdd\u969c",
    me: "\u4e2a\u4eba\u4e2d\u5fc3",
    login: "\u767b\u5f55",
    footer: "PDF \u7a33\u5b9a\u4f9b\u5e94 / \u8f7b\u6c34\u5370 / \u6301\u7eed\u7ef4\u62a4",
  };

  return (
    <main className="min-h-screen px-3 py-4 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-6xl min-w-0 flex-col gap-6">
        <header className="sticky top-4 z-20 w-full max-w-full rounded-2xl border border-border/80 bg-card/90 px-3 py-3 shadow-sm backdrop-blur sm:px-4">
          <nav className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
            <Link className="flex w-full min-w-0 items-center gap-2 font-semibold tracking-tight sm:w-auto" href="/">
              <span className="grid size-8 flex-none place-items-center rounded-xl bg-primary text-primary-foreground">
                <BookOpen className="size-4" aria-hidden="true" />
              </span>
              <span className="min-w-0 truncate">{copy.brand}</span>
            </Link>
            <div className="grid w-full min-w-0 grid-cols-5 gap-1.5 text-sm text-muted-foreground sm:flex sm:w-auto sm:items-center sm:justify-end sm:gap-2">
              <Link className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/courses">
                {copy.courses}
              </Link>
              <Link className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/forum">
                {copy.forum}
              </Link>
              <a className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/#guarantee">
                {copy.guarantee}
              </a>
              <Link className="min-w-0 rounded-lg px-1.5 py-2 text-center hover:bg-muted hover:text-foreground sm:px-3" href="/me">
                {copy.me}
              </Link>
              <Link className="min-w-0 rounded-lg border border-border bg-card px-1.5 py-2 text-center text-foreground hover:bg-muted sm:px-3" href="/login">
                {copy.login}
              </Link>
            </div>
          </nav>
        </header>
        {children}
        <footer className="pb-6 text-center text-xs text-muted-foreground">
          <span className="inline-flex max-w-full flex-wrap items-center justify-center gap-x-1 gap-y-1 break-words">
            <ShieldCheck className="size-3 flex-none" aria-hidden="true" />
            <span>{copy.footer}</span>
          </span>
        </footer>
      </div>
    </main>
  );
}
