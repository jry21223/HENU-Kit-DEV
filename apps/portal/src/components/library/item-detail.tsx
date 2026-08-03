"use client";

import Link from "next/link";
import { useState } from "react";
import {
  MATERIAL_TYPES,
  Material,
  STATIC_MATERIALS,
} from "@/lib/library/mock";
import MaterialCard from "@/components/library/material-card";
import { useReveal } from "@/components/account/use-reveal";

export default function ItemDetail({ id }: { id: string }) {
  const material = STATIC_MATERIALS.find((m) => m.id === id);
  useReveal();
  const [tocOpen, setTocOpen] = useState(false);

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

  const t = MATERIAL_TYPES[material.type];
  const free = material.price === 0;
  const related = STATIC_MATERIALS.filter(
    (m) => m.id !== id && (m.subject === material.subject || m.type === material.type)
  ).slice(0, 3);
  const toc = tocOpen ? material.toc : material.toc.slice(0, 6);

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <div className="gap-10 md:flex">
        {/* 封面 */}
        <div data-enter className="bg-blueprint relative flex h-72 w-full shrink-0 flex-col justify-between border border-ink p-5 md:w-64">
          <div className="flex items-start justify-between">
            <span className="font-mono text-[10px] tracking-[0.3em] text-ink/50">{t.code}</span>
            {free ? (
              <span className="bg-ink px-1.5 py-0.5 font-mono text-[10px] text-paper">免费</span>
            ) : (
              <span className="bg-accent px-1.5 py-0.5 font-mono text-[10px] text-paper">
                {material.price} 积分
              </span>
            )}
          </div>
          <div>
            <p className="font-display text-2xl font-bold leading-snug">{material.title}</p>
            <p className="mt-2 font-mono text-[10px] tracking-wider text-ink/50">
              {material.pageCount ?? material.pages.length} 页 · {material.subject}
            </p>
          </div>
        </div>

        {/* 元信息 + 操作 */}
        <div className="mt-8 min-w-0 flex-1 md:mt-0">
          <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">{t.code}</span>
            <span className="mx-2">/</span>
            {t.name} · {material.subject}
          </p>
          <h1 data-enter className="mt-3 font-display text-3xl font-bold tracking-tight md:text-4xl">
            {material.title}
          </h1>
          <p data-enter className="mt-3 font-mono text-[11px] tracking-wider text-ink/50">
            {material.author} · ★ {material.rating.toFixed(1)} · ↓ {material.downloads} · 收藏 {material.favs}
          </p>
          <p data-enter className="mt-2 font-mono text-[10px] tracking-wider text-ink/40">
            当前页面为示例资料，正式内容接入中
          </p>
          <p data-enter className="mt-5 max-w-xl text-sm leading-7 text-ink/75">
            {material.intro}
          </p>

          {/* 目录 */}
          <div data-enter className="mt-6 max-w-xl">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">CONTENTS / 目录</p>
            <ul className="mt-2 border-t border-line">
              {toc.map((c, i) => (
                <li key={i} className="flex gap-3 border-b border-line py-2 font-mono text-xs text-ink/70">
                  <span className="text-ink/30">{String(i + 1).padStart(2, "0")}</span>
                  {c}
                </li>
              ))}
            </ul>
            {material.toc.length > 6 && (
              <button
                type="button"
                onClick={() => setTocOpen((v) => !v)}
                className="mt-2 font-mono text-[10px] tracking-widest text-ink/50 hover:text-accent"
              >
                {tocOpen ? "收起 −" : `展开全部 ${material.toc.length} 节 +`}
              </button>
            )}
          </div>

          {/* 三形态操作条 */}
          <div data-enter className="mt-8 flex flex-wrap items-center gap-3">
            {free ? (
              <>
                <Link
                  href={`/library/read/${id}`}
                  className="border border-ink bg-ink px-7 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
                >
                  立即阅读 →
                </Link>
                <p className="border border-ink/30 px-6 py-3 font-mono text-sm tracking-widest text-ink/55">
                  下载即将开放
                </p>
              </>
            ) : (
              <>
                <Link
                  href={`/library/read/${id}`}
                  className="border border-ink/30 px-6 py-3 font-mono text-sm tracking-widest transition-colors hover:border-ink"
                >
                  免费试读 {material.previewPages} 页
                </Link>
                <p
                  data-library-purchase-state="unavailable"
                  className="border border-ink/30 px-6 py-3 font-mono text-sm tracking-widest text-ink/55"
                >
                  积分兑换暂未开放
                </p>
              </>
            )}
            <p
              data-library-favorite-state="unavailable"
              className="border border-ink/30 px-5 py-3 font-mono text-sm text-ink/55"
            >
              收藏功能即将上线
            </p>
          </div>
        </div>
      </div>

      {/* 相关推荐 */}
      <section data-enter className="mt-14">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">MORE / 同学也在看</p>
        <div className="mt-5 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {related.map((m: Material) => (
            <MaterialCard key={m.id} material={m} />
          ))}
        </div>
      </section>
    </main>
  );
}
