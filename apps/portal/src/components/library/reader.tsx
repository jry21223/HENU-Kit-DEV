"use client";

import Link from "next/link";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import {
  MATERIAL_TYPES,
  getMaterial,
} from "@/lib/library/mock";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const subscribeToHydration = () => () => {};
const hydratedSnapshot = () => true;
const serverHydrationSnapshot = () => false;

export default function Reader({ id }: { id: string }) {
  const material = getMaterial(id);

  const [page, setPage] = useState(0);
  const isInteractive = useSyncExternalStore(
    subscribeToHydration,
    hydratedSnapshot,
    serverHydrationSnapshot
  );
  const dirRef = useRef(1);
  const contentRef = useRef<HTMLDivElement>(null);

  const total = material?.pageCount ?? material?.pages.length ?? 0;
  const free = !material || material.price === 0;
  const visiblePages = material && !free
    ? Math.min(material.previewPages, material.pages.length)
    : total;
  const locked = !free && page >= visiblePages;
  const previousDisabled = !isInteractive || page === 0;
  const nextDisabled = !isInteractive || page === total - 1;

  const movePage = (nextForCurrentPage: (current: number) => number) => {
    setPage((current) => {
      const next = Math.max(0, Math.min(total - 1, nextForCurrentPage(current)));
      if (next === current) return current;
      dirRef.current = next > current ? 1 : -1;
      return next;
    });
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
              本文共 {total} 页，前 {visiblePages} 页已免费试读。
              全文需要积分兑换，兑换功能即将开放。
            </p>
            <p className="mt-6 font-display text-4xl font-bold">
              {material.price}
              <span className="ml-1 font-mono text-xs font-normal text-ink/50">积分</span>
            </p>
            <p
              data-library-purchase-state="unavailable"
              className="mt-6 border border-ink/30 px-8 py-3 font-mono text-sm tracking-widest text-ink/55"
            >
              积分兑换暂未开放
            </p>
          </div>
        ) : (
          /* 正文页 */
          <article>
            <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
              PAGE {String(page + 1).padStart(2, "0")}
              {!free && (
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
          onClick={() => movePage((current) => current - 1)}
          disabled={previousDisabled}
          className={cn(
            "border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
            previousDisabled ? "cursor-not-allowed border-line text-ink/30" : "border-ink/40 hover:border-ink"
          )}
        >
          ← 上一页
        </button>
        <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">← / → 键翻页</p>
        <button
          type="button"
          onClick={() => movePage((current) => current + 1)}
          disabled={nextDisabled}
          className={cn(
            "border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
            nextDisabled
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
