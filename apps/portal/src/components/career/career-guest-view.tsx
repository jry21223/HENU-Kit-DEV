"use client";

import Link from "next/link";
import WorkRadar from "@/components/career/work-radar";
import { useReveal } from "@/components/account/use-reveal";

// 描述段说扫描什么，要点只列描述段之外的好处，两处不重复（#549）。
const FEATURES = [
  "无需守在页面",
  "匹配结果与命中原因一目了然",
  "完成后向已验证的账户邮箱发送结果简报",
];

/** 未登录：产品介绍 + 雷达视觉 + 登录 CTA，不含任何个人数据。 */
export default function CareerGuestView() {
  useReveal();

  return (
    <section data-career-state="anonymous" className="mt-10">
      <div className="grid items-center gap-12 md:grid-cols-2">
        <div data-enter>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent-text">R-01</span>
            <span className="mx-2">/</span>
            WORK RADAR
          </p>
          {/* 按词组换行：窄屏上不把“招聘”拆到两行（#549）。 */}
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
            <span className="inline-block">让雷达替你</span>
            <span className="inline-block">扫一遍</span>
            <span className="inline-block">招聘信息</span>
          </h1>
          <p className="mt-4 max-w-xl text-sm leading-7 text-ink/70">
            设定求职画像后，雷达会在后台扫描已收录的官方招聘来源。
          </p>
          <ul className="mt-6 space-y-2 font-mono text-xs tracking-wider text-ink/60">
            {FEATURES.map((f) => (
              <li key={f}>
                <span aria-hidden className="mr-2 text-accent-text">+</span>
                {f}
              </li>
            ))}
          </ul>
          <Link
            href="/account/login?next=/career"
            className="mt-8 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            登录后开始使用 →
          </Link>
        </div>

        <div data-enter className="flex items-center justify-center">
          <WorkRadar compact schematic className="w-full" />
        </div>
      </div>
    </section>
  );
}
