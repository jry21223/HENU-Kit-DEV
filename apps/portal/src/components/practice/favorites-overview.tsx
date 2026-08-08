"use client";

import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import FavoritesLoginPrompt from "@/components/practice/favorites-login-prompt";
import TransitionLink from "@/components/practice/transition/transition-link";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import { fetchFavoritesOverview } from "@/lib/api/client";
import type { FavoriteFolder } from "@/lib/api/types";
import { useFetchState } from "@/lib/api/use-fetch-state";

function FolderCard({ folder, index }: { folder: FavoriteFolder; index: number }) {
  const total = folder.available_count + folder.unavailable_count;
  return (
    <TransitionLink
      href={`/practice/favorites/${encodeURIComponent(folder.bank_id)}?name=${encodeURIComponent(folder.bank_name)}`}
      morph={{ kind: "list", id: folder.bank_id, title: folder.bank_name, sub: `${total} 题` }}
      className="group block border border-ink/25 bg-paper p-5 transition-colors hover:border-ink"
    >
      <div className="flex items-start justify-between">
        <span className="font-mono text-xs text-accent">
          F-{String(index + 1).padStart(2, "0")}
        </span>
        <span aria-hidden className="font-mono text-xs text-ink/40">+</span>
      </div>
      <h3 className="mt-3 font-display text-xl font-bold leading-snug group-hover:underline">
        {folder.bank_name}
      </h3>
      <p className="mt-2 font-mono text-[10px] tracking-wider text-ink/50">
        {folder.available_count} 题可练习
        {folder.unavailable_count > 0 && ` · ${folder.unavailable_count} 题暂不可用`}
      </p>
      <div className="mt-5 border-t border-line pt-3">
        <span className="font-mono text-xs tracking-wider text-ink/45">
          查看收藏 →
        </span>
      </div>
    </TransitionLink>
  );
}

export default function FavoritesOverview() {
  usePageEnter(null);
  const { state, retry } = useFetchState<FavoriteFolder[]>(() => fetchFavoritesOverview(), []);

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div data-block data-enter>
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">FAV</span>
          <span className="mx-2">/</span>
          COLLECTION
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
          收藏夹
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-ink/65">
          按题库分组的收藏题目。不可用的题目保留收藏关系，但不会进入收藏练习。
        </p>
      </div>

      {state.status === "loading" && (
        <section data-testid="practice-favorites-loading" className="mt-8">
          <LoadingBlock label="正在读取收藏夹" />
        </section>
      )}

      {state.status === "anonymous" && (
        <section data-testid="practice-favorites-anonymous">
          <FavoritesLoginPrompt next="/practice/favorites" />
        </section>
      )}

      {state.status === "error" && (
        <section data-testid="practice-favorites-error" className="mt-8">
          <ErrorBanner
            message={state.message}
            onRetry={() => retry()}
          />
        </section>
      )}

      {state.status === "ready" && (
        <section data-testid="practice-favorites-overview" className="mt-8">
          {state.data.length === 0 ? (
            <EmptyBlock label="还没有收藏任何题目，刷题时点击「收藏」即可加入" />
          ) : (
            <div data-enter className="grid gap-5 md:grid-cols-2">
              {state.data.map((folder, index) => (
                <FolderCard key={folder.bank_id} folder={folder} index={index} />
              ))}
            </div>
          )}
        </section>
      )}
    </main>
  );
}
