"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { campusStore } from "@/lib/campus/mock";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

const STATUS_LABEL = { open: "待接单", ongoing: "进行中", done: "已完成", hidden: "已隐藏" } as const;

export default function AccountDealsPage() {
  const data = useSyncExternalStore(campusStore.subscribe, campusStore.get, campusStore.getServer);
  useReveal();

  const mine = data.items.filter((i) => i.isMine);
  const ongoingDeals = data.deals.filter((d) => d.status === "ongoing");

  return (
    <div>
      <div data-enter className="flex items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">A-08</span>
            <span className="mx-2">/</span>
            DEALS
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">我的交易</h1>
        </div>
        <Link
          href="/campus/deals"
          className="border border-ink px-5 py-2.5 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          完整管理 →
        </Link>
      </div>

      {/* 进行中的订单 */}
      <section data-enter className="mt-8">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
          进行中订单 · {ongoingDeals.length}
        </p>
        <div className="mt-4 border-t border-ink/40">
          {ongoingDeals.length === 0 ? (
            <p className="border-b border-line py-8 text-center font-mono text-xs text-ink/40">
              暂无进行中订单
            </p>
          ) : (
            ongoingDeals.map((d) => (
              <div key={d.id} className="flex flex-wrap items-center gap-x-5 gap-y-2 border-b border-line py-4">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{d.title}</p>
                  <p className="mt-1 font-mono text-[10px] text-ink/50">
                    {d.id} · 对方 {d.other} · <span className="text-accent">¥</span>
                    {d.price}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => campusStore.confirmDone(d.itemId)}
                  className="border border-ink bg-ink px-4 py-2 font-mono text-xs text-paper transition-colors hover:border-accent hover:bg-accent"
                >
                  确认完成 → 结算
                </button>
              </div>
            ))
          )}
        </div>
      </section>

      {/* 我发布的 */}
      <section data-enter className="mt-10">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
          我发布的 · {mine.length}
        </p>
        <div className="mt-4 border-t border-ink/40">
          {mine.map((it) => (
            <div key={it.id} className="flex flex-wrap items-center gap-x-5 gap-y-2 border-b border-line py-4">
              <Link href={`/campus/item/${it.id}`} className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium transition-colors hover:text-accent">
                  {it.title}
                </p>
                <p className="mt-1 font-mono text-[10px] text-ink/50">
                  <span className="text-accent">¥</span>
                  {it.price} · 想要 {it.wants}
                </p>
              </Link>
              <span
                className={cn(
                  "border px-1.5 py-0.5 font-mono text-[10px]",
                  it.status === "open" && "border-accent text-accent",
                  it.status === "ongoing" && "border-ink text-ink",
                  (it.status === "done" || it.status === "hidden") && "border-line text-ink/40"
                )}
              >
                {STATUS_LABEL[it.status]}
              </span>
              {(it.status === "open" || it.status === "hidden") && (
                <div className="flex gap-2">
                  <Link
                    href={`/campus/publish?edit=${it.id}`}
                    className="border border-ink/30 px-3 py-1.5 font-mono text-xs transition-colors hover:border-ink"
                  >
                    编辑
                  </Link>
                  <button
                    type="button"
                    onClick={() => campusStore.toggleHidden(it.id)}
                    className="border border-ink/30 px-3 py-1.5 font-mono text-xs transition-colors hover:border-ink"
                  >
                    {it.status === "hidden" ? "显示" : "隐藏"}
                  </button>
                </div>
              )}
              {it.status === "ongoing" && (
                <button
                  type="button"
                  onClick={() => campusStore.confirmDone(it.id)}
                  className="border border-ink bg-ink px-3 py-1.5 font-mono text-xs text-paper transition-colors hover:border-accent hover:bg-accent"
                >
                  确认完成
                </button>
              )}
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
