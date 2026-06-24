import Link from "next/link";
import { ArrowRight, Search } from "lucide-react";
import { heroLinks } from "./home-data";

export function HeroIntro() {
  return (
    <section className="relative mx-auto grid min-h-[calc(100dvh-88px)] w-[min(1160px,calc(100%-32px))] items-start gap-8 pb-8 pt-20 lg:grid-cols-[0.9fr_1.1fr] lg:pt-28">
      <div className="z-10 max-w-xl">
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Final Review Platform</p>
        <h1 className="mt-5 text-5xl font-black leading-[0.95] tracking-tight text-[#2b2117] sm:text-6xl lg:text-7xl">
          打开你的期末复习资料册
        </h1>
        <p className="mt-6 max-w-lg text-base leading-7 text-[#685b4b] sm:text-lg">
          按课程找到讲义、真题、实验资料和复习包，围绕资料继续刷题、讨论和共创。
        </p>
        <form action="/search" className="mt-7 flex max-w-lg items-center rounded-2xl border border-[#2b2117]/14 bg-white/86 p-2 shadow-[0_20px_60px_rgba(71,49,27,0.11)]" method="get">
          <label className="sr-only" htmlFor="home-search">
            搜索课程、讲义、真题、实验资料
          </label>
          <Search className="ml-2 size-5 shrink-0 text-[#9a7154]" aria-hidden="true" />
          <input id="home-search" name="q" className="min-w-0 flex-1 bg-transparent px-3 py-2 text-sm text-[#2b2117] outline-none placeholder:text-[#9b8b78]" placeholder="搜索课程、讲义、真题、实验资料" type="search" />
          <button className="inline-flex shrink-0 items-center rounded-xl bg-[#2f6b58] px-4 py-2 text-sm font-semibold text-white transition hover:bg-[#285a4b]" type="submit">
            搜索
          </button>
        </form>
        <div className="mt-6 flex flex-wrap gap-3">
          <Link className="inline-flex items-center gap-2 rounded-full bg-[#2b2117] px-5 py-3 text-sm font-semibold text-white shadow-[0_16px_40px_rgba(43,33,23,0.2)] transition hover:-translate-y-0.5" href={heroLinks.primary.href}>
            {heroLinks.primary.label}
            <ArrowRight className="size-4" aria-hidden="true" />
          </Link>
          <Link className="inline-flex items-center rounded-full border border-[#2b2117]/18 bg-white/72 px-5 py-3 text-sm font-semibold text-[#2b2117] transition hover:-translate-y-0.5 hover:bg-white" href={heroLinks.secondary.href}>
            {heroLinks.secondary.label}
          </Link>
        </div>
      </div>
      <div className="hidden lg:block" aria-hidden="true" />
    </section>
  );
}
