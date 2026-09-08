"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  fetchLibraryMaterials,
} from "@/lib/api/client";
import type { Material as ApiMaterial } from "@/lib/api/types";
import type { Material } from "@/lib/library/mock";
import { MATERIAL_TYPES, type MaterialType } from "@/lib/library/material-types";
import MaterialCard from "@/components/library/material-card";
import SubHero from "@/components/site-hero/sub-hero";
import { SceneBooks } from "@/components/site-hero/scenes";
import { useReveal } from "@/components/account/use-reveal";
import { useScrollRestoration } from "@/components/use-scroll-restoration";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { cn } from "@/lib/cn";

const TYPE_KEYS = Object.keys(MATERIAL_TYPES) as MaterialType[];

function toMaterial(m: ApiMaterial): Material {
  return {
    id: m.id,
    type: m.type,
    subject: m.subject,
    title: m.title,
    author: m.author,
    intro: m.intro,
    toc: m.toc ?? [],
    pages: [],
    price: m.price,
    previewPages: 0,
    rating: m.rating,
    downloads: m.downloads,
    favs: m.favs,
    downloadAvailable: m.downloadAvailable,
  };
}

type LoadState = "loading" | "ready" | "error";
type LibraryStatistics = {
  materialCount: number;
  downloadStarts: number;
};

export default function LibraryHomePage() {
  const [query, setQuery] = useState("");
  const [type, setType] = useState<MaterialType | "all">("all");
  const [price, setPrice] = useState<"all" | "free" | "paid">("all");
  const [subject, setSubject] = useState("all");
  const [materials, setMaterials] = useState<Material[]>([]);
  const [statistics, setStatistics] = useState<LibraryStatistics | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  useReveal();
  useScrollRestoration(loadState === "ready");

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    setMaterials([]);
    setStatistics(null);
    try {
      const resp = await fetchLibraryMaterials();
      if (
        resp.statistics.materialCount !== resp.materials.length ||
        resp.statistics.materialCount < 0 ||
        resp.statistics.downloadStarts < 0
      ) {
        throw new Error("资料库返回了不一致的目录统计，请稍后重试。");
      }
      setMaterials(resp.materials.map(toMaterial));
      setStatistics({
        materialCount: resp.statistics.materialCount,
        downloadStarts: resp.statistics.downloadStarts,
      });
      setLoadState("ready");
      return;
    } catch {
      setError("资料库暂时无法加载，请稍后重试。");
      setLoadState("error");
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const subjects = useMemo(
    () => Array.from(new Set(materials.map((m) => m.subject))),
    [materials]
  );

  const items = materials.filter(
    (m) =>
      (type === "all" || m.type === type) &&
      (price === "all" || (price === "free" ? m.price === 0 : m.price > 0)) &&
      (subject === "all" || m.subject === subject) &&
      (!query.trim() ||
        m.title.includes(query.trim()) ||
        m.subject.includes(query.trim()))
  );
  const hasActiveFilter = query.trim() !== "" || type !== "all" || price !== "all" || subject !== "all";
  const hasElectronicTextbooks = materials.some((material) => material.type === "textbook");
  const emptyLabel =
    materials.length === 0 && !hasActiveFilter
      ? "资料库当前暂无公开资料"
      : type === "textbook" && !hasElectronicTextbooks
        ? "电子版教材暂未收录；通过公开资料审核后会在此展示"
        : "无匹配资料";

  return (
    <main>
      <SubHero
        index="01"
        en="LIBRARY"
        title="资料库"
        slogan="公开免费资料，可按科目和类型浏览；累计下载从本资料库启用下载后开始统计。"
        counters={[
          {
            label: "收录资料",
            value: loadState === "ready" ? statistics?.materialCount ?? null : null,
            busy: loadState === "loading",
          },
          {
            label: "累计下载",
            value: loadState === "ready" ? statistics?.downloadStarts ?? null : null,
            busy: loadState === "loading",
          },
        ]}
        fig="FIG.02 书脊 / SPINES"
        scene={<SceneBooks />}
        compactOnMobile
      />

      <div className="mx-auto max-w-[1440px] px-5 py-6 md:px-8 lg:py-10">
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}

        {/* 搜索 + 筛选行 */}
        <div data-enter role="search" aria-label="资料搜索与筛选" className="space-y-4">
          <div className="flex max-w-3xl items-end gap-3">
            <div className="min-w-0 flex-1">
              <label htmlFor="library-query" className="mb-1 block font-mono text-xs text-ink/70">搜索资料</label>
              <input
                id="library-query"
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索：真题 / 高数 / 课件"
                className="min-h-11 w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-accent"
              />
            </div>
            <div className="max-w-[45%]">
              <label htmlFor="library-subject" className="mb-1 block font-mono text-xs text-ink/70">科目</label>
              <select
                id="library-subject"
                aria-label="按科目筛选"
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
                className="min-h-11 w-full border border-line bg-paper px-3 py-2 font-mono text-xs text-ink/70 outline-none focus:border-ink"
              >
                <option value="all">全部科目</option>
                {subjects.map((s) => (
                  <option key={s} value={s}>{s}</option>
                ))}
              </select>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <div role="group" aria-label="资料类型" className="flex flex-wrap gap-2">
              {(["all", ...TYPE_KEYS] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setType(t)}
                  aria-pressed={type === t}
                  className={cn(
                    "min-h-11 min-w-11 border px-3 py-1.5 font-mono text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                    type === t ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                  )}
                >
                  {t === "all" ? "全部" : MATERIAL_TYPES[t].name}
                </button>
              ))}
            </div>
            <div role="group" aria-label="资料价格" className="flex flex-wrap gap-2">
              {(["all", "free", "paid"] as const).map((p) => (
                <button
                  key={p}
                  type="button"
                  onClick={() => setPrice(p)}
                  aria-pressed={price === p}
                  className={cn(
                    "min-h-11 min-w-11 border px-3 py-1.5 font-mono text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                    price === p ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                  )}
                >
                  {p === "all" ? "全部" : p === "free" ? "免费" : "收费"}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* 书架网格 */}
        <div data-enter className="mt-8">
          {loadState === "loading" ? (
            <LoadingBlock label="加载资料" />
          ) : loadState === "error" ? (
            <EmptyBlock label="内容暂时加载不出来，请稍后刷新试试" />
          ) : items.length === 0 ? (
            <EmptyBlock label={emptyLabel} />
          ) : (
            <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
              {items.map((m) => (
                <MaterialCard key={m.id} material={m} />
              ))}
            </div>
          )}
        </div>
      </div>
    </main>
  );
}
