"use client";

import { useEffect, useState } from "react";
import { useSyncExternalStore } from "react";
import { CATEGORIES, campusStore, ItemType } from "@/lib/campus/mock";
import { hasGateway } from "@/lib/api/client";
import { getItems, getCategories } from "@/lib/campus/gateway";
import ItemCard from "@/components/campus/item-card";
import SubHero from "@/components/site-hero/sub-hero";
import { SceneHandshake } from "@/components/site-hero/scenes";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function MarketPage() {
  const data = useSyncExternalStore(campusStore.subscribe, campusStore.get, campusStore.getServer);
  const [campusItems, setCampusItems] = useState(data.items);
  const [categories, setCategories] = useState(CATEGORIES);
  const [query, setQuery] = useState("");
  const [cat, setCat] = useState<string>("all");
  const [type, setType] = useState<ItemType | "all">("all");
  useReveal();

  useEffect(() => {
    if (!hasGateway) return;
    let cancelled = false;
    Promise.all([getItems(), getCategories()]).then(([itemsResp, catsResp]) => {
      if (cancelled) return;
      if (itemsResp) setCampusItems(itemsResp as typeof data.items);
      if (catsResp) setCategories(catsResp as typeof CATEGORIES);
    });
    return () => { cancelled = true; };
  }, []);

  const items = campusItems.filter(
    (it) =>
      it.status !== "hidden" &&
      (cat === "all" || it.category === cat) &&
      (type === "all" || it.type === type) &&
      (!query.trim() || it.title.includes(query.trim()))
  );
  const openCount = campusItems.filter((i) => i.status !== "hidden").length;
  const doneCount = campusItems.filter((i) => i.status === "done").length + data.deals.filter((d) => d.status === "done").length;

  return (
    <main>
      <SubHero
        index="04"
        en="CAMPUS MARKET"
        title="互助平台"
        slogan="代取快递、搬行李、小项目、出闲置——发单有人接，赏金平台托管，完成才结算。"
        counters={[
          { label: "在架单子", value: openCount },
          { label: "累计成交", value: doneCount },
        ]}
        fig="FIG.04 交接 / HANDOVER"
        scene={<SceneHandshake />}
      />

      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
        {/* 搜索 + 分类 + 类型 */}
        <div data-enter className="flex flex-wrap items-center gap-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索：快递 / 键盘 / 占座"
            className="w-52 border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-accent"
          />
          <span aria-hidden className="hidden h-4 w-px bg-ink/20 sm:block" />
          {(["all", "help", "sell"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setType(t)}
              className={cn(
                "border px-3 py-1.5 font-mono text-xs transition-colors",
                type === t
                  ? t === "help"
                    ? "border-accent bg-accent text-paper"
                    : "border-ink bg-ink text-paper"
                  : "border-line text-ink/60 hover:border-ink/40"
              )}
            >
              {t === "all" ? "全部" : t === "help" ? "求助单" : "闲置单"}
            </button>
          ))}
          <button
            type="button"
            onClick={() => setCat("all")}
            className={cn(
              "border px-3 py-1.5 font-mono text-xs transition-colors",
              cat === "all" ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
            )}
          >
            全部
          </button>
          {categories.map((c) => (
            <button
              key={c.key}
              type="button"
              onClick={() => setCat(c.key)}
              className={cn(
                "border px-3 py-1.5 font-mono text-xs transition-colors",
                cat === c.key ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
              )}
            >
              {c.name}
            </button>
          ))}
        </div>

        {/* 双列瀑布流 */}
        <div data-enter className="mt-8">
          {items.length === 0 ? (
            <p className="border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
              无匹配单子 / NO RESULT
            </p>
          ) : (
            <div className="columns-1 gap-4 sm:columns-2">
              {items.map((it) => (
                <ItemCard key={it.id} item={it} />
              ))}
            </div>
          )}
        </div>
      </div>
    </main>
  );
}

