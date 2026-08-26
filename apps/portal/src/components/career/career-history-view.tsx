"use client";

import Link from "next/link";
import { useReveal } from "@/components/account/use-reveal";
import {
  careerDigestStatusLabel,
  careerScanFailedMessage,
  careerScanStageLabel,
  careerSearchStatusLabel,
  formatCareerSearchTime,
} from "@/lib/career/career-scan-state";
import type { CareerHistoryViewState } from "@/lib/career/page-state";

/**
 * /career/history 历史页（#402）：当前用户搜索历史列表。
 * 只展示网关持久化字段（status/stage/created_at/has_email/error 摘要），
 * 不本地伪造计数；非 Lifetime 分支给出明确提示。
 */
export default function CareerHistoryView({ state }: { state: CareerHistoryViewState }) {
  useReveal();

  if (state.kind === "anonymous") {
    return (
      <section data-career-history-state="anonymous" className="mt-10 max-w-2xl">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">R-02</span>
          <span className="mx-2">/</span>
          SIGN IN REQUIRED
        </p>
        <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
          登录后查看扫描历史
        </h1>
        <p data-enter className="mt-4 text-sm leading-7 text-ink/70">
          扫描历史按账户保存。登录后即可查看每次扫描的状态与结果记录。
        </p>
        <Link
          data-enter
          href="/account/login?next=/career/history"
          className="mt-8 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          登录后查看 →
        </Link>
      </section>
    );
  }

  if (state.kind === "free") {
    return (
      <section data-career-history-state="free" className="mt-10 max-w-2xl">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">R-02</span>
          <span className="mx-2">/</span>
          LIFETIME REQUIRED
        </p>
        <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
          扫描历史属于 Lifetime VIP 权益
        </h1>
        <p data-enter className="mt-4 text-sm leading-7 text-ink/70">
          当前账户不是 Lifetime VIP 会员，无法查看求职雷达的扫描历史。
          开通后即可发起扫描并查看每次任务的记录。
        </p>
        <Link
          data-enter
          href="/account/membership"
          className="mt-8 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          ¥9.9 开通 Lifetime VIP →
        </Link>
      </section>
    );
  }

  if (state.kind === "lifetime-no-profile") {
    return (
      <section data-career-history-state="no-profile" className="mt-10 max-w-2xl">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">R-02</span>
          <span className="mx-2">/</span>
          PROFILE REQUIRED
        </p>
        <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
          先完成求职画像，再开始扫描
        </h1>
        <p data-enter className="mt-4 text-sm leading-7 text-ink/70">
          完成求职画像后才能发起扫描，扫描记录也会在这里汇总。
        </p>
        <Link
          data-enter
          href="/account/profile"
          className="mt-8 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          去设置求职画像 →
        </Link>
      </section>
    );
  }

  if (state.kind === "error") {
    return (
      <section
        data-career-history-state="error"
        role="alert"
        className="mt-10 max-w-2xl border border-accent px-5 py-6"
      >
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">R-02</span>
          <span className="mx-2">/</span>
          HISTORY UNAVAILABLE
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
          扫描历史暂时不可用
        </h1>
        <p className="mt-4 text-sm leading-6 text-ink/65">{state.message}</p>
        <p className="mt-3 text-sm leading-6 text-ink/60">
          扫描历史读取失败时，不会以本地或会话数据替代真实记录。
        </p>
        <Link
          href="/career"
          className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          返回扫描页
        </Link>
      </section>
    );
  }

  const { searches } = state;

  if (searches.length === 0) {
    return (
      <section data-career-history-state="empty" className="mt-10">
        <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">R-02</span>
          <span className="mx-2">/</span>
          NO SCAN RECORDS
        </p>
        <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
          扫描历史
        </h1>
        <div data-enter className="mt-8 max-w-2xl border border-line px-5 py-12 text-center">
          <p className="text-sm leading-6 text-ink/60">
            还没有扫描记录。发起第一次扫描后，任务状态与结果会汇总在这里。
          </p>
          <Link
            href="/career"
            className="mt-6 inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            去发起扫描 →
          </Link>
        </div>
      </section>
    );
  }

  return (
    <section data-career-history-state="ready" className="mt-10">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">R-02</span>
        <span className="mx-2">/</span>
        SCAN HISTORY
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
        扫描历史
      </h1>
      <div className="mt-8 max-w-3xl">
        <ul data-enter className="divide-y divide-line border border-line">
          {searches.map((search) => (
            <li key={search.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span
                    className={
                      search.status === "failed"
                        ? "border border-accent/60 px-2 py-0.5 font-mono text-[10px] tracking-widest text-accent"
                        : search.status === "completed"
                          ? "border border-ink/40 px-2 py-0.5 font-mono text-[10px] tracking-widest text-ink/70"
                          : "border border-ink/30 px-2 py-0.5 font-mono text-[10px] tracking-widest text-ink/60"
                    }
                  >
                    {careerSearchStatusLabel(search.status)}
                  </span>
                  {search.status === "running" && search.stage ? (
                    <span className="font-mono text-[10px] tracking-wider text-ink/40">
                      {careerScanStageLabel(search.stage)}
                    </span>
                  ) : null}
                  <span className="font-mono text-[10px] tracking-wider text-ink/40">
                    #{search.id.slice(0, 8)}
                  </span>
                </div>
                <p className="mt-1.5 text-xs leading-5 text-ink/55">
                  {formatCareerSearchTime(search.created_at)}
                  {careerDigestStatusLabel(search)
                    ? ` · ${careerDigestStatusLabel(search)}`
                    : ""}
                </p>
                {search.status === "failed" ? (
                  <p className="mt-1 text-xs leading-5 text-ink/50">
                    {careerScanFailedMessage()}。可在扫描页重新发起。
                  </p>
                ) : null}
              </div>
              <Link
                href={`/career?search=${encodeURIComponent(search.id)}`}
                className="shrink-0 font-mono text-[11px] tracking-widest text-ink/60 transition-colors hover:text-accent"
              >
                查看详情 →
              </Link>
            </li>
          ))}
        </ul>
        <p data-enter className="mt-4 font-mono text-[10px] tracking-[0.15em] text-ink/40">
          记录来自服务端持久化的扫描任务，不展示本地或会话数据
        </p>
      </div>
    </section>
  );
}
