"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { campusStore, categoryOf } from "@/lib/campus/mock";
import Img from "@/components/ui/img";
import { useReveal } from "@/components/account/use-reveal";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const STATUS_LABEL = { open: "待接单", ongoing: "进行中", done: "已完成", hidden: "已隐藏" } as const;
const FLOW_STEPS = ["发布", "赏金托管", "接单服务", "确认完成", "平台结算"];

/** 担保流程条（SVG 五步描线，当前步橙色） */
function EscrowFlow({ current }: { current: number }) {
  return (
    <svg viewBox="0 0 480 84" fill="none" className="w-full">
      {FLOW_STEPS.map((_, i) =>
        i < FLOW_STEPS.length - 1 ? (
          <line
            key={i}
            x1={48 + i * 96 + 22}
            y1={30}
            x2={48 + (i + 1) * 96 - 22}
            y2={30}
            className={i < current - 1 ? "stroke-ink" : "stroke-line"}
            strokeWidth="1.5"
            strokeDasharray={i < current - 1 ? undefined : "4 5"}
          />
        ) : null
      )}
      {FLOW_STEPS.map((label, i) => {
        const active = i < current;
        const isNow = i === current - 1;
        return (
          <g key={label}>
            <rect
              x={48 + i * 96 - 22}
              y={16}
              width={44}
              height={28}
              className={cn(
                isNow ? "stroke-accent fill-accent/10" : active ? "stroke-ink" : "stroke-line"
              )}
              strokeWidth="1.5"
            />
            <text
              x={48 + i * 96}
              y={34}
              textAnchor="middle"
              fontSize="10"
              className={cn(isNow ? "fill-accent" : active ? "fill-ink" : "fill-ink/40")}
            >
              {String(i + 1).padStart(2, "0")}
            </text>
            <text
              x={48 + i * 96}
              y={66}
              textAnchor="middle"
              fontSize="9"
              className={isNow ? "fill-accent" : "fill-ink/50"}
            >
              {label}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

export default function ItemDetail({ id }: { id: string }) {
  const router = useRouter();
  const data = useSyncExternalStore(campusStore.subscribe, campusStore.get, campusStore.getServer);
  const { user } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  useReveal();
  const wantRef = useRef<HTMLButtonElement>(null);
  const [draft, setDraft] = useState("");

  const item = data.items.find((i) => i.id === id);
  if (!item) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
        <p className="mt-4 font-display text-2xl font-bold">单子不存在或已删除</p>
        <Link href="/campus" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
          ← 返回市集
        </Link>
      </main>
    );
  }

  const cat = categoryOf(item.category);
  const messages = data.messages.filter((m) => m.itemId === id);
  const flowStep = item.status === "open" ? 2 : item.status === "ongoing" ? 3 : item.status === "done" ? 5 : 1;

  const bounce = () => {
    if (window.matchMedia(REDUCED_MOTION).matches) return;
    gsap.fromTo(wantRef.current, { scale: 1 }, { scale: 1.15, duration: 0.15, yoyo: true, repeat: 1 });
  };

  const onAccept = () => {
    if (!user) {
      router.push(`/account/login?next=/campus/item/${id}`);
      return;
    }
    campusStore.accept(id);
  };

  const onMessage = () => {
    if (!user || !draft.trim()) return;
    campusStore.addMessage(id, user.name, draft.trim());
    setDraft("");
  };

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

          {/* 留言区 */}
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
              {user ? (
                <div className="flex gap-3">
                  <input
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    placeholder="公开留言，咨询细节…"
                    className="flex-1 border-b border-ink/30 bg-transparent py-2 text-sm outline-none placeholder:text-ink/30 focus:border-ink"
                  />
                  <button
                    type="button"
                    onClick={onMessage}
                    disabled={!draft.trim()}
                    className={cn(
                      "border px-5 py-2 font-mono text-xs tracking-widest transition-colors",
                      draft.trim()
                        ? "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                        : "cursor-not-allowed border-line text-ink/30"
                    )}
                  >
                    留言
                  </button>
                </div>
              ) : (
                <p className="font-mono text-xs text-ink/50">
                  <Link href={`/account/login?next=/campus/item/${id}`} className="text-accent hover:underline">
                    登录
                  </Link>{" "}
                  后留言咨询
                </p>
              )}
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
              <p className="mt-3 flex items-center gap-2 border border-accent px-2.5 py-1.5 font-mono text-[10px] tracking-widest text-accent">
                <span aria-hidden>▣</span> 平台担保 · 确认完成才结算
              </p>

              {item.isMine ? (
                <Link
                  href="/campus/deals"
                  className="mt-5 block border border-ink/30 py-3 text-center font-mono text-sm tracking-widest transition-colors hover:border-ink"
                >
                  我发布的 · 去管理 →
                </Link>
              ) : item.status === "open" ? (
                <button
                  type="button"
                  onClick={onAccept}
                  className="mt-5 w-full border border-ink bg-ink py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
                >
                  {item.type === "help" ? "我要接单" : "我想要"}
                </button>
              ) : (
                <p className="mt-5 border border-line py-3 text-center font-mono text-sm tracking-widest text-ink/40">
                  {item.status === "ongoing" ? "进行中 · 已被接" : item.status === "done" ? "已完成" : "已隐藏"}
                </p>
              )}

              <button
                ref={wantRef}
                type="button"
                onClick={() => {
                  campusStore.toggleWant(id);
                  if (!item.wanted) bounce();
                }}
                className={cn(
                  "mt-3 w-full border py-2.5 font-mono text-xs tracking-widest transition-colors",
                  item.wanted ? "border-accent text-accent" : "border-ink/30 hover:border-ink"
                )}
              >
                {item.wanted ? "★ 已想要" : "☆ 想要"} · {item.wants}
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

            {/* 担保流程 */}
            <div data-enter className="border border-ink/25 p-5">
              <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
                ESCROW / 担保流程
              </p>
              <div className="mt-4">
                <EscrowFlow current={flowStep} />
              </div>
            </div>
          </div>
        </aside>
      </div>
    </main>
  );
}
