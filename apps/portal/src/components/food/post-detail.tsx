"use client";

import Link from "next/link";
import dynamic from "next/dynamic";
import { useRef, useState } from "react";
import { useSyncExternalStore } from "react";
import { CAMPUSES, foodStore } from "@/lib/food/mock";
import { authStore } from "@/lib/auth/store";
import PostBlocks from "@/components/food/post-blocks";
import Img from "@/components/ui/img";
import { useReveal } from "@/components/account/use-reveal";
import { gsap, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const ShopMap = dynamic(() => import("@/components/food/map"), {
  ssr: false,
  loading: () => (
    <div className="bg-blueprint flex h-[280px] items-center justify-center border-t border-line">
      <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">MAP LOADING…</p>
    </div>
  ),
});

export default function PostDetail({ id }: { id: string }) {
  const data = useSyncExternalStore(foodStore.subscribe, foodStore.get, foodStore.getServer);
  const { user } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);
  useReveal();
  const likeRef = useRef<HTMLButtonElement>(null);
  const [draft, setDraft] = useState("");

  const post = data.posts.find((p) => p.id === id);

  if (!post || (post.hidden && !post.isMine)) {
    return (
      <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
        <p className="mt-4 font-display text-2xl font-bold">文章不存在或已隐藏</p>
        <Link href="/food" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
          ← 返回美食榜
        </Link>
      </main>
    );
  }

  const comments = data.comments.filter((c) => c.postId === id);

  const bounce = () => {
    if (window.matchMedia(REDUCED_MOTION).matches) return;
    gsap.fromTo(
      likeRef.current,
      { scale: 1 },
      { scale: 1.18, duration: 0.15, yoyo: true, repeat: 1, ease: "power2.out" }
    );
  };

  const submitComment = () => {
    if (!user || !draft.trim()) return;
    foodStore.addComment(id, user.name, draft.trim());
    setDraft("");
  };

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      <div className="max-w-[70ch]">
      {/* 头部 */}
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">{CAMPUSES[post.campus].index}</span>
        <span className="mx-2">/</span>
        {CAMPUSES[post.campus].name}
        {post.hidden && <span className="ml-3 border border-accent px-1.5 py-0.5 text-[10px] text-accent">已隐藏</span>}
      </p>
      <h1 data-enter className="mt-4 font-display text-3xl font-bold leading-tight tracking-tight md:text-4xl">
        {post.title}
      </h1>
      <div data-enter className="mt-4 flex items-center gap-3 border-b border-line pb-5">
        <span className="flex h-8 w-8 items-center justify-center border border-ink font-display text-sm font-bold">
          {post.author.slice(0, 1)}
        </span>
        <span className="text-sm">{post.author}</span>
        <span className="font-mono text-[11px] text-ink/50">{post.time}</span>
        <span className="ml-auto flex gap-1.5">
          {post.tags.map((t) => (
            <span key={t} className="border border-line px-1.5 py-0.5 font-mono text-[10px] text-ink/60">
              {t}
            </span>
          ))}
        </span>
      </div>

      {/* 封面图（有则显示） */}
      {post.images?.[0] && (
        <div data-enter className="mt-6">
          <Img src={post.images[0]} alt={post.title} label="COVER" className="h-56 w-full md:h-72" />
        </div>
      )}

      {/* 正文 */}
      <article data-enter className="mt-8">
        <PostBlocks blocks={post.blocks} />
      </article>

      {/* 操作条 */}
      <div data-enter className="mt-10 flex gap-3 border-y border-line py-4">
        <button
          ref={likeRef}
          type="button"
          onClick={() => {
            foodStore.toggleLike(id);
            if (!post.liked) bounce();
          }}
          className={cn(
            "flex items-center gap-2 border px-5 py-2.5 font-mono text-sm transition-colors",
            post.liked ? "border-accent bg-accent text-paper" : "border-ink/30 hover:border-ink"
          )}
        >
          ▲ 点赞 {post.likes}
        </button>
        <button
          type="button"
          onClick={() => foodStore.toggleStar(id)}
          className={cn(
            "flex items-center gap-2 border px-5 py-2.5 font-mono text-sm transition-colors",
            post.starred ? "border-accent text-accent" : "border-ink/30 hover:border-ink"
          )}
        >
          ★ 收藏 {post.stars}
        </button>
      </div>

      {/* 地图卡 */}
      <section data-enter className="mt-10 border border-ink/25">
        <div className="flex items-center justify-between px-4 py-3">
          <p className="font-mono text-xs tracking-[0.2em]">
            <span className="text-accent">LOC</span>
            <span className="mx-2">/</span>
            {post.shop.name}
          </p>
          <p className="font-mono text-[10px] text-ink/50">
            {post.shop.lat.toFixed(4)}, {post.shop.lng.toFixed(4)}
          </p>
        </div>
        <ShopMap shop={post.shop} />
      </section>

      {/* 评论区 */}
      <section data-enter className="mt-12">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
          COMMENTS / 评论 · {comments.length}
        </p>
        <ul className="mt-5 space-y-5">
          {comments.map((c) => (
            <li key={c.id} className="flex gap-3">
              <span className="flex h-7 w-7 shrink-0 items-center justify-center border border-ink/40 font-display text-xs font-bold">
                {c.author.slice(0, 1)}
              </span>
              <div className="min-w-0">
                <p className="font-mono text-[11px] text-ink/50">
                  {c.author}
                  <span className="mx-2">·</span>
                  {c.time}
                </p>
                <p className="mt-1 text-sm leading-6 text-ink/85">{c.text}</p>
              </div>
            </li>
          ))}
        </ul>

        <div className="mt-8 border-t border-line pt-5">
          {user ? (
            <div className="flex gap-3">
              <input
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="写下你的锐评…"
                className="flex-1 border-b border-ink/30 bg-transparent py-2 text-sm outline-none placeholder:text-ink/30 focus:border-ink"
              />
              <button
                type="button"
                onClick={submitComment}
                disabled={!draft.trim()}
                className={cn(
                  "border px-5 py-2 font-mono text-xs tracking-widest transition-colors",
                  draft.trim()
                    ? "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                    : "cursor-not-allowed border-line text-ink/30"
                )}
              >
                发表
              </button>
            </div>
          ) : (
            <p className="font-mono text-xs text-ink/50">
              <Link href={`/account/login?next=/food/post/${id}`} className="text-accent hover:underline">
                登录
              </Link>{" "}
              后参与评论
            </p>
          )}
        </div>
      </section>
      </div>
    </main>
  );
}
