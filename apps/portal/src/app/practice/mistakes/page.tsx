"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchLearningState,
  fetchQuizCraftCatalog,
  formatPortalError,
  PortalUnauthorizedError,
  redirectToLogin,
} from "@/lib/api/client";
import { quizCraftCatalogEnabled, quizCraftV2ReadsEnabled } from "@/lib/api/env";
import type { LearningStateItem } from "@/lib/api/types";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";

type LearningStateStatus =
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "unauthenticated" }
  | { status: "error"; message: string }
  | { status: "empty" }
  | { status: "ready"; items: LearningStateItem[] };

/** Renders the server timestamp as-is when it cannot be parsed; never guesses. */
function formatUpdatedAt(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { dateStyle: "medium", timeStyle: "short" });
}

/**
 * The 错题 list is a pure projection of server facts: the server marks each
 * question wrong or not, so the page shows only `wrong === true` items. A
 * failed read stays loading/error/unauthenticated — it can never become a
 * local mock success state.
 */
export default function MistakesPage() {
  usePageEnter(null);
  const enabled = quizCraftV2ReadsEnabled();
  // Bank names come from the coordinated catalog cutover surface, the same
  // source the leaderboard uses. A failed catalog read never blocks the
  // mistake facts; unknown bank ids render as ids instead of made-up names.
  const catalogEnabled = quizCraftCatalogEnabled();
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<LearningStateStatus>({
    status: enabled ? "loading" : "disabled",
  });
  const [bankNames, setBankNames] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    if (!catalogEnabled) return;
    let cancelled = false;
    void fetchQuizCraftCatalog().then(
      (response) => {
        if (cancelled) return;
        setBankNames(
          new Map(response.banks.map((bank) => [bank.bank_id, bank.name]))
        );
      },
      () => {
        // Honest fallback: the mistakes still list with bank ids.
      }
    );
    return () => {
      cancelled = true;
    };
  }, [catalogEnabled]);

  useEffect(() => {
    let active = true;
    if (!enabled) {
      return () => {
        active = false;
      };
    }
    void fetchLearningState()
      .then((response) => {
        if (!active) return;
        const wrong = response.data
          .filter((item) => item.wrong)
          .sort((left, right) => right.updated_at.localeCompare(left.updated_at));
        setState(wrong.length === 0 ? { status: "empty" } : { status: "ready", items: wrong });
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error instanceof PortalUnauthorizedError) {
          setState({ status: "unauthenticated" });
          return;
        }
        setState({ status: "error", message: formatPortalError(error) });
      });

    return () => {
      active = false;
    };
  }, [attempt, enabled]);

  const retry = useCallback(() => {
    if (!enabled) return;
    setState({ status: "loading" });
    setAttempt((current) => current + 1);
  }, [enabled]);

  const bankName = (bankID: string) => bankNames.get(bankID) ?? bankID;

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div data-block data-enter>
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">MISTAKES</span>
          <span className="mx-2">/</span>
          LEARNING STATE
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
          错题本
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-ink/65">
          错题事实由服务端根据作答记录生成，跨设备一致；页面只展示服务端同步的
          事实，不会显示本地猜测的题干或答案。
        </p>
      </div>

      {state.status === "disabled" && (
        <section data-testid="mistakes-disabled" className="mt-10">
          <EmptyBlock label="错题本即将上线，敬请期待" />
        </section>
      )}

      {state.status === "loading" && (
        <section data-testid="mistakes-loading" className="mt-10">
          <LoadingBlock label="正在同步错题" />
        </section>
      )}

      {state.status === "unauthenticated" && (
        <section data-testid="mistakes-unauthenticated" className="mt-10 border border-ink/25 p-6">
          <p className="font-mono text-xs tracking-[0.2em] text-ink/55">
            SIGN IN REQUIRED / 请先登录后查看跨设备同步的错题
          </p>
          <button
            type="button"
            onClick={() => redirectToLogin("/practice/mistakes")}
            className="mt-5 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            登录查看
          </button>
        </section>
      )}

      {state.status === "error" && (
        <section data-testid="mistakes-error" className="mt-10">
          <ErrorBanner message={state.message} onRetry={retry} />
        </section>
      )}

      {state.status === "empty" && (
        <section data-testid="mistakes-empty" className="mt-10">
          <EmptyBlock label="还没有错题记录，继续保持" />
        </section>
      )}

      {state.status === "ready" && (
        <section data-testid="mistakes-list" data-block data-enter className="mt-10">
          <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">
            共 {state.items.length} 道错题 · 服务端学习状态
          </p>
          <div className="mt-5 space-y-4">
            {state.items.map((item) => (
              <article key={item.question_id} className="border border-ink/25 p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="font-mono text-[10px] tracking-[0.2em] text-accent">
                      {bankName(item.bank_id)}
                    </p>
                    <p className="mt-2 break-all font-mono text-xs text-ink/70">
                      题目 {item.question_id}
                    </p>
                  </div>
                  <p className="shrink-0 font-mono text-[10px] tracking-wider text-ink/45">
                    最近更新 {formatUpdatedAt(item.updated_at)}
                  </p>
                </div>
                <div className="mt-4 flex flex-wrap gap-4 border-t border-line pt-3 font-mono text-xs text-ink/60">
                  <span>作答 {item.attempt_count} 次</span>
                  <span>答对 {item.correct_count} 次</span>
                </div>
              </article>
            ))}
          </div>
        </section>
      )}
    </main>
  );
}
