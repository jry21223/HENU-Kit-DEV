"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { campusStore } from "@/lib/campus/mock";
import { hasGateway } from "@/lib/api/client";
import { getItems } from "@/lib/campus/gateway";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

const STATUS_LABEL = { open: "待接单", ongoing: "进行中", done: "已完成", hidden: "已隐藏" } as const;
const STATUS_CLS = {
  open: "border-accent text-accent",
  ongoing: "border-ink text-ink",
  done: "border-line text-ink/40",
  hidden: "border-line text-ink/40",
} as const;

export default function DealsPage() {
  const router = useRouter();
  const { user, ready } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  const data = useSyncExternalStore(campusStore.subscribe, campusStore.get, campusStore.getServer);
  const [campusItems, setCampusItems] = useState(data.items);
  const [tab, setTab] = useState<"mine" | "taken">("mine");
  useReveal();

  useEffect(() => {
    if (!hasGateway) return;
    let cancelled = false;
    getItems().then((resp) => {
      if (!cancelled && resp) setCampusItems(resp as typeof data.items);
    });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (ready && !user) router.replace("/account/login?next=/campus/deals");
  }, [ready, user, router]);

  if (!ready || !user) {
    return (
      <main className="flex min-h-[60vh] items-center justify-center">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
          AUTH CHECK<span className="animate-pulse text-accent">…</span>
        </p>
      </main>
    );
  }

  const mine = campusItems.filter((i) => i.isMine);

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">M-02</span>
        <span className="mx-2">/</span>
        MY DEALS
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">我的交易</h1>

      <div data-enter className="mt-6 flex gap-2">
        {(["mine", "taken"] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={cn(
              "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
              tab === t ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
            )}
          >
            {t === "mine" ? `我发布的（${mine.length}）` : `我接的·我买的（${data.deals.length}）`}
          </button>
        ))}
      </div>

      {tab === "mine" ? (
        <div data-enter className="mt-6 border-t border-ink/40">
          {mine.length === 0 ? (
            <p className="border-b border-line py-12 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
              还没有发布过单子 / EMPTY
            </p>
          ) : (
            mine.map((it) => (
              <div key={it.id} className="flex flex-wrap items-center gap-x-5 gap-y-2 border-b border-line py-4">
                <Link href={`/campus/item/${it.id}`} className="min-w-0 flex-1">
                  <p className="truncate font-medium transition-colors hover:text-accent">{it.title}</p>
                  <p className="mt-1 font-mono text-[10px] tracking-wider text-ink/50">
                    <span className="text-accent">¥</span>
                    {it.price} · 想要 {it.wants} · {it.time}
                  </p>
                </Link>
                <span className={cn("border px-1.5 py-0.5 font-mono text-[10px]", STATUS_CLS[it.status])}>
                  {STATUS_LABEL[it.status]}
                </span>
                <div className="flex flex-wrap gap-2">
                  {it.status === "ongoing" && (
                    <button
                      type="button"
                      onClick={() => campusStore.confirmDone(it.id)}
                      className="border border-ink bg-ink px-3 py-1.5 font-mono text-xs text-paper transition-colors hover:border-accent hover:bg-accent"
                    >
                      确认完成 → 结算
                    </button>
                  )}
                  {(it.status === "open" || it.status === "hidden") && (
                    <>
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
                      <button
                        type="button"
                        onClick={() => campusStore.removeItem(it.id)}
                        className="border border-accent/60 px-3 py-1.5 font-mono text-xs text-accent transition-colors hover:border-accent"
                      >
                        下架删除
                      </button>
                    </>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      ) : (
        <div data-enter className="mt-6 space-y-4">
          {data.deals.length === 0 ? (
            <p className="border border-dashed border-ink/30 px-5 py-12 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
              暂无订单 / EMPTY
            </p>
          ) : (
            data.deals.map((d) => {
              const item = campusItems.find((i) => i.id === d.itemId);
              return (
                <div key={d.id} className="border border-ink/25 p-5">
                  <div className="flex flex-wrap items-baseline justify-between gap-2">
                    <Link href={item ? `/campus/item/${item.id}` : "#"} className="font-medium hover:text-accent">
                      {d.title}
                    </Link>
                    <span
                      className={cn(
                        "border px-1.5 py-0.5 font-mono text-[10px]",
                        d.status === "done" ? "border-line text-ink/40" : "border-ink text-ink"
                      )}
                    >
                      {d.status === "done" ? "已结算" : "进行中"}
                    </span>
                  </div>
                  <p className="mt-2 font-mono text-[11px] text-ink/50">
                    {d.id} · 对方 {d.other} · {d.role === "taker" ? "我接单" : "我购买"} ·{" "}
                    <span className="text-accent">¥</span>
                    {d.price}
                  </p>
                  {/* 时间线 */}
                  <div className="mt-4 flex flex-wrap gap-x-4 gap-y-1.5 border-t border-line pt-3">
                    {d.timeline.map((t, i) => (
                      <p key={i} className="font-mono text-[10px] text-ink/50">
                        <span className={cn("mr-1", i === d.timeline.length - 1 && d.status === "ongoing" ? "text-accent" : "")}>
                          {String(i + 1).padStart(2, "0")}
                        </span>
                        {t.label} {t.time}
                      </p>
                    ))}
                  </div>
                  {d.status === "ongoing" && (
                    <div className="mt-4 flex gap-2">
                      <button
                        type="button"
                        onClick={() => campusStore.confirmDone(d.itemId)}
                        className="border border-ink bg-ink px-4 py-2 font-mono text-xs text-paper transition-colors hover:border-accent hover:bg-accent"
                      >
                        确认完成
                      </button>
                      <button
                        type="button"
                        onClick={() => campusStore.cancelDeal(d.itemId)}
                        className="border border-ink/30 px-4 py-2 font-mono text-xs transition-colors hover:border-ink"
                      >
                        取消订单
                      </button>
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      )}
    </main>
  );
}
