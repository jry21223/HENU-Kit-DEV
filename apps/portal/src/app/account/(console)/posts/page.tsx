"use client";

import Link from "next/link";
import { useState } from "react";
import { useSyncExternalStore } from "react";
import { CAMPUSES, foodStore } from "@/lib/food/mock";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

export default function AccountPostsPage() {
  const data = useSyncExternalStore(foodStore.subscribe, foodStore.get, foodStore.getServer);
  const [confirmId, setConfirmId] = useState<string | null>(null);
  useReveal();

  const mine = data.posts.filter((p) => p.isMine);

  return (
    <div>
      <div data-enter className="flex items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">A-07</span>
            <span className="mx-2">/</span>
            MY POSTS
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">我的文章</h1>
        </div>
        <Link
          href="/food/publish"
          className="border border-ink px-5 py-2.5 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          去发布 +
        </Link>
      </div>

      {mine.length === 0 ? (
        <p data-enter className="mt-10 border border-dashed border-ink/30 px-5 py-12 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
          还没有发布过锐评 / EMPTY
        </p>
      ) : (
        <div data-enter className="mt-8 border-t border-ink/40">
          {mine.map((p) => (
            <div key={p.id} className="border-b border-line py-4">
              <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
                <Link href={`/food/post/${p.id}`} className="min-w-0 flex-1">
                  <p className="truncate font-medium transition-colors hover:text-accent">
                    {p.title}
                  </p>
                  <p className="mt-1 font-mono text-[10px] tracking-wider text-ink/50">
                    {CAMPUSES[p.campus].name} · ▲ {p.likes} · ★ {p.stars} · {p.time}
                  </p>
                </Link>
                <span
                  className={cn(
                    "border px-1.5 py-0.5 font-mono text-[10px]",
                    p.hidden ? "border-accent text-accent" : "border-line text-ink/50"
                  )}
                >
                  {p.hidden ? "已隐藏" : "可见"}
                </span>
                <div className="flex gap-2">
                  <Link
                    href={`/food/publish?edit=${p.id}`}
                    className="border border-ink/30 px-3 py-1.5 font-mono text-xs transition-colors hover:border-ink"
                  >
                    编辑
                  </Link>
                  <button
                    type="button"
                    onClick={() => foodStore.toggleHidden(p.id)}
                    className="border border-ink/30 px-3 py-1.5 font-mono text-xs transition-colors hover:border-ink"
                  >
                    {p.hidden ? "显示" : "隐藏"}
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmId(p.id)}
                    className="border border-accent/60 px-3 py-1.5 font-mono text-xs text-accent transition-colors hover:border-accent"
                  >
                    删除
                  </button>
                </div>
              </div>
              {/* 删除二次确认 */}
              {confirmId === p.id && (
                <div className="mt-3 flex items-center gap-4 border border-accent px-4 py-3">
                  <p className="font-mono text-xs text-accent">
                    确认删除《{p.title}》？该操作不可撤销（本会话内）。
                  </p>
                  <button
                    type="button"
                    onClick={() => {
                      foodStore.removePost(p.id);
                      setConfirmId(null);
                    }}
                    className="ml-auto border border-accent bg-accent px-4 py-1.5 font-mono text-xs text-paper"
                  >
                    确认删除
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmId(null)}
                    className="border border-ink/30 px-4 py-1.5 font-mono text-xs hover:border-ink"
                  >
                    取消
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
