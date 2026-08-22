"use client";

import Link from "next/link";
import { useReveal } from "@/components/account/use-reveal";

const STEPS = [
  "目标岗位 / 方向，例如后端开发、数据分析",
  "技术栈关键词与目标城市",
  "经历摘要（可选），用于命中原因说明",
];

/** Lifetime 但画像为空：引导完成求职画像，可跳转账户设置页。 */
export default function CareerNoProfileView() {
  useReveal();

  return (
    <section data-career-state="lifetime-no-profile" className="mt-10">
      <div className="max-w-2xl">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">R-01</span>
          <span className="mx-2">/</span>
          PROFILE REQUIRED
        </p>
        <h2 data-enter className="mt-3 font-display text-3xl font-bold tracking-tight md:text-4xl">
          先完成求职画像，再开始扫描
        </h2>
        <p data-enter className="mt-4 text-sm leading-7 text-ink/70">
          求职雷达根据你的画像匹配受控招聘来源。目前画像尚未设置，
          完成画像后即可发起首次扫描。
        </p>
        <ul className="mt-6 space-y-2 font-mono text-xs tracking-wider text-ink/60">
          {STEPS.map((s) => (
            <li key={s} data-enter>
              <span className="mr-2 text-accent">+</span>
              {s}
            </li>
          ))}
        </ul>
        <Link
          href="/account/profile"
          className="mt-8 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          去设置求职画像 →
        </Link>
        <p className="mt-3 font-mono text-[10px] tracking-[0.15em] text-ink/40">
          简历文件仅在识别任务期间临时保存，任务完成或失败后删除原文件字节；不会保存招聘站账号
        </p>
      </div>
    </section>
  );
}
