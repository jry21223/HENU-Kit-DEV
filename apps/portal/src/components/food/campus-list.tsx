"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  fetchFoodPosts,
  formatPortalError,
  mockAllowed,
} from "@/lib/api/client";
import type { FoodPost } from "@/lib/api/types";
import { CAMPUSES, CampusKey, foodStore } from "@/lib/food/mock";
import Img from "@/components/ui/img";
import { useReveal } from "@/components/account/use-reveal";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { cn } from "@/lib/cn";

const TABS = [
  { key: "rec", label: "推荐" },
  { key: "new", label: "最新" },
  { key: "hot", label: "最热" },
] as const;

type TabKey = (typeof TABS)[number]["key"];
type LoadState = "loading" | "ready" | "error";
type CampusPost = FoodPost & { commentCount?: number };

function Avatar({ name }: { name: string }) {
  return (
    <span className="flex h-6 w-6 shrink-0 items-center justify-center border border-ink/40 font-display text-xs font-bold">
      {name.slice(0, 1)}
    </span>
  );
}

function PostCard({ post }: { post: CampusPost }) {
  return (
    <Link href={`/food/post/${post.id}`} className="group flex gap-4 border-b border-line py-5">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2.5 font-mono text-[11px] text-ink/50">
          <Avatar name={post.author} />
          <span>{post.author}</span>
          <span aria-hidden>·</span>
          <span>{post.time}</span>
        </div>
        <h3 className="mt-2.5 font-display text-xl font-bold leading-snug transition-colors group-hover:text-accent">
          {post.title}
        </h3>
        <p className="mt-1.5 truncate text-sm text-ink/60">{post.excerpt}</p>
        <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2">
          {post.tags.map((t) => (
            <span key={t} className="border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink/60">
              {t}
            </span>
          ))}
          <span className="ml-auto font-mono text-[11px] text-ink/50">
            ▲ {post.likes} · ★ {post.stars} · 评 {post.commentCount ?? 0}
          </span>
        </div>
      </div>
      {post.images?.[0] && (
        <Img
          src={post.images[0]}
          alt={post.title}
          label="IMG"
          className="hidden h-24 w-32 shrink-0 sm:block"
        />
      )}
    </Link>
  );
}

function fallbackCampusPosts(campus: CampusKey): CampusPost[] {
  const local = foodStore.get();
  return local.posts
    .filter((p) => p.campus === campus && !p.hidden)
    .map((p) => ({
      ...p,
      commentCount: local.comments.filter((cm) => cm.postId === p.id).length,
    }));
}

export default function CampusList({ campus }: { campus: CampusKey }) {
  const [tab, setTab] = useState<TabKey>("rec");
  const [posts, setPosts] = useState<CampusPost[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  useReveal();

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const response = await fetchFoodPosts(campus);
      setPosts(response.posts.filter((p) => !p.hidden));
      setLoadState("ready");
    } catch (cause) {
      if (mockAllowed) {
        setPosts(fallbackCampusPosts(campus));
        setLoadState("ready");
        return;
      }
      setPosts([]);
      setError(formatPortalError(cause) || "美食列表接口不可用。");
      setLoadState("error");
    }
  }, [campus]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const c = CAMPUSES[campus];
  const visible = posts;
  const sorted =
    tab === "hot"
      ? [...visible].sort((a, b) => b.likes - a.likes)
      : tab === "new"
        ? [...visible].sort((a, b) => (a.time === "刚刚" ? -1 : b.time === "刚刚" ? 1 : b.time.localeCompare(a.time)))
        : [...visible].sort((a, b) => b.likes + b.stars - (a.likes + a.stars));

  const top5 = [...visible].sort((a, b) => b.likes - a.likes).slice(0, 5);
  const allTags = Array.from(new Set(visible.flatMap((p) => p.tags)));

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">{c.index}</span>
        <span className="mx-2">/</span>
        {c.name}
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
        {c.name} · 吃什么
      </h1>
      <p data-enter className="mt-2 font-mono text-[10px] tracking-wider text-ink/40">
        当前页面为示例内容，正式数据接入中
      </p>

      <div className="mt-8 gap-10 lg:flex">
        <div className="min-w-0 flex-1">
          <div data-enter className="flex gap-2 border-b border-ink/40 pb-3">
            {TABS.map((t) => (
              <button
                key={t.key}
                type="button"
                onClick={() => setTab(t.key)}
                className={cn(
                  "border px-4 py-1.5 font-mono text-xs tracking-widest transition-colors",
                  tab === t.key
                    ? "border-ink bg-ink text-paper"
                    : "border-line text-ink/60 hover:border-ink/40 hover:text-ink"
                )}
              >
                {t.label}
              </button>
            ))}
          </div>
          <div data-enter>
            {loadState === "loading" ? (
              <div className="py-10">
                <LoadingBlock label="加载校区美食" />
              </div>
            ) : loadState === "error" ? (
              <div className="py-8">
                <ErrorBanner message={error ?? "美食列表接口不可用。"} onRetry={() => void load()} />
              </div>
            ) : sorted.length === 0 ? (
              <div className="py-8">
                <EmptyBlock label="暂无内容 / EMPTY" />
              </div>
            ) : (
              sorted.map((p) => <PostCard key={p.id} post={p} />)
            )}
          </div>
        </div>

        <aside className="mt-10 w-full shrink-0 lg:mt-0 lg:w-72">
          <div data-enter className="border border-ink/25 p-5">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">CAMPUS / 校区信息</p>
            <p className="mt-3 font-display text-xl font-bold">{c.name}</p>
            <p className="mt-2 font-mono text-[10px] leading-5 tracking-wider text-ink/50">
              锚点 {c.lat.toFixed(4)}, {c.lng.toFixed(4)}
              <br />
              收录 {visible.length} 篇
            </p>
          </div>

          <div data-enter className="mt-5 border border-ink/25 p-5">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">TOP5 / 本周热门</p>
            {loadState === "loading" ? (
              <p className="mt-3 font-mono text-[10px] text-ink/40">加载中…</p>
            ) : top5.length === 0 ? (
              <p className="mt-3 font-mono text-[10px] text-ink/40">暂无榜单</p>
            ) : (
              <ul className="mt-3 space-y-2.5">
                {top5.map((p, i) => (
                  <li key={p.id}>
                    <Link href={`/food/post/${p.id}`} className="group flex items-baseline gap-2.5">
                      <span className={cn("font-mono text-xs", i < 3 ? "text-accent" : "text-ink/30")}>
                        {String(i + 1).padStart(2, "0")}
                      </span>
                      <span className="truncate text-sm group-hover:text-accent">{p.title}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div data-enter className="mt-5 border border-ink/25 p-5">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">TAGS / 标签云</p>
            {allTags.length === 0 ? (
              <p className="mt-3 font-mono text-[10px] text-ink/40">暂无标签</p>
            ) : (
              <div className="mt-3 flex flex-wrap gap-1.5">
                {allTags.map((t) => (
                  <span key={t} className="border border-line px-2 py-0.5 font-mono text-[10px] text-ink/60">
                    {t}
                  </span>
                ))}
              </div>
            )}
          </div>
        </aside>
      </div>
    </main>
  );
}
