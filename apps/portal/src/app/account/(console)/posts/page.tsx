"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { useReveal } from "@/components/account/use-reveal";
import { formatPortalError } from "@/lib/api/client";
import type { FoodPostListResponse } from "@/lib/api/types";
import { CAMPUSES } from "@/lib/food/campuses";
import { fetchMyFoodPosts } from "@/lib/food/myposts";
import { FOOD_TIERS, resolveFoodTier } from "@/lib/food/ranking";

type ListState =
  | { kind: "loading" }
  | { kind: "success"; response: FoodPostListResponse }
  | { kind: "error"; message: string };

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function tierLabelFor(tags: string[]): string {
  const key = resolveFoodTier({ tags });
  return FOOD_TIERS.find((tier) => tier.key === key)?.label ?? "未定档";
}

export default function MyFoodPostsPage() {
  const [listState, setListState] = useState<ListState>({ kind: "loading" });
  const requestVersion = useRef(0);
  const handleUnauthorized = useAccountConsoleUnauthorizedHandler();
  useReveal();

  const loadPosts = useCallback(() => {
    const version = ++requestVersion.current;
    void fetchMyFoodPosts().then(
      (response) => {
        if (version === requestVersion.current) {
          setListState({ kind: "success", response });
        }
      },
      (error: unknown) => {
        if (version === requestVersion.current && !handleUnauthorized(error)) {
          setListState({ kind: "error", message: formatPortalError(error) });
        }
      }
    );
  }, [handleUnauthorized]);

  useEffect(() => {
    loadPosts();
    return () => {
      requestVersion.current += 1;
    };
  }, [loadPosts]);

  const posts = listState.kind === "success" ? listState.response.posts : [];

  return (
    <div>
      <section data-enter className="border-b border-ink pb-5">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
          <span className="text-accent">A-07</span>
          <span className="mx-2">/</span>
          MY FOOD POSTS
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">我的投稿</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
          这里是你发布过的全部美食投稿；投稿创建后立即公开，点击任意一条可查看公开详情页。
        </p>
      </section>

      {listState.kind === "loading" ? (
        <section
          data-account-food-posts-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          FOOD POSTS LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {listState.kind === "error" ? (
        <section data-account-food-posts-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">FOOD POSTS UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{listState.message}</p>
          <button
            type="button"
            onClick={() => {
              setListState({ kind: "loading" });
              loadPosts();
            }}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {listState.kind === "success" ? (
        <section data-account-food-posts-state="success" className="mt-6">
          {posts.length === 0 ? (
            <div data-account-food-posts-empty className="border-y border-line py-8">
              <p className="font-display text-xl font-bold">还没有发布过投稿</p>
              <p className="mt-2 text-sm leading-6 text-ink/60">
                分享你吃到的好店，投稿创建后立即公开上榜；发布过的内容会全部出现在这里。
              </p>
              <Link
                href="/food/publish"
                className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent"
              >
                去投稿
              </Link>
            </div>
          ) : (
            <div className="border-t border-ink">
              {posts.map((post) => (
                <Link
                  key={post.id}
                  href={`/food/post/${post.id}`}
                  className="group block border-b border-line px-1 py-5 transition-colors hover:bg-ink/5"
                >
                  <div className="flex items-start justify-between gap-3">
                    <p className="min-w-0 truncate font-display text-lg font-bold transition-colors group-hover:text-accent">
                      {post.title}
                    </p>
                    <span className="shrink-0 border border-accent px-2 py-1 font-mono text-[10px] tracking-wider text-accent">
                      {tierLabelFor(post.tags)}
                    </span>
                  </div>
                  <p className="mt-2 font-mono text-[10px] tracking-[0.12em] text-ink/45">
                    {CAMPUSES[post.campus].name} · 发布于 {formatTimestamp(post.time)}
                  </p>
                </Link>
              ))}
            </div>
          )}
        </section>
      ) : null}
    </div>
  );
}
