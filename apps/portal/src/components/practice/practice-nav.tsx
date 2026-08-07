"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import TransitionLink from "@/components/practice/transition/transition-link";
import AccountEntry from "@/components/account/account-entry";
import { quizCraftV2ReadsEnabled } from "@/lib/api/env";
import { cn } from "@/lib/cn";

const TABS = [
  {
    href: "/practice",
    index: "P-01",
    label: "题库",
    match: (p: string) => p === "/practice" || p.startsWith("/practice/lists"),
  },
  { href: "/practice/quiz", index: "P-02", label: "刷题", match: (p: string) => p.startsWith("/practice/quiz") },
  { href: "/practice/stats", index: "P-03", label: "数据", match: (p: string) => p.startsWith("/practice/stats") },
];

export default function PracticeNav() {
  const pathname = usePathname();

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-paper">
      <div className="mx-auto flex min-h-14 max-w-6xl flex-wrap items-center px-5 md:flex-nowrap md:justify-between md:px-8">
        <div className="flex h-14 w-full items-center justify-between md:h-auto md:w-auto md:justify-start md:gap-4">
          <div className="flex items-baseline gap-4">
          <Link
            href="/"
            className="font-mono text-xs tracking-widest text-ink/60 transition-colors hover:text-accent"
          >
            ← henukit
          </Link>
          <span className="font-display text-base font-bold tracking-tight">
            PRACTICE<span className="text-accent">®</span>
          </span>
          </div>
          <div className="md:hidden">
            <AccountEntry compact />
          </div>
        </div>

        <nav className="order-3 -mx-5 flex w-[calc(100%+2.5rem)] min-w-0 items-center gap-5 overflow-x-auto border-t border-line px-5 py-2 scrollbar-none md:order-none md:mx-0 md:w-auto md:gap-8 md:overflow-visible md:border-t-0 md:px-0 md:py-0">
          {[...TABS, ...(quizCraftV2ReadsEnabled() ? [
            { href: "/practice/leaderboard", index: "P-04", label: "排行榜", match: (p: string) => p.startsWith("/practice/leaderboard") },
            // P-06 (not P-05) anticipates the #266 favorites renumbering, which
            // moves leaderboard to P-05 after favorites joins the tabs.
            { href: "/practice/mistakes", index: "P-06", label: "错题", match: (p: string) => p.startsWith("/practice/mistakes") },
          ] : [])].map((tab) => {
            const active = tab.match(pathname);
            return (
              <TransitionLink
                key={tab.href}
                href={tab.href}
                className={cn(
                  "group relative py-1 font-mono text-xs tracking-widest transition-colors",
                  active ? "text-ink" : "text-ink/50 hover:text-ink"
                )}
              >
                <span className={cn("mr-1", active ? "text-accent" : "text-ink/30")}>
                  {tab.index}
                </span>
                {tab.label}
                <span
                  aria-hidden
                  className={cn(
                    "absolute inset-x-0 -bottom-0.5 h-px origin-left bg-accent transition-transform duration-300",
                    active ? "scale-x-100" : "scale-x-0 group-hover:scale-x-100"
                  )}
                />
              </TransitionLink>
            );
          })}
          <span aria-hidden className="hidden h-4 w-px bg-ink/20 md:block" />
          <span className="hidden md:block">
            <AccountEntry compact />
          </span>
        </nav>
      </div>
    </header>
  );
}
