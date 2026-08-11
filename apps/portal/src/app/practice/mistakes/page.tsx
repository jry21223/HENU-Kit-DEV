"use client";

import { redirectToLogin } from "@/lib/api/client";
import { useLearningState } from "@/lib/practice/learning-state";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";

function ReloadButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="min-h-11 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
    >
      刷新同步
    </button>
  );
}

export default function MistakesPage() {
  usePageEnter(null);
  const { state, reload, previousPage, nextPage } = useLearningState();

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <header data-block data-enter>
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">MISTAKES</span>
          <span className="mx-2">/</span>
          LEARNING STATE
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
          错题本
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-ink/65">
          错题标记与作答计数由服务端同步；本页只展示这些事实和稳定引用，不补写题目内容。
        </p>
      </header>

      {state.status === "disabled" && (
        <section data-testid="learning-state-disabled" className="mt-10">
          <EmptyBlock label="错题同步即将上线，敬请期待" />
        </section>
      )}

      {state.status === "loading" && (
        <section data-testid="learning-state-loading" className="mt-10">
          <LoadingBlock label="正在同步错题" />
        </section>
      )}

      {state.status === "unauthenticated" && (
        <section data-testid="learning-state-signed-out" className="mt-10 border border-ink/25 p-6">
          <p className="font-mono text-xs tracking-[0.2em] text-ink/55">
            SIGN IN REQUIRED / 登录后查看跨设备同步的错题
          </p>
          <button
            type="button"
            onClick={() => redirectToLogin("/practice/mistakes")}
            className="mt-5 min-h-11 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            登录查看
          </button>
        </section>
      )}

      {state.status === "error" && (
        <section data-testid="learning-state-error" className="mt-10">
          <ErrorBanner message={state.message} onRetry={reload} />
        </section>
      )}

      {state.status === "empty" && (
        <section data-testid="learning-state-empty" className="mt-10 space-y-4">
          <EmptyBlock label="还没有同步的错题记录" />
          <div className="flex justify-end"><ReloadButton onClick={reload} /></div>
        </section>
      )}

      {state.status === "ready" && (
        <section data-testid="learning-state-success" data-block data-enter className="mt-10">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">
              已同步 {state.pagination.total} 条错题事实 · 第 {state.pagination.page} / {state.pagination.total_pages} 页
            </p>
            <ReloadButton onClick={reload} />
          </div>
          <div className="mt-5 space-y-4">
            {state.items.map((item) => (
              <article key={`${item.bank_id}:${item.question_id}`} className="min-w-0 border border-ink/25 p-5">
                <p className="break-all font-mono text-[10px] tracking-[0.12em] text-accent">
                  题库 {item.bank_id}
                </p>
                <p className="mt-2 break-all font-mono text-xs text-ink/70">
                  题目 {item.question_id}
                </p>
                <p className="mt-1 break-all font-mono text-[10px] text-ink/45">
                  版本 {item.question_version_id}
                </p>
                <div className="mt-4 flex flex-wrap gap-4 border-t border-line pt-3 font-mono text-xs text-ink/60">
                  <span>作答 {item.attempt_count} 次</span>
                  <span>答对 {item.correct_count} 次</span>
                  <time dateTime={item.updated_at}>更新 {item.updated_at}</time>
                </div>
              </article>
            ))}
          </div>
          <nav aria-label="错题分页" className="mt-6 flex items-center justify-between gap-4 border-t border-line pt-4">
            <button
              type="button"
              onClick={previousPage}
              disabled={state.pagination.page <= 1}
              className="min-h-11 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-not-allowed disabled:opacity-35"
            >
              上一页
            </button>
            <span className="font-mono text-xs text-ink/55">
              {state.pagination.page} / {state.pagination.total_pages}
            </span>
            <button
              type="button"
              onClick={nextPage}
              disabled={state.pagination.page >= state.pagination.total_pages}
              className="min-h-11 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-not-allowed disabled:opacity-35"
            >
              下一页
            </button>
          </nav>
        </section>
      )}
    </main>
  );
}
