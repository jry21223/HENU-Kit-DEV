"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchCampusCategories,
  fetchCampusItems,
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
  getGatewayCategories,
  getGatewayItems,
  initCampusGateway,
  rememberCampusItems,
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
  const filtersRef = useRef<HTMLDivElement>(null);
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
      rememberCampusItems(itemsResp.items, catsResp?.categories ?? null);
      setLoadState("ready");
    } catch {
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
        setError("互助信息暂时无法加载，请重试。");
        setLoadState("error");
      } catch {
        if (mockAllowed) {
          setItems(campusStore.get().items);
          setCategories(CATEGORIES);
          setLoadState("ready");
          return;
        }
        setError("互助信息暂时无法加载，请重试。");
        setLoadState("error");
      }
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
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
  const hasActiveFilter = query.trim() !== "" || cat !== "all" || type !== "all";
  const clearFilters = () => {
    setQuery("");
    setCat("all");
    setType("all");
    // 清除筛选按钮会随空状态一起消失；焦点交给刚复位的搜索与筛选区，键盘和读屏用户不丢位置。
    // 不直接聚焦搜索框：手机上会弹出输入法，挡住刚恢复的列表。
    filtersRef.current?.focus();
  };

  return (
    // 主体至少一屏高：单子是挂载后才拉取的，加载中的短页面会让页脚露在首屏底部，
    // 单子一到就把页脚挤出去，手机上这一下就是 0.13 的布局偏移（#548）。
    <main className="min-h-svh">
      <SubHero
        index="04"
        en="CAMPUS MARKET"
        title="互助平台"
        slogan="可浏览互助与闲置信息；发布、接单和结算暂未开放。"
        counters={[
          { label: "在架单子", value: loadState === "ready" ? openCount : null, busy: loadState === "loading" },
          { label: "已完成单子", value: loadState === "ready" ? doneCount : null, busy: loadState === "loading" },
        ]}
        fig={{ code: "FIG.04", name: "交接", en: "HANDOVER" }}
        scene={<SceneHandshake />}
        compactOnMobile
      />

      <div className="mx-auto max-w-site px-5 py-6 md:px-8 lg:py-10">
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}

        <div
          ref={filtersRef}
          data-enter
          role="search"
          aria-label="互助搜索与筛选"
          tabIndex={-1}
          className="flex flex-wrap items-center gap-x-6 gap-y-3 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-accent"
        >
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索：快递 / 键盘 / 占座"
            className="h-11 w-52 border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/60 focus:border-accent"
          />
          {/* 两组筛选都以“全部”开头：各带一个看得见的组名，免得分不清。 */}
          <div role="group" aria-labelledby="campus-filter-type" className="flex flex-wrap items-center gap-2">
            <span id="campus-filter-type" className="mr-1 font-mono text-xs text-ink/70">
              单子类型
            </span>
            {(["all", "help", "sell"] as const).map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setType(t)}
                aria-pressed={type === t}
                className={cn(
                  "min-h-11 border px-3 font-mono text-xs transition-colors",
                  type === t
                    ? t === "help"
                      ? "border-accent bg-accent text-ink"
                      : "border-ink bg-ink text-paper"
                    : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {t === "all" ? "全部" : t === "help" ? "求助单" : "闲置单"}
              </button>
            ))}
          </div>
          <div role="group" aria-labelledby="campus-filter-category" className="flex flex-wrap items-center gap-2">
            <span id="campus-filter-category" className="mr-1 font-mono text-xs text-ink/70">
              分类
            </span>
            <button
              type="button"
              onClick={() => setCat("all")}
              aria-pressed={cat === "all"}
              className={cn(
                "min-h-11 border px-3 font-mono text-xs transition-colors",
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
                aria-pressed={cat === c.key}
                className={cn(
                  "min-h-11 border px-3 font-mono text-xs transition-colors",
                  cat === c.key ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {c.name}
              </button>
            ))}
          </div>
        </div>

        <div data-enter className="mt-8">
          {loadState === "loading" ? (
            <LoadingBlock label="加载互助单" />
          ) : loadState === "error" ? null : filtered.length === 0 ? (
            // 一条单子都没有时清除筛选帮不上忙，发布也还没开放：只给回首页。
            <EmptyBlock
              label={openCount === 0 ? "暂无互助或闲置信息" : "无匹配单子"}
              action={
                openCount === 0
                  ? { label: "回首页", href: "/" }
                  : hasActiveFilter
                    ? { label: "清除筛选", onClick: clearFilters }
                    : undefined
              }
            />
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
