"use client";

import Link from "next/link";
import { useReveal } from "@/components/account/use-reveal";

const BENEFITS = [
  "在后台扫描已收录的官方招聘来源",
  "匹配结果与命中原因一目了然",
  "完成后向已验证的账户邮箱发送结果简报",
];

/** 已登录但非 Lifetime：产品介绍 + 会员权益说明 + ¥9.9 购买 CTA；无任何可执行的扫描入口。 */
export default function CareerFreeView() {
  useReveal();

  return (
    <section data-career-state="free" className="mt-10">
      <div className="grid gap-10 md:grid-cols-2">
        <div data-enter>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">R-01</span>
            <span className="mx-2">/</span>
            WORK RADAR
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
            求职雷达属于终身会员权益
          </h1>
          <p className="mt-4 max-w-xl text-sm leading-7 text-ink/70">
            设定求职画像后，雷达会在后台扫描已收录的官方招聘来源，
            匹配结果与命中原因一目了然，完成后自动把结果简报发送到已验证的账户邮箱。
          </p>
          <ul className="mt-6 space-y-2 font-mono text-xs tracking-wider text-ink/60">
            {BENEFITS.map((b) => (
              <li key={b}>
                <span className="mr-2 text-accent">+</span>
                {b}
              </li>
            ))}
          </ul>
        </div>

        <div data-enter className="flex items-start">
          <article className="w-full max-w-md border border-dashed border-ink/25 bg-paper p-6">
            <div className="flex items-center justify-between font-mono text-[10px] tracking-[0.25em] text-ink/50">
              <span>LIFETIME MEMBERSHIP</span>
              <span className="border border-ink/30 px-2 py-0.5 text-ink/50">¥9.9 终身</span>
            </div>
            <h3 className="mt-4 font-display text-2xl font-bold">一次付费，无需续费</h3>
            <p className="mt-2 text-sm leading-6 text-ink/60">
              开通终身会员后即可使用求职雷达：发起扫描、查看匹配结果、
              接收邮件简报；权益绑定账户，换设备登录同样可用。
            </p>
            <Link
              href="/account/membership"
              className="mt-6 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
            >
              ¥9.9 开通终身会员 →
            </Link>
            <p className="mt-3 font-mono text-[10px] tracking-[0.15em] text-ink/40">
              在账户中心完成支付，开通后立即生效
            </p>
          </article>
        </div>
      </div>
    </section>
  );
}
