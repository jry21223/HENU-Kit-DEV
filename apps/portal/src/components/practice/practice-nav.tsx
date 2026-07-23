"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import TransitionLink from "@/components/practice/transition/transition-link";
import AccountEntry from "@/components/account/account-entry";
import { cn } from "@/lib/cn";

const TABS = [
  {
    href: "/practice",
    index: "P-01",
    label: "题库",
    match: (p: string) => p === "/practice" || p.startsWith("/practice/lists"),
  },
  { href: "/practice/quiz", index: "P-02", label: "刷题", match: (p: string) => p.startsWith("/practice/quiz") },
  { href: "/practice/leaderboard", index: "P-03", label: "排行榜", match: (p: string) => p.startsWith("/practice/leaderboard") },
  { href: "/practice/stats", index: "P-04", label: "数据", match: (p: string) => p.startsWith("/practice/stats") },
];

export default function PracticeNav() {
  const pathname = usePathname();

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-paper">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-5 md:px-8">
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

        <nav className="flex items-center gap-5 md:gap-8">
          {TABS.map((tab) => {
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
          <span aria-hidden className="hidden h-4 w-px bg-ink/20 sm:block" />
          <AccountEntry compact />
        </nav>
      </div>
    </header>
  );
}
