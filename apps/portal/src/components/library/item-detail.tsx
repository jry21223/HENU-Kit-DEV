"use client";

import { useState } from "react";
import type { Material } from "@/lib/library/mock";
import { MATERIAL_TYPES } from "@/lib/library/material-types";
import { getMaterials } from "@/lib/library/gateway";
import MaterialCard from "@/components/library/material-card";
import { useReveal } from "@/components/account/use-reveal";
import { LibraryLoading, LibraryNotFound, LibraryUnavailable } from "@/components/library/material-states";
import { useMaterialDetail } from "@/lib/library/use-material-detail";
import MaterialDownloadButton from "@/components/library/material-download-button";

export default function ItemDetail({ id }: { id: string }) {
  const state = useMaterialDetail(id);
  useReveal();
  const [tocOpen, setTocOpen] = useState(false);

  if (state.loadState !== "ready") {
    if (state.loadState === "loading") return <LibraryLoading />;
    if (state.loadState === "not-found") return <LibraryNotFound />;
    return <LibraryUnavailable message={state.error} onRetry={state.retry} />;
  }
  const { material } = state;

  const t = MATERIAL_TYPES[material.type];
  const free = material.price === 0;
  const related = getMaterials().filter(
    (m) => m.id !== id && (m.subject === material.subject || m.type === material.type)
  ).slice(0, 3);
  const toc = tocOpen ? material.toc : material.toc.slice(0, 6);

  const meta =
    material.fileSize
      ? `${formatBytes(material.fileSize)} · ${material.subject}`
      : `${material.pageCount ?? material.pages.length} 页 · ${material.subject}`;

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
            <p className="mt-2 font-mono text-[10px] tracking-wider text-ink/50">{meta}</p>
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
            {material.author}
            {material.rating !== undefined ? ` · ★ ${material.rating.toFixed(1)}` : ""}
            {` · ↓ ${material.downloads}`}
            {material.favs !== undefined ? ` · 收藏 ${material.favs}` : ""}
          </p>
          {material.intro && (
            <p data-enter className="mt-5 max-w-xl text-sm leading-7 text-ink/75">
              {material.intro}
            </p>
          )}

          {/* 目录 */}
          {material.toc.length > 0 && (
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
          )}

          {/* 操作条 */}
          <div data-enter className="mt-8 flex flex-wrap items-center gap-3">
            {free ? (
              !material.downloadAvailable ? (
                <p className="border border-ink/30 px-6 py-3 font-mono text-sm tracking-widest text-ink/55">
                  下载即将开放
                </p>
              ) : null
            ) : (
                <p
                  data-library-purchase-state="unavailable"
                  className="border border-ink/30 px-6 py-3 font-mono text-sm tracking-widest text-ink/55"
                >
                  积分兑换暂未开放
                </p>
            )}
            {free && material.downloadAvailable && (
              <MaterialDownloadButton
                materialId={material.id}
                label={`下载资料 ↓${material.fileSize ? `（${formatBytes(material.fileSize)}）` : ""}`}
                className="border border-ink bg-ink px-7 py-3 font-mono text-sm tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent disabled:cursor-wait disabled:opacity-60"
              />
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
      {related.length > 0 && (
        <section data-enter className="mt-14">
          <p className="font-mono text-xs tracking-[0.25em] text-ink/60">MORE / 同学也在看</p>
          <div className="mt-5 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {related.map((m: Material) => (
              <MaterialCard key={m.id} material={m} />
            ))}
          </div>
        </section>
      )}
    </main>
  );
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}
