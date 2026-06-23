import Link from "next/link";
import { BookOpen, Download } from "lucide-react";

const links = [
  { label: "课程资料", href: "/courses" },
  { label: "社区共创", href: "#community" },
  { label: "刷题 AI", href: "#practice" },
  { label: "资料保障", href: "#guarantee" },
];

export function HomeNav() {
  return (
    <header className="sticky top-3 z-40 mx-auto flex w-[min(1120px,calc(100%-24px))] items-center justify-between rounded-full border border-[#2b2117]/12 bg-[#fffaf2]/86 px-3 py-2 shadow-[0_16px_48px_rgba(71,49,27,0.12)] backdrop-blur-md">
      <Link aria-label="软件学院资料库" className="flex min-w-0 items-center gap-2 rounded-full pr-2 text-sm font-semibold text-[#2b2117]" href="/">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-[#2f6b58] text-white">
          <BookOpen className="size-4" aria-hidden="true" />
        </span>
        <span className="hidden sm:inline">软件学院资料库</span>
      </Link>
      <nav className="hidden items-center gap-1 text-sm text-[#6f604f] md:flex">
        {links.map((link) => (
          <Link key={link.href} className="rounded-full px-3 py-2 transition hover:bg-[#f0e4d2] hover:text-[#2b2117]" href={link.href}>
            {link.label}
          </Link>
        ))}
      </nav>
      <Link aria-label="我的下载" className="inline-flex items-center gap-2 rounded-full border border-[#2b2117]/16 bg-white px-3 py-2 text-sm font-medium text-[#2b2117] shadow-sm transition hover:-translate-y-0.5 hover:shadow-md" href="/me/downloads">
        <Download className="size-4" aria-hidden="true" />
        <span className="hidden sm:inline">我的下载</span>
      </Link>
    </header>
  );
}
