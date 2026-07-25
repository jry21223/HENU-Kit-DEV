"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  fetchFoodPosts,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { FoodPost } from "@/lib/api/types";
import { CAMPUSES, CAMPUS_KEYS, foodStore, type Post } from "@/lib/food/mock";
import {
  getFoodGatewayError,
  getGatewayPosts,
  initFoodGateway,
} from "@/lib/food/gateway";
import SubHero from "@/components/site-hero/sub-hero";
import { SceneFood } from "@/components/site-hero/scenes";
import { useReveal } from "@/components/account/use-reveal";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";

type LoadState = "loading" | "ready" | "error";

function toPost(p: FoodPost): Post {
  return {
    id: p.id,
    campus: p.campus,
    title: p.title,
    excerpt: p.excerpt,
    blocks: p.blocks,
    author: p.author,
    likes: p.likes,
    stars: p.stars,
    tags: p.tags,
    shop: p.shop,
    time: p.time,
    hidden: p.hidden,
    images: p.images,
  };
}

export default function FoodHomePage() {
  const [posts, setPosts] = useState<Post[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  useReveal();

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const resp = await fetchFoodPosts();
      setPosts(resp.posts.map(toPost));
      setLoadState("ready");
    } catch (e) {
      try {
        await initFoodGateway();
        const cached = getGatewayPosts();
        if (cached?.length) {
          setPosts(cached.map(toPost));
          setLoadState("ready");
          return;
        }
        if (mockAllowed) {
          // local mock store
          const data = foodStore.get();
          setPosts(data.posts);
          setLoadState("ready");
          return;
        }
        setPosts([]);
        setError(
          getFoodGatewayError() ||
            formatPortalError(e) ||
            "美食接口不可用，生产环境已禁用 mock 回退。"
        );
        setLoadState("error");
      } catch (e2) {
        if (mockAllowed) {
          setPosts(foodStore.get().posts);
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

  const latestOf = (campus: string) =>
    posts.filter((p) => p.campus === campus && !p.hidden)[0];
  const visible = posts.filter((p) => !p.hidden);
  const totalLikes = visible.reduce((s, p) => s + p.likes, 0);

  return (
    <main>
      <SubHero
        index="03"
        en="FOOD RANKING"
        title="美食榜"
        slogan="「从夯到拉，只说人话。」全校学生真实打分与锐评，不接受充值，不接受公关。"
        counters={[
          { label: "锐评总数", value: visible.length },
          { label: "累计点赞", value: totalLikes },
        ]}
        fig="FIG.03 定位 / LOCATE"
        scene={<SceneFood />}
      />

      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}

        {loadState === "loading" ? (
          <LoadingBlock label="加载美食" />
        ) : loadState === "error" ? (
          <EmptyBlock label="接口不可用" />
        ) : (
          <>
            <div className="grid gap-4 md:grid-cols-3">
              {CAMPUS_KEYS.map((key) => {
                const campus = CAMPUSES[key];
                const campusPosts = posts.filter((p) => p.campus === key && !p.hidden);
                const latest = latestOf(key);
                return (
                  <Link
                    key={key}
                    href={`/food/campus/${key}`}
                    data-enter
                    className="group border border-ink/25 border-l-2 border-l-transparent p-6 transition-colors hover:border-ink hover:border-l-accent"
                  >
                    <p className="font-display text-5xl font-bold text-ink/20 transition-colors group-hover:text-accent">
                      {campus.index}
                    </p>
                    <h2 className="mt-4 font-display text-2xl font-bold">{campus.name}</h2>
                    <p className="mt-3 border-t border-line pt-3 font-mono text-[10px] leading-5 tracking-wider text-ink/50">
                      {campusPosts.length} 篇锐评
                      <br />
                      {latest ? `最新 · ${latest.title.slice(0, 14)}…` : "暂无内容"}
                    </p>
                  </Link>
                );
              })}
            </div>

            <div data-enter className="mt-10 border border-ink p-6 md:flex md:items-center md:justify-between">
              <div>
                <p className="font-mono text-[10px] tracking-[0.3em] text-ink/50">RANK / 全站</p>
                <p className="mt-2 font-display text-2xl font-bold">必吃美食排行榜</p>
              </div>
              <Link
                href="/food/leaderboard"
                className="mt-4 inline-block border border-ink px-6 py-3 font-mono text-sm tracking-widest transition-colors hover:bg-ink hover:text-paper md:mt-0"
              >
                查看榜单 →
              </Link>
            </div>
          </>
        )}
      </div>
    </main>
  );
}
