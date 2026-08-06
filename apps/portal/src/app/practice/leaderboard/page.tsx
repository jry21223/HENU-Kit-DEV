"use client";

import { useEffect, useRef, useState } from "react";

import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { RankingAvatar } from "@/components/practice/ranking-avatar";
import {
  fetchQuizCraftBankRanking,
  fetchQuizCraftCatalog,
  fetchQuizCraftOverallRanking,
  formatPortalError,
} from "@/lib/api/client";
import type {
  QuizCraftCatalogBank,
  QuizCraftRankingPeriod,
  QuizCraftRankingResponse,
} from "@/lib/api/types";
import { quizCraftCatalogEnabled, quizCraftV2ReadsEnabled } from "@/lib/api/env";
import { cn } from "@/lib/cn";

type RankingState =
  | { status: "disabled" }
  | { status: "loading" }
  | { status: "ready"; data: QuizCraftRankingResponse["data"] }
  | { status: "error"; message: string };

type BankSourceState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ready"; banks: QuizCraftCatalogBank[] }
  | { status: "error"; message: string };

const periods: Array<{ value: QuizCraftRankingPeriod; label: string }> = [
  { value: "weekly", label: "本周" },
  { value: "lifetime", label: "总榜" },
];

export default function LeaderboardPage() {
  const enabled = quizCraftV2ReadsEnabled();
  // The bank dimension needs the coordinated catalog cutover flag, not just the
  // V2 reads gate: the catalog is a separate cutover surface from rankings.
  const catalogEnabled = quizCraftCatalogEnabled();
  const [scope, setScope] = useState<"overall" | "bank">("overall");
  const [bankID, setBankID] = useState<string | null>(null);
  const [period, setPeriod] = useState<QuizCraftRankingPeriod>("weekly");
  const [state, setState] = useState<RankingState>(
    enabled ? { status: "loading" } : { status: "disabled" }
  );
  const [banks, setBanks] = useState<BankSourceState>(
    catalogEnabled
      ? { status: "loading" }
      : { status: "error", message: "题库列表暂不可用，无法切换题库维度。" }
  );
  const [retry, setRetry] = useState(0);
  const requestVersion = useRef(0);

  // The Bank dimension needs the published V2 catalog to map a bank UUID to a
  // display name. A failed catalog read must never block the Overall ranking.
  useEffect(() => {
    if (!catalogEnabled) return;
    let cancelled = false;
    void fetchQuizCraftCatalog().then(
      (response) => {
        if (!cancelled) setBanks({ status: "ready", banks: response.banks });
      },
      (error: unknown) => {
        if (!cancelled) {
          setBanks({ status: "error", message: formatPortalError(error) });
        }
      }
    );
    return () => {
      cancelled = true;
    };
  }, [catalogEnabled, retry]);

  useEffect(() => {
    if (!enabled) return;
    let request: Promise<QuizCraftRankingResponse>;
    if (scope === "bank") {
      // The bank dimension waits for a user-selected bank; until then the
      // section renders its own prompt instead of stale Overall facts.
      if (banks.status !== "ready" || !bankID) return;
      request = fetchQuizCraftBankRanking(bankID, period);
    } else {
      request = fetchQuizCraftOverallRanking(period);
    }
    const version = ++requestVersion.current;
    // Defer the loading transition to a microtask: the fetch starts here, so
    // the pending state must still be published before any response arrives.
    void Promise.resolve()
      .then(() => {
        if (version !== requestVersion.current) return;
        setState({ status: "loading" });
      })
      .then(() => request)
      .then(
        (response) => {
          if (version !== requestVersion.current) return;
          setState({ status: "ready", data: response.data });
        },
        (error: unknown) => {
          if (version !== requestVersion.current) return;
          setState({ status: "error", message: formatPortalError(error) });
        }
      );
    return () => {
      requestVersion.current += 1;
    };
    // banks intentionally excluded: the catalog resolves once at startup and
    // must not re-fire the ranking request when the bank list state settles.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, scope, bankID, period, retry]);

  const selectPeriod = (value: QuizCraftRankingPeriod) => {
    if (!enabled || value === period) return;
    setPeriod(value);
  };

  const selectScope = (value: "overall" | "bank") => {
    if (!enabled || value === scope) return;
    setBankID(null);
    setScope(value);
  };

  const bankNameFor = (data: QuizCraftRankingResponse["data"]) =>
    banks.status === "ready"
      ? banks.banks.find((bank) => bank.bank_id === data.bank_id)?.name
      : undefined;

  const bankDimensionPending =
    scope === "bank" && (banks.status !== "ready" || !bankID);

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
          只统计 QuizCraft 服务端确认的正确作答；重复提交不会重复计分，公开结果只展示受控昵称与系统头像，不包含邮箱或账户标识。
        </p>

        {enabled && (
          <div className="mt-8 flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2" aria-label="排行榜维度">
              <button
                type="button"
                aria-pressed={scope === "overall"}
                onClick={() => selectScope("overall")}
                className={cn(
                  "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
                  scope === "overall"
                    ? "border-ink bg-ink text-paper"
                    : "border-ink/25 hover:border-ink"
                )}
              >
                综合榜
              </button>
              <button
                type="button"
                aria-pressed={scope === "bank"}
                onClick={() => selectScope("bank")}
                className={cn(
                  "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
                  scope === "bank"
                    ? "border-ink bg-ink text-paper"
                    : "border-ink/25 hover:border-ink"
                )}
              >
                题库榜
              </button>
              {scope === "bank" && banks.status === "ready" && (
                <select
                  aria-label="选择题库"
                  value={bankID ?? ""}
                  onChange={(event) => setBankID(event.target.value || null)}
                  className="max-w-full border border-ink/25 bg-paper px-3 py-2 font-mono text-xs tracking-widest text-ink focus:border-ink focus:outline-none"
                >
                  <option value="" disabled>
                    请选择题库
                  </option>
                  {banks.banks.map((bank) => (
                    <option key={bank.bank_id} value={bank.bank_id}>
                      {bank.name}
                    </option>
                  ))}
                </select>
              )}
              {scope === "bank" && banks.status === "loading" && (
                <span className="py-2 font-mono text-xs tracking-widest text-ink/50">
                  正在读取题库列表…
                </span>
              )}
            </div>
            <div className="flex gap-2" aria-label="排行榜周期">
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
            </div>
          </div>
        )}

        <section className="mt-8" data-testid="practice-leaderboard">
          {state.status === "disabled" && (
            <EmptyBlock label="QuizCraft V2 排行榜将在确认切换后启用" />
          )}
          {bankDimensionPending && banks.status === "loading" && (
            <LoadingBlock label="正在读取题库列表" />
          )}
          {bankDimensionPending && banks.status === "error" && (
            <ErrorBanner message="题库列表暂不可用，无法切换题库维度。" />
          )}
          {bankDimensionPending &&
            banks.status === "ready" &&
            (banks.banks.length === 0 ? (
              <EmptyBlock label="当前没有已发布的题库" />
            ) : (
              <EmptyBlock label="请选择题库查看题库排行" />
            ))}
          {!bankDimensionPending && state.status === "loading" && (
            <LoadingBlock label="正在读取真实排行榜" />
          )}
          {!bankDimensionPending && state.status === "error" && (
            <ErrorBanner
              message={state.message}
              onRetry={() => {
                setState({ status: "loading" });
                setRetry((value) => value + 1);
              }}
            />
          )}
          {!bankDimensionPending && state.status === "ready" && state.data.entries.length === 0 && (
            <EmptyBlock label="当前周期尚无公开排行事实" />
          )}
          {!bankDimensionPending && state.status === "ready" && state.data.entries.length > 0 && (
            <div>
              {scope === "bank" && bankNameFor(state.data) && (
                <p className="mb-3 font-mono text-xs tracking-[0.3em] text-ink/60">
                  BANK / {bankNameFor(state.data)}
                </p>
              )}
              <ol
                className="divide-y divide-ink/15 border border-ink/25"
                data-practice-leaderboard-state="ready"
              >
                {state.data.entries.map((entry, index) => (
                  <li
                    key={`${entry.rank}:${index}`}
                    className="grid grid-cols-[3rem_2rem_1fr_auto] items-center gap-3 px-4 py-4 md:px-6"
                  >
                    <span className="font-display text-2xl font-bold tabular-nums">
                      {String(entry.rank).padStart(2, "0")}
                    </span>
                    <RankingAvatar avatar={entry.system_avatar} size={28} />
                    <span className="min-w-0 truncate font-medium">{entry.nickname}</span>
                    <span className="font-mono text-xs tabular-nums text-ink/65">
                      {entry.correct_answer_count} 题
                    </span>
                  </li>
                ))}
              </ol>
            </div>
          )}
        </section>
      </div>
    </main>
  );
}
