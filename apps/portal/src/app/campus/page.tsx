"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchCampusCategories,
  fetchCampusItems,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { CampusCategory, CampusItem } from "@/lib/api/types";
import {
  CATEGORIES,
  campusStore,
  ItemType,
  type Item,
  type Category,
} from "@/lib/campus/mock";
import {
  getCampusGatewayError,
  getGatewayCategories,
  getGatewayItems,
  initCampusGateway,
} from "@/lib/campus/gateway";
import ItemCard from "@/components/campus/item-card";
import SubHero from "@/components/site-hero/sub-hero";
import { SceneHandshake } from "@/components/site-hero/scenes";
import { useReveal } from "@/components/account/use-reveal";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { cn } from "@/lib/cn";

type LoadState = "loading" | "ready" | "error";

function toItem(it: CampusItem): Item {
  return {
    id: it.id,
    type: it.type,
    category: it.category,
    title: it.title,
    desc: it.desc,
    price: it.price,
    seller: it.seller,
    credit: it.credit,
    dealsDone: it.dealsDone,
    wants: it.wants,
    place: it.place,
    deadline: it.deadline,
    status: it.status,
    time: it.time,
    images: it.images,
  };
}

function toCategories(cats: CampusCategory[] | null): Category[] {
  if (!cats?.length) return CATEGORIES;
  return cats.map((c) => ({ key: c.key, name: c.name, code: c.code }));
}

export default function MarketPage() {
  const [items, setItems] = useState<Item[]>([]);
  const [categories, setCategories] = useState<Category[]>(CATEGORIES);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [cat, setCat] = useState<string>("all");
  const [type, setType] = useState<ItemType | "all">("all");
  useReveal();

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const [itemsResp, catsResp] = await Promise.all([
        fetchCampusItems(),
        fetchCampusCategories().catch(() => null),
      ]);
      setItems(itemsResp.items.map(toItem));
      setCategories(toCategories(catsResp?.categories ?? null));
      setLoadState("ready");
    } catch (e) {
      try {
        await initCampusGateway();
        const cached = getGatewayItems();
        const cats = getGatewayCategories();
        if (cached?.length) {
          setItems(cached.map(toItem));
          setCategories(toCategories(cats));
          setLoadState("ready");
          return;
        }
        if (mockAllowed) {
          const data = campusStore.get();
          setItems(data.items);
          setCategories(CATEGORIES);
          setLoadState("ready");
          return;
        }
        setItems([]);
        setError(
          getCampusGatewayError() ||
            formatPortalError(e) ||
            "互助平台接口不可用，生产环境已禁用 mock 回退。"
        );
        setLoadState("error");
      } catch (e2) {
        if (mockAllowed) {
          setItems(campusStore.get().items);
          setCategories(CATEGORIES);
          setLoadState("ready");
          return;
        }
        setError(formatPortalError(e2));
        setLoadState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const filtered = items.filter(
    (it) =>
      it.status !== "hidden" &&
      (cat === "all" || it.category === cat) &&
      (type === "all" || it.type === type) &&
      (!query.trim() || it.title.includes(query.trim()))
  );
  const openCount = items.filter((i) => i.status !== "hidden").length;
  const doneCount = items.filter((i) => i.status === "done").length;

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
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}

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

        <div data-enter className="mt-8">
          {loadState === "loading" ? (
            <LoadingBlock label="加载互助单" />
          ) : loadState === "error" ? (
            <EmptyBlock label="接口不可用" />
          ) : filtered.length === 0 ? (
            <EmptyBlock label="无匹配单子" />
          ) : (
            <div className="columns-1 gap-4 sm:columns-2">
              {filtered.map((it) => (
                <ItemCard key={it.id} item={it} />
              ))}
            </div>
          )}
        </div>
      </div>
    </main>
  );
}
