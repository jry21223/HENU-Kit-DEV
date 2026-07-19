"use client";

import { useSyncExternalStore } from "react";
import { accountStore } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function WalletPage() {
  const data = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);
  useReveal();

  return (
    <div>
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">A-03</span>
        <span className="mx-2">/</span>
        WALLET
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">积分钱包</h1>

      {/* 余额 + 签到 */}
      <div data-enter className="mt-8 flex flex-wrap items-end justify-between gap-6 border border-ink p-6 md:p-8">
        <div>
          <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
            BALANCE / 当前余额
          </p>
          <p className="mt-2 font-display text-6xl font-bold tabular-nums">
            {data.balance}
            <span className="ml-2 font-mono text-sm font-normal text-ink/50">积分</span>
          </p>
        </div>
        <button
          type="button"
          onClick={() => accountStore.signIn()}
          disabled={data.signedToday}
          className={cn(
            "border px-6 py-3 font-mono text-sm tracking-widest transition-colors",
            data.signedToday
              ? "cursor-not-allowed border-line text-ink/40"
              : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
          )}
        >
          {data.signedToday ? "今日已签到 ✓" : "每日签到 +10"}
        </button>
      </div>

      {/* 流水 */}
      <div data-enter className="mt-10">
        <div className="grid grid-cols-[6rem_1fr_4rem] gap-3 border-b border-ink/40 pb-2 font-mono text-[10px] tracking-[0.25em] text-ink/40">
          <span>时间</span>
          <span>事项</span>
          <span className="text-right">变动</span>
        </div>
        <ul>
          {data.txns.map((t) => (
            <li
              key={t.id}
              className="grid grid-cols-[6rem_1fr_4rem] items-baseline gap-3 border-b border-line py-3"
            >
              <span className="font-mono text-[11px] text-ink/50">{t.time}</span>
              <span className="truncate text-sm">{t.item}</span>
              <span
                className={cn(
                  "text-right font-mono text-sm",
                  t.amount > 0 ? "text-ink" : "text-accent"
                )}
              >
                {t.amount > 0 ? `+${t.amount}` : t.amount}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
