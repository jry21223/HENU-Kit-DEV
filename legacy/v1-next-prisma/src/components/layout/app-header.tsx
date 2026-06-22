import Link from "next/link";
import { AuthStatus } from "@/components/auth/auth-status";

const navItems = [
  { href: "/", label: "首页" },
  { href: "/schools", label: "学校专业" },
  { href: "/courses", label: "课程库" },
  { href: "/packages", label: "复习包" },
  { href: "/me/wrong-questions", label: "我的复习" },
  { href: "/me/submissions", label: "资料共建" },
  { href: "/admin", label: "管理后台" },
];

export async function AppHeader() {
  return (
    <header className="sticky top-0 z-20 border-b border-line bg-white/95 backdrop-blur">
      <div className="mx-auto flex max-w-6xl flex-col gap-3 px-4 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
        <Link href="/" className="flex min-w-0 items-center gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-brand text-sm font-bold text-white">
            复
          </span>
          <span className="min-w-0">
            <span className="block truncate text-base font-semibold text-ink">
              一站式期末复习平台
            </span>
            <span className="block truncate text-xs text-muted">
              河南大学软件学院 MVP
            </span>
          </span>
        </Link>
        <nav className="flex flex-wrap items-center gap-2 text-sm text-muted">
          {navItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="rounded-md px-3 py-2 hover:bg-panel hover:text-ink focus-ring"
            >
              {item.label}
            </Link>
          ))}
          <AuthStatus />
        </nav>
      </div>
    </header>
  );
}
