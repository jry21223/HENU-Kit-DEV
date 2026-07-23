"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useSyncExternalStore } from "react";
import {
  MATERIAL_TYPES,
  getMaterial,
  libraryStore,
} from "@/lib/library/mock";
import { usePurchase } from "@/components/library/use-purchase";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

export default function Reader({ id }: { id: string }) {
  const material = getMaterial(id);
  const lib = useSyncExternalStore(libraryStore.subscribe, libraryStore.get, libraryStore.getServer);
  const { buy, balance, user } = usePurchase();

  const [page, setPage] = useState(0);
  const [msg, setMsg] = useState("");
  const dirRef = useRef(1);
  const contentRef = useRef<HTMLDivElement>(null);

  const total = material?.pages.length ?? 0;
  const free = !material || material.price === 0;
  const owned = free || lib.owned.includes(id);
  const locked = !free && !owned && material ? page >= material.previewPages : false;

  const goto = (next: number) => {
    const clamped = Math.max(0, Math.min(total - 1, next));
    if (clamped === page) return;
    dirRef.current = clamped > page ? 1 : -1;
    setPage(clamped);
  };

  // 键盘翻页
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight") {
        dirRef.current = 1;
        setPage((p) => Math.min(total - 1, p + 1));
      } else if (e.key === "ArrowLeft") {
        dirRef.current = -1;
        setPage((p) => Math.max(0, p - 1));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [total]);

  // 翻页横滑（reduced-motion 直切）
  useEffect(() => {
    const el = contentRef.current;
    if (!el || window.matchMedia(REDUCED_MOTION).matches) return;
    gsap.fromTo(
      el,
      { x: 40 * dirRef.current, autoAlpha: 0 },
      { x: 0, autoAlpha: 1, duration: 0.3, ease: "power2.out", clearProps: "all" }
    );
  }, [page]);

  if (!material) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
        <Link href="/library" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
          ← 返回书库
        </Link>
      </main>
    );
  }

  const onBuy = () => {
    const r = buy(id, `/library/read/${id}`);
    if (r === "ok") setMsg("");
    else if (r === "no-points") setMsg("积分不足，请先去钱包签到攒积分");
  };

  const progress = ((page + 1) / total) * 100;

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-6 md:px-8">
      <div className="max-w-[70ch]">
      {/* 顶栏 */}
      <div className="flex items-center gap-4 border-b border-line pb-3">
        <Link href={`/library/item/${id}`} className="font-mono text-xs text-ink/60 hover:text-accent">
          ← 详情
        </Link>
        <p className="min-w-0 flex-1 truncate font-mono text-xs text-ink/70">
          {material.title}
          <span className="ml-2 text-ink/40">{MATERIAL_TYPES[material.type].name}</span>
        </p>
        <p className="shrink-0 font-mono text-xs tabular-nums">
          {page + 1} / {total}
        </p>
      </div>
      <div className="h-0.5 w-full bg-ink/10">
        <div className="h-full bg-accent transition-[width] duration-300" style={{ width: `${progress}%` }} />
      </div>

      {/* 内容区 */}
      <div ref={contentRef} className="min-h-[50vh] py-10">
        {locked ? (
          /* 锁定墙 */
          <div className="flex flex-col items-center border border-dashed border-ink/40 px-6 py-16 text-center">
            <span aria-hidden className="font-mono text-2xl text-accent">▣</span>
            <p className="mt-4 font-mono text-xs tracking-[0.3em] text-ink/50">
              试读到此为止 · PREVIEW ENDS
            </p>
            <p className="mt-3 max-w-sm text-sm leading-7 text-ink/70">
              本文共 {total} 页，前 {material.previewPages} 页已免费试读。
              购买后可阅读全文。
            </p>
            <p className="mt-6 font-display text-4xl font-bold">
              {material.price}
              <span className="ml-1 font-mono text-xs font-normal text-ink/50">积分</span>
            </p>
            <p className="mt-1 font-mono text-[10px] text-ink/50">
              当前余额：{user ? balance : "未登录"}
            </p>
            <button
              type="button"
              onClick={onBuy}
              className="mt-6 border border-ink bg-ink px-8 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
            >
              {user ? "积分购买 · 解锁全文" : "登录后购买"}
            </button>
            {msg && (
              <p className="mt-3 font-mono text-xs text-accent">
                {msg}
                <Link href="/account/wallet" className="ml-2 underline">去钱包 →</Link>
              </p>
            )}
          </div>
        ) : (
          /* 正文页 */
          <article>
            <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
              PAGE {String(page + 1).padStart(2, "0")}
              {!free && !owned && (
                <span className="ml-3 border border-accent/60 px-1.5 py-0.5 text-accent">试读页</span>
              )}
            </p>
            <div className="mt-6 space-y-5">
              {material.pages[page].map((para, i) => (
                <p key={i} className="text-[15px] leading-8 text-ink/85">
                  {para}
                </p>
              ))}
            </div>
          </article>
        )}
      </div>

      {/* 底部翻页 */}
      <div className="flex items-center justify-between border-t border-line pt-4">
        <button
          type="button"
          onClick={() => goto(page - 1)}
          disabled={page === 0}
          className={cn(
            "border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
            page === 0 ? "cursor-not-allowed border-line text-ink/30" : "border-ink/40 hover:border-ink"
          )}
        >
          ← 上一页
        </button>
        <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">← / → 键翻页</p>
        <button
          type="button"
          onClick={() => goto(page + 1)}
          disabled={page === total - 1}
          className={cn(
            "border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
            page === total - 1
              ? "cursor-not-allowed border-line text-ink/30"
              : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
          )}
        >
          下一页 →
        </button>
      </div>
      </div>
    </main>
  );
}
