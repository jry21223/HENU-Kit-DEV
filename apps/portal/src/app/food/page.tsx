"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";

import { useReveal } from "@/components/account/use-reveal";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import Img from "@/components/ui/img";
import {
  fetchFoodPosts,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { FoodPost } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { CAMPUSES, CAMPUS_KEYS, type CampusKey, foodStore } from "@/lib/food/mock";
import { groupFoodPostsByTier } from "@/lib/food/ranking";

type LoadState = "loading" | "ready" | "error";

export default function FoodBoardPage() {
  const [posts, setPosts] = useState<FoodPost[]>([]);
  const [campus, setCampus] = useState<CampusKey | "all">("all");
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const response = await fetchFoodPosts();
      setPosts(response.posts);
      setLoadState("ready");
    } catch (loadError) {
      if (mockAllowed) {
        setPosts(foodStore.get().posts);
        setLoadState("ready");
        return;
      }
      setPosts([]);
      setError(formatPortalError(loadError));
      setLoadState("error");
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  useReveal([campus, loadState]);

  const groups = useMemo(
    () => groupFoodPostsByTier(posts, campus),
    [posts, campus]
  );
  const visibleCount = groups.reduce((total, group) => total + group.posts.length, 0);

  return (
    <main>
      <section className="border-b border-ink bg-ink text-paper">
        <div className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
          <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_22rem] lg:items-end">
            <div data-enter>
              <p className="font-mono text-xs tracking-[0.3em] text-paper/55">
                <span className="text-accent">F-01</span>
                <span className="mx-2">/</span>
                FIVE-TIER BOARD
              </p>
              <h1 className="mt-4 font-display text-5xl font-bold tracking-tight sm:text-6xl md:text-7xl">
                从夯到拉
              </h1>
              <p className="mt-5 max-w-2xl text-sm leading-7 text-paper/65 md:text-base">
                夯、顶级、人上人、NPC、拉完了。学生视角分档，档内按点赞排序；
                不接受充值，不接受公关。
              </p>
            </div>

            <div data-enter className="border-t border-paper/25 pt-5 lg:border-l lg:border-t-0 lg:pl-7 lg:pt-0">
              <p className="font-mono text-[10px] tracking-[0.28em] text-paper/45">
                STUDENT FOOD DESK
              </p>
              <p className="mt-2 font-display text-2xl font-bold">你吃到的好店，也该上榜。</p>
              <Link
                href="/food/publish"
                className="mt-5 inline-flex min-h-11 items-center border border-paper px-5 font-mono text-xs tracking-[0.18em] transition-colors hover:border-accent hover:bg-accent hover:text-paper"
              >
                投稿一家好店
                <span aria-hidden className="ml-3">
                  →
                </span>
              </Link>
            </div>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8 md:py-12">
        <div data-enter className="flex flex-col gap-5 border-b border-ink pb-6 md:flex-row md:items-end md:justify-between">
          <div>
            <p className="font-mono text-[10px] tracking-[0.28em] text-ink/45">
              CAMPUS FILTER
            </p>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                type="button"
                aria-pressed={campus === "all"}
                onClick={() => setCampus("all")}
                className={cn(
                  "min-h-10 border px-4 font-mono text-xs transition-colors",
                  campus === "all"
                    ? "border-ink bg-ink text-paper"
                    : "border-line text-ink/60 hover:border-ink"
                )}
              >
                全部校区
              </button>
              {CAMPUS_KEYS.map((key) => (
                <button
                  key={key}
                  type="button"
                  aria-pressed={campus === key}
                  onClick={() => setCampus(key)}
                  className={cn(
                    "min-h-10 border px-4 font-mono text-xs transition-colors",
                    campus === key
                      ? "border-ink bg-ink text-paper"
                      : "border-line text-ink/60 hover:border-ink"
                  )}
                >
                  {CAMPUSES[key].name}
                </button>
              ))}
            </div>
          </div>
          <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">
            {loadState === "ready" ? `${visibleCount} ENTRIES` : "SYNCING"}
          </p>
        </div>

        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mt-8" />
        )}

        {loadState === "loading" ? (
          <div className="mt-8">
            <LoadingBlock label="加载五档榜单" />
          </div>
        ) : loadState === "error" ? (
          <div className="mt-8">
            <EmptyBlock label="榜单暂时加载不出来，请稍后刷新试试" />
          </div>
        ) : (
          <>
            <nav
              data-enter
              aria-label="五档榜单导览"
              className="mt-8 flex max-w-full gap-px overflow-x-auto border border-ink bg-ink"
            >
              {groups.map(({ tier, posts: tierPosts }) => (
                <a
                  key={tier.key}
                  href={`#tier-${tier.key}`}
                  className="min-w-28 flex-1 bg-paper px-4 py-3 transition-colors hover:bg-accent hover:text-paper"
                >
                  <span className="block font-mono text-[9px] tracking-[0.2em] opacity-50">
                    {tier.index}
                  </span>
                  <span className="mt-1 block font-display text-lg font-bold">{tier.label}</span>
                  <span className="mt-1 block font-mono text-[9px] opacity-45">
                    {tierPosts.length} ENTRIES
                  </span>
                </a>
              ))}
            </nav>

            <div className="mt-16">
              {groups.map(({ tier, posts: tierPosts }) => (
                <section
                  key={tier.key}
                  id={`tier-${tier.key}`}
                  data-food-tier={tier.key}
                  className="grid scroll-mt-20 border-t border-ink py-8 md:grid-cols-[12rem_minmax(0,1fr)] md:gap-10 md:py-12"
                >
                  <header data-enter className="pb-6 md:pb-0">
                    <p className="font-mono text-[10px] tracking-[0.25em] text-ink/45">
                      {tier.index} / {tier.en}
                    </p>
                    <h2
                      data-food-tier-label
                      className={cn(
                        "mt-2 font-display text-5xl font-bold tracking-tight md:text-6xl",
                        tier.key === "hang" && "text-accent"
                      )}
                    >
                      {tier.label}
                    </h2>
                    <p className="mt-3 font-mono text-xs text-ink/55">{tier.blurb}</p>
                  </header>

                  {tierPosts.length === 0 ? (
                    <p
                      data-enter
                      className="border-t border-dashed border-line py-8 font-mono text-xs tracking-[0.18em] text-ink/35"
                    >
                      暂无已审核条目
                    </p>
                  ) : (
                    <ol className="border-t border-ink">
                      {tierPosts.map((post, index) => (
                        <li key={post.id} data-enter>
                          <Link
                            href={`/food/post/${post.id}`}
                            className="group grid grid-cols-[3rem_minmax(0,1fr)] gap-3 border-b border-line py-5 md:grid-cols-[3rem_7.5rem_minmax(0,1fr)_auto] md:items-center md:gap-5"
                          >
                            <span className="font-mono text-sm text-ink/30">
                              {String(index + 1).padStart(2, "0")}
                            </span>
                            <div className="hidden h-20 overflow-hidden bg-ink/[0.04] md:block">
                              {post.images?.[0] ? (
                                <Img
                                  src={post.images[0]}
                                  alt=""
                                  label={tier.index}
                                  className="h-full w-full transition-transform duration-500 group-hover:scale-[1.04]"
                                />
                              ) : (
                                <span className="flex h-full items-center justify-center font-display text-4xl font-bold text-ink/12">
                                  {tier.label}
                                </span>
                              )}
                            </div>
                            <span className="min-w-0">
                              <span className="block font-display text-xl font-bold transition-colors group-hover:text-accent">
                                {post.shop.name}
                              </span>
                              <span className="mt-1 block truncate text-sm text-ink/65">
                                {post.title}
                              </span>
                              <span className="mt-2 block font-mono text-[10px] tracking-[0.12em] text-ink/40">
                                {CAMPUSES[post.campus].name} · {post.tags.join(" / ")}
                              </span>
                            </span>
                            <span className="col-start-2 flex items-center gap-4 font-mono text-xs text-ink/50 md:col-start-auto">
                              <span>▲ {post.likes}</span>
                              <span className="transition-transform group-hover:translate-x-1" aria-hidden>
                                →
                              </span>
                            </span>
                          </Link>
                        </li>
                      ))}
                    </ol>
                  )}
                </section>
              ))}
            </div>
          </>
        )}

        <div data-enter className="mt-4 border-y border-ink py-8 md:flex md:items-center md:justify-between">
          <div>
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/45">
              RANKING NOTE
            </p>
            <p className="mt-2 max-w-2xl text-sm leading-7 text-ink/65">
              分档代表学生攻略编辑视角；信息可能随价格、营业状态与出品变化。吃过以后，欢迎来补充真实体验。
            </p>
          </div>
          <Link
            href="/food/publish"
            className="mt-5 inline-flex min-h-11 items-center border border-ink bg-ink px-5 font-mono text-xs tracking-[0.18em] text-paper transition-colors hover:border-accent hover:bg-accent md:mt-0"
          >
            提交推荐 →
          </Link>
        </div>
      </div>
    </main>
  );
}
