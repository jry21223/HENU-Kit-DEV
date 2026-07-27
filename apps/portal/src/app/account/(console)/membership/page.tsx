"use client";

import { useState } from "react";
import { useSyncExternalStore } from "react";
import { accountStore, MEMBERSHIP_PLANS } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function MembershipPage() {
  const data = useSyncExternalStore(
    accountStore.subscribe,
    accountStore.get,
    accountStore.getServer
  );
  useReveal();
  const [confirmPlan, setConfirmPlan] = useState<
    (typeof MEMBERSHIP_PLANS)[number] | null
  >(null);
  const [pending, setPending] = useState(false);

  const m = data.membership;
  const isLifetime = m.lifetime;

  const open = () => {
    if (!confirmPlan?.lifetime) return;
    setPending(true);
    setTimeout(() => {
      accountStore.openLifetimeMembership();
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
      <h1
        data-enter
        className="mt-3 font-display text-4xl font-bold tracking-tight"
      >
        会员
      </h1>

      <section data-enter className="mt-8 border border-ink p-6 md:p-8">
        <div className="flex flex-wrap items-baseline justify-between gap-3">
          <p className="font-display text-2xl font-bold">{m.plan}</p>
          <p className="font-mono text-xs text-ink/60">
            {isLifetime ? "有效期 永久" : "当前档位 · 免费"}
          </p>
        </div>
        <p className="mt-4 font-mono text-[11px] leading-6 tracking-wide text-ink/50">
          {isLifetime
            ? "已开通终身会员，权益长期有效（支付与权益落库仍待服务端接入）。"
            : "默认免费档。付费仅一档：¥9.9 终身会员。"}
        </p>
      </section>

      <div className="mt-10 grid gap-4 md:grid-cols-2">
        <div data-enter className="border border-ink/25 p-6">
          <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
            免费
          </p>
          <p className="mt-3 font-display text-4xl font-bold">
            ¥0
            <span className="ml-1 font-mono text-xs font-normal text-ink/50">
              / 长期
            </span>
          </p>
          <p className="mt-2 font-mono text-[10px] text-ink/50">
            基础浏览与练习
          </p>
          <p className="mt-5 font-mono text-[10px] tracking-widest text-ink/40">
            {isLifetime ? "已升级" : "当前档位"}
          </p>
        </div>

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
                唯一付费档
              </span>
            )}
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
              {p.name}
            </p>
            <p className="mt-3 font-display text-4xl font-bold">
              ¥{p.price}
              <span className="ml-1 font-mono text-xs font-normal text-ink/50">
                / 终身
              </span>
            </p>
            <p className="mt-2 font-mono text-[10px] text-ink/50">{p.note}</p>
            <button
              type="button"
              disabled={isLifetime}
              onClick={() => setConfirmPlan(p)}
              className={cn(
                "mt-5 w-full border py-2.5 font-mono text-xs tracking-widest transition-colors",
                isLifetime
                  ? "cursor-default border-line text-ink/40"
                  : "border-ink hover:bg-ink hover:text-paper"
              )}
            >
              {isLifetime ? "已开通" : "开通"}
            </button>
          </div>
        ))}
      </div>

      {confirmPlan && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink/40 px-5">
          <div className="w-full max-w-sm border border-ink bg-paper p-7">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
              CONFIRM / 确认开通
            </p>
            <p className="mt-3 text-sm leading-7">
              开通 <span className="font-bold">{confirmPlan.name}</span>
              ，应付{" "}
              <span className="font-display text-xl font-bold text-accent">
                ¥{confirmPlan.price}
              </span>
              。当前为会话内预览：未接真实支付，确认后仅本地标记为终身会员。
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
                {pending ? "处理中…" : "确认"}
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
