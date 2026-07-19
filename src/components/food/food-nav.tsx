"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import AccountEntry from "@/components/account/account-entry";
import { cn } from "@/lib/cn";

const TABS = [
  { href: "/food", index: "F-01", label: "首页", match: (p: string) => p === "/food" || p.startsWith("/food/campus") || p.startsWith("/food/post") },
  { href: "/food/leaderboard", index: "F-02", label: "排行榜", match: (p: string) => p.startsWith("/food/leaderboard") },
  { href: "/food/publish", index: "F-03", label: "发布", match: (p: string) => p.startsWith("/food/publish") },
];

export default function FoodNav() {
  const pathname = usePathname();
  return (
    <header className="sticky top-0 z-40 border-b border-line bg-paper">
      <div className="mx-auto flex h-14 max-w-[1440px] items-center justify-between px-5 md:px-8">
        <div className="flex items-baseline gap-4">
          <Link href="/" className="font-mono text-xs tracking-widest text-ink/60 transition-colors hover:text-accent">
            ← henukit
          </Link>
          <span className="font-display text-base font-bold tracking-tight">
            FOOD<span className="text-accent">®</span>
          </span>
        </div>
        <nav className="flex items-center gap-5 md:gap-8">
          {TABS.map((tab) => {
            const active = tab.match(pathname);
            return (
              <Link
                key={tab.href}
                href={tab.href}
                className={cn(
                  "group relative py-1 font-mono text-xs tracking-widest transition-colors",
                  active ? "text-ink" : "text-ink/50 hover:text-ink"
                )}
              >
                <span className={cn("mr-1", active ? "text-accent" : "text-ink/30")}>{tab.index}</span>
                {tab.label}
                <span
                  aria-hidden
                  className={cn(
                    "absolute inset-x-0 -bottom-0.5 h-px origin-left bg-accent transition-transform duration-300",
                    active ? "scale-x-100" : "scale-x-0 group-hover:scale-x-100"
                  )}
                />
              </Link>
            );
          })}
          <span aria-hidden className="hidden h-4 w-px bg-ink/20 sm:block" />
          <AccountEntry compact />
        </nav>
      </div>
    </header>
  );
}
