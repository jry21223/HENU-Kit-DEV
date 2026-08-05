"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { categoryOf, campusStore } from "@/lib/campus/mock";
import Img from "@/components/ui/img";
import { useReveal } from "@/components/account/use-reveal";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";
import {
  fetchCampusItemDetail,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { CampusItem, CampusMessage } from "@/lib/api/types";

const STATUS_LABEL = { open: "待接单", ongoing: "进行中", done: "已完成", hidden: "已隐藏" } as const;

type LoadState = "loading" | "ready" | "error";

export default function ItemDetail({ id }: { id: string }) {
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [item, setItem] = useState<CampusItem | null>(null);
  const [messages, setMessages] = useState<CampusMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  useReveal();
  const wantRef = useRef<HTMLButtonElement>(null);
  const mounted = useRef(true);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const response = await fetchCampusItemDetail(id);
      if (!mounted.current) return;
      setItem(response.item);
      setMessages(response.messages ?? []);
      setLoadState("ready");
    } catch (loadError) {
      if (!mounted.current) return;
      if (mockAllowed) {
        const data = campusStore.get();
        const mock = data.items.find((candidate) => candidate.id === id);
        if (mock) {
          setItem(mock);
          setMessages(data.messages.filter((m) => m.itemId === id));
          setLoadState("ready");
          return;
        }
      }
      setItem(null);
      setError(formatPortalError(loadError));
      setLoadState("error");
    }
  }, [id]);

  useEffect(() => {
    mounted.current = true;
    const timer = window.setTimeout(() => void load(), 0);
    return () => {
      mounted.current = false;
      window.clearTimeout(timer);
    };
  }, [load]);

  const bounce = () => {
    if (window.matchMedia(REDUCED_MOTION).matches || !wantRef.current) return;
    gsap.fromTo(wantRef.current, { scale: 1 }, { scale: 1.15, duration: 0.15, yoyo: true, repeat: 1 });
  };

  if (loadState === "loading") {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">LOADING / 加载中</p>
        <Link href="/campus" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
          ← 返回市集
        </Link>
      </main>
    );
  }

  if (loadState === "error" || !item) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
        <p className="mt-4 font-display text-2xl font-bold">单子不存在或已下架</p>
        {error && <p className="mt-2 font-mono text-[11px] text-ink/50">{error}</p>}
        <Link href="/campus" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
          ← 返回市集
        </Link>
      </main>
    );
  }

  const cat = categoryOf(item.category);

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <div className="gap-10 lg:flex">
        {/* 左侧：单子内容 + 留言 */}
        <div className="min-w-0 flex-1">
          <div data-enter className="bg-blueprint relative flex h-44 items-center justify-center border border-ink/25">
            {item.images?.[0] ? (
              <Img src={item.images[0]} alt={item.title} label={cat.code} className="h-full w-full border-0" />
            ) : (
              <span className="font-display text-6xl font-bold tracking-widest text-ink/20">
                {cat.code}
              </span>
            )}
            <span
              className={cn(
                "absolute left-0 top-0 px-2 py-0.5 font-mono text-[10px] text-paper",
                item.type === "help" ? "bg-accent" : "bg-ink"
              )}
            >
              {item.type === "help" ? "求助单" : "闲置单"}
            </span>
            <span className="absolute right-3 top-3 font-mono text-[10px] tracking-widest text-ink/50">
              {STATUS_LABEL[item.status]}
            </span>
          </div>

          <h1 data-enter className="mt-6 font-display text-3xl font-bold leading-tight tracking-tight">
            {item.title}
          </h1>

          {/* 图集（第 2 张起） */}
          {item.images && item.images.length > 1 && (
            <div data-enter className={cn("mt-4 grid gap-2", item.images.length === 2 ? "grid-cols-2" : "grid-cols-3")}>
              {item.images.slice(1).map((src, i) => (
                <Img key={i} src={src} alt={`${item.title} 图 ${i + 2}`} label={`FIG.${i + 2}`} className="h-32 w-full" />
              ))}
            </div>
          )}
          <p data-enter className="mt-4 whitespace-pre-line text-sm leading-7 text-ink/80">
            {item.desc}
          </p>

          <div data-enter className="mt-6 grid grid-cols-2 gap-3 border-y border-line py-4 font-mono text-[11px] text-ink/60 md:grid-cols-3">
            <p>分类 · {cat.name}</p>
            <p>位置 · {item.place}</p>
            {item.deadline && <p className="text-accent">时限 · {item.deadline}</p>}
            <p>发布于 · {item.time}</p>
          </div>

          {/* 留言区（真实数据来自服务端；留言发布接口尚未接通） */}
          <section data-enter className="mt-10">
            <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
              MESSAGES / 留言 · {messages.length}
            </p>
            <ul className="mt-5 space-y-5">
              {messages.map((m) => (
                <li key={m.id} className="flex gap-3">
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center border border-ink/40 font-display text-xs font-bold">
                    {m.author.slice(0, 1)}
                  </span>
                  <div className="min-w-0">
                    <p className="font-mono text-[11px] text-ink/50">
                      {m.author}
                      <span className="mx-2">·</span>
                      {m.time}
                    </p>
                    <p className="mt-1 text-sm leading-6 text-ink/85">{m.text}</p>
                  </div>
                </li>
              ))}
            </ul>
            <div className="mt-6 border-t border-line pt-5">
              <p
                data-campus-message-state="unavailable"
                className="font-mono text-xs text-ink/50"
              >
                留言功能尚未接通，上线后即可咨询发单人。
              </p>
            </div>
          </section>
        </div>

        {/* 右侧栏 */}
        <aside className="mt-10 w-full shrink-0 lg:mt-0 lg:w-80">
          <div className="lg:sticky lg:top-20 lg:space-y-5">
            {/* 赏金卡 */}
            <div data-enter className="border border-ink p-6">
              <p className="font-mono text-[10px] tracking-[0.25em] text-ink/50">
                {item.type === "help" ? "BOUNTY / 赏金" : "PRICE / 一口价"}
              </p>
              <p className="mt-2 font-display text-5xl font-bold tabular-nums">
                <span className="text-accent">¥</span>
                {item.price}
              </p>
              <p
                data-campus-escrow-state="unavailable"
                className="mt-3 border border-dashed border-ink/30 px-2.5 py-1.5 font-mono text-[10px] tracking-widest text-ink/50"
              >
                接单与结算功能尚未接通，暂不涉及资金托管。
              </p>

              {item.status === "open" ? (
                <button
                  type="button"
                  disabled
                  className="mt-5 w-full cursor-not-allowed border border-line py-3 font-mono text-sm tracking-widest text-ink/40"
                >
                  {item.type === "help" ? "接单功能即将开放" : "想要功能即将开放"}
                </button>
              ) : (
                <p className="mt-5 border border-line py-3 text-center font-mono text-sm tracking-widest text-ink/40">
                  {item.status === "ongoing" ? "进行中 · 已被接" : item.status === "done" ? "已完成" : "已隐藏"}
                </p>
              )}

              <button
                ref={wantRef}
                type="button"
                disabled
                onClick={bounce}
                className="mt-3 w-full cursor-not-allowed border border-line py-2.5 font-mono text-xs tracking-widest text-ink/40"
              >
                ☆ 想要 · {item.wants}
              </button>
            </div>

            {/* 卖家信用卡 */}
            <div data-enter className="border border-ink/25 p-5">
              <div className="flex items-center gap-3">
                <span className="bg-blueprint flex h-11 w-11 items-center justify-center border border-ink font-display text-lg font-bold">
                  {item.seller.slice(0, 1)}
                </span>
                <div>
                  <p className="text-sm font-medium">{item.seller}</p>
                  <p className="font-mono text-[10px] text-ink/50">
                    成交 {item.dealsDone} 单
                  </p>
                </div>
                <div className="ml-auto text-right">
                  <p className="font-display text-2xl font-bold text-accent">{item.credit}</p>
                  <p className="font-mono text-[9px] tracking-widest text-ink/40">信用分</p>
                </div>
              </div>
            </div>
          </div>
        </aside>
      </div>
    </main>
  );
}
