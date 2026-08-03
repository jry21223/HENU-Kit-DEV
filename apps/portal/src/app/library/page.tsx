"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  formatPortalError,
  fetchLibraryMaterials,
  mockAllowed,
} from "@/lib/api/client";
import type { Material as ApiMaterial } from "@/lib/api/types";
import {
  MATERIAL_TYPES,
  MaterialType,
  STATIC_MATERIALS,
  type Material,
} from "@/lib/library/mock";
import {
  getLibraryGatewayError,
  getMaterials,
  initGateway,
} from "@/lib/library/gateway";
import MaterialCard from "@/components/library/material-card";
import SubHero from "@/components/site-hero/sub-hero";
import { SceneBooks } from "@/components/site-hero/scenes";
import { useReveal } from "@/components/account/use-reveal";
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
    pages: m.pages ?? [],
    price: m.price,
    previewPages: m.previewPages,
    rating: m.rating,
    downloads: m.downloads,
    favs: m.favs,
  };
}

type LoadState = "loading" | "ready" | "error";

export default function LibraryHomePage() {
  const [query, setQuery] = useState("");
  const [type, setType] = useState<MaterialType | "all">("all");
  const [price, setPrice] = useState<"all" | "free" | "paid">("all");
  const [subject, setSubject] = useState("all");
  const [materials, setMaterials] = useState<Material[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  useReveal();

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      // Direct list endpoint first (portal-api materials).
      const resp = await fetchLibraryMaterials();
      setMaterials(resp.materials.map(toMaterial));
      setLoadState("ready");
      return;
    } catch (e) {
      // Fall through to gateway cache / mock (dev only).
      try {
        await initGateway();
        const cached = getMaterials();
        if (cached.length > 0) {
          setMaterials(cached);
          setLoadState("ready");
          return;
        }
        if (mockAllowed) {
          setMaterials(STATIC_MATERIALS);
          setLoadState("ready");
          return;
        }
        setMaterials([]);
        setError(
          getLibraryGatewayError() ||
            formatPortalError(e) ||
            "资料库暂时加载不出来，请稍后刷新试试。"
        );
        setLoadState("error");
      } catch (e2) {
        setMaterials([]);
        setError(formatPortalError(e2));
        setLoadState("error");
      }
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

  const totalDownloads = materials.reduce((s, m) => s + m.downloads, 0);

  return (
    <main>
      <SubHero
        index="01"
        en="LIBRARY"
        title="资料库"
        slogan="学长笔记、往年试卷、模拟卷、学习路径、实验报告——免费资料可直接阅读，收费资料可试读，积分兑换将在真实账户服务接通后开放。"
        counters={[
          { label: "收录资料", value: materials.length },
          { label: "累计下载", value: totalDownloads },
        ]}
        fig="FIG.02 书脊 / SPINES"
        scene={<SceneBooks />}
      />

      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}

        {/* 搜索 + 筛选行 */}
        <div data-enter className="flex flex-wrap items-center gap-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索：真题 / 高数 / 实验报告"
            className="w-56 border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-accent"
          />
          <span aria-hidden className="hidden h-4 w-px bg-ink/20 sm:block" />
          {(["all", ...TYPE_KEYS] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setType(t)}
              className={cn(
                "border px-3 py-1.5 font-mono text-xs transition-colors",
                type === t ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
              )}
            >
              {t === "all" ? "全部" : MATERIAL_TYPES[t].name}
            </button>
          ))}
          <span aria-hidden className="hidden h-4 w-px bg-ink/20 sm:block" />
          {(["all", "free", "paid"] as const).map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setPrice(p)}
              className={cn(
                "border px-3 py-1.5 font-mono text-xs transition-colors",
                price === p ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
              )}
            >
              {p === "all" ? "全部" : p === "free" ? "免费" : "收费"}
            </button>
          ))}
          <select
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            className="border border-line bg-paper px-3 py-1.5 font-mono text-xs text-ink/70 outline-none focus:border-ink"
          >
            <option value="all">全部科目</option>
            {subjects.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>

        {/* 书架网格 */}
        <div data-enter className="mt-8">
          {loadState === "loading" ? (
            <LoadingBlock label="加载资料" />
          ) : loadState === "error" ? (
            <EmptyBlock label="内容暂时加载不出来，请稍后刷新试试" />
          ) : items.length === 0 ? (
            <EmptyBlock label="无匹配资料" />
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
