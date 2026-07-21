"use client";

import { useState } from "react";
import { useSyncExternalStore } from "react";
import { accountStore, MEMBERSHIP_PLANS } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function MembershipPage() {
  const data = useSyncExternalStore(accountStore.subscribe, accountStore.get, accountStore.getServer);
  useReveal();
  const [confirmPlan, setConfirmPlan] = useState<(typeof MEMBERSHIP_PLANS)[number] | null>(null);
  const [pending, setPending] = useState(false);

  const m = data.membership;
  const progress = Math.round((m.daysLeft / m.totalDays) * 100);

  const open = () => {
    if (!confirmPlan) return;
    setPending(true);
    setTimeout(() => {
      const expire = new Date(Date.now() + confirmPlan.days * 86400000)
        .toISOString()
        .slice(0, 10);
      accountStore.openMembership(confirmPlan.name, confirmPlan.days, expire);
      setPending(false);
      setConfirmPlan(null);
    }, 600);
  };

  return (
    <div>
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">A-04</span>
        <span className="mx-2">/</span>
        MEMBERSHIP
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">会员</h1>

      {/* 当前状态 */}
      <section data-enter className="mt-8 border border-ink p-6 md:p-8">
        <div className="flex flex-wrap items-baseline justify-between gap-3">
          <p className="font-display text-2xl font-bold">{m.plan}</p>
          <p className="font-mono text-xs text-ink/60">到期日 {m.expire}</p>
        </div>
        <div className="mt-5">
          <div className="mb-1.5 flex justify-between font-mono text-[10px] text-ink/50">
            <span>剩余 {m.daysLeft} 天</span>
            <span>共 {m.totalDays} 天</span>
          </div>
          <div className="h-1.5 w-full bg-ink/10">
            <div className="h-full bg-accent" style={{ width: `${progress}%` }} />
          </div>
        </div>
      </section>

      {/* 套餐 */}
      <div className="mt-10 grid gap-4 md:grid-cols-3">
        {MEMBERSHIP_PLANS.map((p) => (
          <div
            key={p.id}
            data-enter
            className={cn(
              "relative border p-6",
              p.recommended ? "border-ink" : "border-ink/25"
            )}
          >
            {p.recommended && (
              <span className="absolute right-0 top-0 bg-accent px-2 py-0.5 font-mono text-[10px] text-paper">
                推荐
              </span>
            )}
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">{p.name}</p>
            <p className="mt-3 font-display text-4xl font-bold">
              ¥{p.price}
              <span className="ml-1 font-mono text-xs font-normal text-ink/50">
                / {p.days} 天
              </span>
            </p>
            <p className="mt-2 font-mono text-[10px] text-ink/50">{p.note}</p>
            <button
              type="button"
              onClick={() => setConfirmPlan(p)}
              className="mt-5 w-full border border-ink py-2.5 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
            >
              开通
            </button>
          </div>
        ))}
      </div>

      {/* 确认层 */}
      {confirmPlan && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 px-5">
          <div className="w-full max-w-sm border border-ink bg-paper p-7">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
              CONFIRM / 确认开通
            </p>
            <p className="mt-3 text-sm leading-7">
              开通 <span className="font-bold">{confirmPlan.name}</span>（
              {confirmPlan.days} 天），应付{" "}
              <span className="font-display text-xl font-bold text-accent">
                ¥{confirmPlan.price}
              </span>
              。v1 预览为 mock 支付，点击确认立即生效。
            </p>
            <div className="mt-6 flex gap-3">
              <button
                type="button"
                onClick={open}
                disabled={pending}
                className={cn(
                  "flex-1 border py-2.5 font-mono text-xs tracking-widest transition-colors",
                  pending
                    ? "cursor-wait border-line text-ink/40"
                    : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                )}
              >
                {pending ? "支付中…" : "确认支付"}
              </button>
              <button
                type="button"
                onClick={() => setConfirmPlan(null)}
                className="border border-ink/30 px-5 py-2.5 font-mono text-xs tracking-widest hover:border-ink"
              >
                取消
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
