"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { useReveal } from "@/components/account/use-reveal";
import { fetchAccountPoints, formatPortalError } from "@/lib/api/client";
import type { AccountPointEntry, AccountPointsResponse } from "@/lib/api/types";

type WalletState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | {
      kind: "success";
      balance: number;
      entries: AccountPointEntry[];
      nextCursor: string | null;
      loadingMore: boolean;
      moreError: string;
    };

function formatPoints(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(date);
}

function mergeEntries(existing: AccountPointEntry[], next: AccountPointEntry[]): AccountPointEntry[] {
  const seen = new Set(existing.map((entry) => entry.id));
  return [...existing, ...next.filter((entry) => !seen.has(entry.id))];
}

function walletState(response: AccountPointsResponse): Extract<WalletState, { kind: "success" }> {
  return {
    kind: "success",
    balance: response.data.balance,
    entries: response.data.entries,
    nextCursor: response.data.next_cursor,
    loadingMore: false,
    moreError: "",
  };
}

export default function WalletPage() {
  const [state, setState] = useState<WalletState>({ kind: "loading" });
  const requestVersion = useRef(0);
  const handleUnauthorized = useAccountConsoleUnauthorizedHandler();
  useReveal();

  const requestWallet = useCallback((version: number) => {
    void fetchAccountPoints().then(
      (response) => {
        if (version === requestVersion.current) setState(walletState(response));
      },
      (error: unknown) => {
        if (version === requestVersion.current && !handleUnauthorized(error)) {
          setState({ kind: "error", message: formatPortalError(error) });
        }
      }
    );
  }, [handleUnauthorized]);

  const loadWallet = useCallback(() => {
    const version = ++requestVersion.current;
    setState({ kind: "loading" });
    requestWallet(version);
  }, [requestWallet]);

  useEffect(() => {
    const version = ++requestVersion.current;
    requestWallet(version);
    return () => {
      requestVersion.current += 1;
    };
  }, [requestWallet]);

  const loadMore = () => {
    if (state.kind !== "success" || !state.nextCursor || state.loadingMore) return;
    const cursor = state.nextCursor;
    const version = requestVersion.current;
    setState((current) => current.kind === "success"
      ? { ...current, loadingMore: true, moreError: "" }
      : current);
    void fetchAccountPoints(cursor).then(
      (response) => {
        if (version !== requestVersion.current) return;
        setState((current) => {
          if (current.kind !== "success" || current.nextCursor !== cursor) return current;
          return {
            kind: "success",
            balance: response.data.balance,
            entries: mergeEntries(current.entries, response.data.entries),
            nextCursor: response.data.next_cursor,
            loadingMore: false,
            moreError: "",
          };
        });
      },
      (error: unknown) => {
        if (version !== requestVersion.current) return;
        if (handleUnauthorized(error)) return;
        setState((current) => current.kind === "success" && current.nextCursor === cursor
          ? { ...current, loadingMore: false, moreError: formatPortalError(error) }
          : current);
      }
    );
  };

  return (
    <div>
      <section data-enter className="border-b border-ink pb-5">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
          <span className="text-accent">A-03</span>
          <span className="mx-2">/</span>
          WALLET
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">积分钱包</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
          余额与明细只来自 Account Portfolio 的不可变积分账本。本页不提供签到、任务、支付或积分消费入口。
        </p>
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-points-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          POINT LEDGER LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section data-account-points-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">POINT LEDGER UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
          <p className="mt-3 text-sm leading-6 text-ink/60">账户服务不可用时，不会以本地余额或会话数据替代真实积分账本。</p>
          <button
            type="button"
            onClick={loadWallet}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {state.kind === "success" ? (
        <section data-account-points-state="success" className="mt-6">
          <div data-enter className="border border-ink p-6 sm:p-8">
            <p className="font-mono text-xs tracking-[0.2em] text-ink/45">PERSISTED BALANCE</p>
            <p className="mt-3 font-display text-5xl font-bold tracking-tight">{formatPoints(state.balance)}</p>
            <p className="mt-2 text-sm leading-6 text-ink/60">当前积分余额 · 每次打开或继续加载都会从服务端重新读取。</p>
          </div>

          <div className="mt-8">
            <div className="flex flex-wrap items-end justify-between gap-3 border-b border-ink pb-3">
              <div>
                <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">IMMUTABLE LEDGER</p>
                <h2 className="mt-1 font-display text-2xl font-bold">积分变动明细</h2>
              </div>
              <span className="font-mono text-[10px] tracking-[0.12em] text-ink/45">时间以中国标准时间显示</span>
            </div>

            {state.entries.length === 0 ? (
              <div data-account-points-empty className="border-b border-line py-8">
                <p className="font-display text-xl font-bold">暂无积分变动</p>
                <p className="mt-2 text-sm leading-6 text-ink/60">后续经授权的积分调整会以不可变账本条目的形式出现在这里。</p>
              </div>
            ) : (
              <div className="border-t border-line">
                {state.entries.map((entry) => (
                  <article key={entry.id} className="grid gap-3 border-b border-line py-5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start sm:gap-6">
                    <div className="min-w-0">
                      <h3 className="break-words text-base font-semibold leading-6">{entry.reason}</h3>
                      <p className="mt-2 font-mono text-[10px] tracking-[0.08em] text-ink/45">{formatTimestamp(entry.created_at)}</p>
                    </div>
                    <p className={`font-display text-2xl font-bold ${entry.amount >= 0 ? "text-accent" : "text-ink"}`}>
                      {entry.amount > 0 ? "+" : ""}{formatPoints(entry.amount)}
                    </p>
                  </article>
                ))}
              </div>
            )}

            {state.moreError ? (
              <p role="alert" className="mt-4 border border-accent px-4 py-3 text-sm leading-6 text-accent">继续加载失败：{state.moreError}</p>
            ) : null}
            {state.nextCursor ? (
              <button
                type="button"
                onClick={loadMore}
                disabled={state.loadingMore}
                className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-wait disabled:opacity-50"
              >
                {state.loadingMore ? "正在加载…" : "加载更多记录"}
              </button>
            ) : null}
          </div>
        </section>
      ) : null}
    </div>
  );
}
