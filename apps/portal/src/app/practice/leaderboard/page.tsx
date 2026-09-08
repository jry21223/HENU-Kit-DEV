"use client";

import { useEffect, useRef, useState } from "react";

import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import {
  fetchQuizCraftOverallRanking,
  formatPortalError,
} from "@/lib/api/client";
import type {
  QuizCraftRankingPeriod,
  QuizCraftRankingResponse,
} from "@/lib/api/types";
import { quizCraftV2ReadsEnabled } from "@/lib/api/env";
import { cn } from "@/lib/cn";

type State =
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "ready"; data: QuizCraftRankingResponse["data"] }
  | { status: "error"; message: string };

const periods: Array<{ value: QuizCraftRankingPeriod; label: string }> = [
  { value: "weekly", label: "本周" },
  { value: "lifetime", label: "总榜" },
];

export default function LeaderboardPage() {
  const enabled = quizCraftV2ReadsEnabled();
  const [period, setPeriod] = useState<QuizCraftRankingPeriod>("weekly");
  const [state, setState] = useState<State>(enabled ? { status: "loading" } : { status: "disabled" });
  const [retry, setRetry] = useState(0);
  const requestRef = useRef<{
    key: string;
    promise: ReturnType<typeof fetchQuizCraftOverallRanking>;
  } | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const requestKey = `${period}:${retry}`;
    if (requestRef.current?.key !== requestKey) {
      requestRef.current = {
        key: requestKey,
        promise: fetchQuizCraftOverallRanking(period),
      };
    }
    void requestRef.current.promise.then(
      (response) => {
        if (!cancelled) setState({ status: "ready", data: response.data });
      },
      (error: unknown) => {
        if (!cancelled) {
          setState({ status: "error", message: formatPortalError(error) });
        }
      }
    );
    return () => {
      cancelled = true;
    };
  }, [enabled, period, retry]);

  const selectPeriod = (value: QuizCraftRankingPeriod) => {
    if (!enabled || value === period) return;
    setState({ status: "loading" });
    setPeriod(value);
  };

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div className="max-w-4xl">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">RANK</span>
          <span className="mx-2">/</span>
          VERIFIED ATTEMPTS
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">排行榜</h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-ink/65">
          只统计练习服务确认的正确作答；重复提交不会重复计分，公开结果不包含邮箱或账户标识。
        </p>

        {enabled && <div className="mt-8 flex gap-2" aria-label="排行榜周期">
          {periods.map((item) => (
            <button
              key={item.value}
              type="button"
              aria-pressed={period === item.value}
              onClick={() => selectPeriod(item.value)}
              className={cn(
                "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
                period === item.value
                  ? "border-ink bg-ink text-paper"
                  : "border-ink/25 hover:border-ink"
              )}
            >
              {item.label}
            </button>
          ))}
        </div>}

        <section className="mt-8" data-testid="practice-leaderboard">
          {state.status === "disabled" && (
            <EmptyBlock label="排行榜数据暂未开放" />
          )}
          {state.status === "loading" && <LoadingBlock label="正在读取真实排行榜" />}
          {state.status === "error" && (
            <ErrorBanner
              message={state.message}
              onRetry={() => {
                setState({ status: "loading" });
                setRetry((value) => value + 1);
              }}
            />
          )}
          {state.status === "ready" && state.data.entries.length === 0 && (
            <EmptyBlock label="当前周期尚无公开排行事实" />
          )}
          {state.status === "ready" && state.data.entries.length > 0 && (
            <ol
              className="divide-y divide-ink/15 border border-ink/25"
              data-practice-leaderboard-state="ready"
            >
              {state.data.entries.map((entry, index) => (
                <li
                  key={`${entry.rank}:${index}`}
                  className="grid grid-cols-[3rem_1fr_auto] items-center gap-3 px-4 py-4 md:px-6"
                >
                  <span className="font-display text-2xl font-bold tabular-nums">
                    {String(entry.rank).padStart(2, "0")}
                  </span>
                  <span className="min-w-0 truncate font-medium">{entry.nickname}</span>
                  <span className="font-mono text-xs tabular-nums text-ink/65">
                    {entry.correct_answer_count} 题
                  </span>
                </li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </main>
  );
}
