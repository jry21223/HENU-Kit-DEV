"use client";

import { useState } from "react";
import { useSyncExternalStore } from "react";
import {
  MATERIAL_TYPES,
  MaterialType,
  STATIC_MATERIALS,
  libraryStore,
} from "@/lib/library/mock";
import MaterialCard from "@/components/library/material-card";
import SubHero from "@/components/site-hero/sub-hero";
import { SceneBooks } from "@/components/site-hero/scenes";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

const TYPE_KEYS = Object.keys(MATERIAL_TYPES) as MaterialType[];
const SUBJECTS = Array.from(new Set(STATIC_MATERIALS.map((m) => m.subject)));

export default function LibraryHomePage() {
  useSyncExternalStore(libraryStore.subscribe, libraryStore.get, libraryStore.getServer);
  const [query, setQuery] = useState("");
  const [type, setType] = useState<MaterialType | "all">("all");
  const [price, setPrice] = useState<"all" | "free" | "paid">("all");
  const [subject, setSubject] = useState("all");
  useReveal();

  const items = STATIC_MATERIALS.filter(
    (m) =>
      (type === "all" || m.type === type) &&
      (price === "all" || (price === "free" ? m.price === 0 : m.price > 0)) &&
      (subject === "all" || m.subject === subject) &&
      (!query.trim() || m.title.includes(query.trim()) || m.subject.includes(query.trim()))
  );

  const totalDownloads = STATIC_MATERIALS.reduce((s, m) => s + m.downloads, 0);

  return (
    <main>
      <SubHero
        index="01"
        en="LIBRARY"
        title="资料库"
        slogan="学长笔记、往年试卷、模拟卷、学习路径、实验报告——免费的尽管拿，收费的先用积分试读。"
        counters={[
          { label: "收录资料", value: STATIC_MATERIALS.length },
          { label: "累计下载", value: totalDownloads },
        ]}
        fig="FIG.02 书脊 / SPINES"
        scene={<SceneBooks />}
      />

      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
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
            {SUBJECTS.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>

        {/* 书架网格 */}
        <div data-enter className="mt-8">
          {items.length === 0 ? (
            <p className="border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
              无匹配资料 / NO RESULT
            </p>
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
