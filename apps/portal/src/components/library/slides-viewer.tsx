"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type { Material } from "@/lib/library/mock";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";
import MaterialDownloadButton from "@/components/library/material-download-button";

/**
 * 课件幻灯片查看器:PPT 转换后的结构化页面,支持 ←/→ 键与按钮翻页。
 * 与 reader.tsx 同一套视觉语言;slides 为空时降级为下载原文件。
 */
export default function SlidesViewer({ material }: { material: Material }) {
  const slides = material.slides ?? [];
  const [index, setIndex] = useState(0);
  const contentRef = useRef<HTMLDivElement>(null);
  const dirRef = useRef(1);

  const total = slides.length;
  const slide = slides[index];

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight") {
        dirRef.current = 1;
        setIndex((i) => Math.min(total - 1, i + 1));
      } else if (e.key === "ArrowLeft") {
        dirRef.current = -1;
        setIndex((i) => Math.max(0, i - 1));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [total]);

  useEffect(() => {
    const el = contentRef.current;
    if (!el || window.matchMedia(REDUCED_MOTION).matches) return;
    gsap.fromTo(
      el,
      { x: 40 * dirRef.current, autoAlpha: 0 },
      { x: 0, autoAlpha: 1, duration: 0.25, ease: "power2.out", clearProps: "all" }
    );
  }, [index]);

  const previousDisabled = index === 0;
  const nextDisabled = index >= total - 1;

  if (total === 0) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
          本课件尚未完成幻灯片转换
        </p>
        <p className="mt-4 text-sm leading-7 text-ink/70">
          {material.downloadAvailable
            ? "可直接下载原文件查看完整内容。"
            : "原文件暂时无法下载，请返回详情查看其他内容。"}
        </p>
        {material.price === 0 && material.downloadAvailable && (
          <MaterialDownloadButton
            materialId={material.id}
            label="下载原文件 ↓"
            className="mt-6 border border-ink bg-ink px-7 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
          />
        )}
        <Link
          href={`/library/item/${material.id}`}
          className="mt-6 block font-mono text-sm text-accent hover:underline"
        >
          ← 返回详情
        </Link>
      </main>
    );
  }

  const progress = ((index + 1) / total) * 100;

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-6 md:px-8">
      <div className="max-w-[70ch]">
        {/* 顶栏 */}
        <div className="flex items-center gap-4 border-b border-line pb-3">
          <Link
            href={`/library/item/${material.id}`}
            className="font-mono text-xs text-ink/60 hover:text-accent"
          >
            ← 详情
          </Link>
          <p className="min-w-0 flex-1 truncate font-mono text-xs text-ink/70">
            {material.title}
            <span className="ml-2 text-ink/40">{material.subject}</span>
          </p>
          <p className="shrink-0 font-mono text-xs tabular-nums">
            {index + 1} / {total}
          </p>
        </div>
        <div className="h-0.5 w-full bg-ink/10">
          <div
            className="h-full bg-accent transition-[width] duration-300"
            style={{ width: `${progress}%` }}
          />
        </div>

        {/* 当前页 */}
        <div ref={contentRef} className="min-h-[50vh] py-10">
          <article>
            <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
              SLIDE {String(index + 1).padStart(2, "0")}
            </p>
            <h1 className="mt-4 font-display text-2xl font-bold leading-snug">
              {slide.title}
            </h1>
            <div className="mt-6 space-y-5">
              {(slide.blocks ?? []).map((para, i) => (
                <p key={i} className="text-[15px] leading-8 text-ink/85">
                  {para}
                </p>
              ))}
              {(slide.blocks ?? []).length === 0 && (
                <p className="font-mono text-xs text-ink/40">（本页无文字内容）</p>
              )}
            </div>
          </article>
        </div>

        {/* 底部翻页 + 下载 */}
        <div className="flex items-center justify-between border-t border-line pt-4">
          <button
            type="button"
            onClick={() => {
              dirRef.current = -1;
              setIndex((i) => Math.max(0, i - 1));
            }}
            disabled={previousDisabled}
            className={cn(
              "border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
              previousDisabled
                ? "cursor-not-allowed border-line text-ink/30"
                : "border-ink/40 hover:border-ink"
            )}
          >
            ← 上一页
          </button>
          <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">← / → 键翻页</p>
          <button
            type="button"
            onClick={() => {
              dirRef.current = 1;
              setIndex((i) => Math.min(total - 1, i + 1));
            }}
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

        {material.price === 0 && material.downloadAvailable && (
          <div className="mt-4 flex justify-end">
            <MaterialDownloadButton
              materialId={material.id}
              label={`下载原文件 ↓${material.fileSize ? `（${formatBytes(material.fileSize)}）` : ""}`}
              className="font-mono text-xs text-ink/50 underline-offset-4 hover:text-accent hover:underline"
            />
          </div>
        )}
      </div>
    </main>
  );
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}
