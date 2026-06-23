import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { recentUpdates } from "./home-data";

export function FinalHomeCta() {
  return (
    <section className="mx-auto grid w-[min(1120px,calc(100%-32px))] gap-8 rounded-[2rem] border border-[#2b2117]/12 bg-[#fffaf2] p-6 shadow-[0_24px_90px_rgba(71,49,27,0.12)] lg:grid-cols-[0.9fr_1.1fr] lg:p-10">
      <div>
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Course Library</p>
        <h2 className="mt-3 text-3xl font-black tracking-tight text-[#2b2117] sm:text-4xl">从一门课开始复习</h2>
        <p className="mt-4 text-sm leading-7 text-[#6f604f]">
          首页讲完整产品愿景，真正的学习路径仍从课程资料库开始。
        </p>
        <Link className="mt-6 inline-flex items-center gap-2 rounded-full bg-[#2f6b58] px-5 py-3 text-sm font-semibold text-white transition hover:-translate-y-0.5 hover:bg-[#285a4b]" href="/courses">
          进入课程资料库
          <ArrowRight className="size-4" aria-hidden="true" />
        </Link>
      </div>
      <div className="grid gap-3">
        {recentUpdates.map((item) => (
          <Link key={`${item.course}-${item.title}`} className="group flex items-center justify-between gap-4 rounded-2xl border border-[#2b2117]/10 bg-white/72 p-4 transition hover:-translate-y-0.5 hover:bg-white" href={item.href}>
            <span className="min-w-0">
              <span className="block truncate text-sm font-semibold text-[#2b2117]">{item.title}</span>
              <span className="mt-1 block text-xs text-[#7a6a58]">{item.course} / {item.label}</span>
            </span>
            <ArrowRight className="size-4 shrink-0 text-[#b75c32] transition group-hover:translate-x-1" aria-hidden="true" />
          </Link>
        ))}
      </div>
    </section>
  );
}
